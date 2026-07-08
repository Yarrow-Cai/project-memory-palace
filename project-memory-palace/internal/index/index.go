package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/atop/project-memory-palace/internal/memory"
	"github.com/atop/project-memory-palace/internal/store"
	_ "modernc.org/sqlite"
)

var (
	cjkRe = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]+`)
)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY, type TEXT NOT NULL, status TEXT NOT NULL,
    title TEXT NOT NULL, summary TEXT NOT NULL, source_kind TEXT NOT NULL,
    confidence REAL NOT NULL, priority INTEGER NOT NULL DEFAULT 3,
    tags_json TEXT NOT NULL, modules_json TEXT NOT NULL,
    paths_json TEXT NOT NULL, expires_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
    id UNINDEXED, title, summary, content, tags, modules, paths, tokenize='unicode61'
);
CREATE TABLE IF NOT EXISTS relations (
    source_id TEXT NOT NULL, relation TEXT NOT NULL, target_id TEXT NOT NULL,
    PRIMARY KEY (source_id, relation, target_id)
);
CREATE TABLE IF NOT EXISTS memory_paths (
    memory_id TEXT NOT NULL, path TEXT NOT NULL, PRIMARY KEY (memory_id, path)
);
`

// cjkBigram converts a CJK string into space-separated bigrams for FTS indexing.
// "inverter" -> inverted bigram; single/double char passes through
func cjkBigram(s string) string {
	runes := []rune(s)
	if len(runes) <= 2 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(runes)-1; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(runes[i])
		b.WriteRune(runes[i+1])
	}
	return b.String()
}

// ftsPreprocess preprocesses text for FTS5 storage:
// ASCII words pass through; CJK runs are converted to bigrams.
func ftsPreprocess(text string) string {
	parts := cjkRe.Split(text, -1)
	cjkParts := cjkRe.FindAllString(text, -1)
	var out strings.Builder
	for i := 0; i < len(parts) || i < len(cjkParts); i++ {
		if i < len(parts) && parts[i] != "" {
			if out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(strings.TrimSpace(parts[i]))
		}
		if i < len(cjkParts) {
			bg := cjkBigram(cjkParts[i])
			if bg != "" {
				if out.Len() > 0 {
					out.WriteByte(' ')
				}
				out.WriteString(bg)
			}
		}
	}
	return out.String()
}

type MemoryIndex struct {
	projectRoot string
	dbPath      string
	dbOnce      sync.Once
	db          *sql.DB
	dbErr       error
	expireOnce  sync.Once
	stopExpire  chan struct{}
}

func NewMemoryIndex(projectRoot string) *MemoryIndex {
	return &MemoryIndex{projectRoot: projectRoot, dbPath: store.IndexPath(projectRoot)}
}

func (idx *MemoryIndex) connect() (*sql.DB, error) {
	idx.dbOnce.Do(func() {
		idx.db, idx.dbErr = sql.Open("sqlite", idx.dbPath)
	})
	return idx.db, idx.dbErr
}

// Close closes the underlying SQLite database connection and resets the
// sync.Once so a future connect() can reopen. Safe to call even if
// connect() was never invoked.
func (idx *MemoryIndex) Close() error {
	if idx.db != nil {
		if idx.stopExpire != nil {
			close(idx.stopExpire)
		}
		err := idx.db.Close()
		idx.db = nil
		idx.dbOnce = sync.Once{}
		idx.expireOnce = sync.Once{}
		return err
	}
	return nil
}

func (idx *MemoryIndex) Initialize() error {
	db, err := idx.connect()
	if err != nil { return err }
	_, err = db.Exec(schemaDDL)
	if err != nil { return err }
	// Migration: add priority column if missing (added in schema v2)
	db.Exec("ALTER TABLE memories ADD COLUMN priority INTEGER NOT NULL DEFAULT 3")
	// Migration: add expires_at column if missing (added in schema v3)
	db.Exec("ALTER TABLE memories ADD COLUMN expires_at TEXT NOT NULL DEFAULT ''")
	// Migration: add access_count and last_accessed_at columns (added in schema v4)
	db.Exec("ALTER TABLE memories ADD COLUMN access_count INTEGER NOT NULL DEFAULT 0")
	db.Exec("ALTER TABLE memories ADD COLUMN last_accessed_at TEXT NOT NULL DEFAULT ''")
	// Auto-expire cards with past expires_at
	db.Exec("UPDATE memories SET status='expired' WHERE status='active' AND expires_at != '' AND expires_at <= datetime('now')")
		// Start background auto-expire goroutine
		idx.startAutoExpire()
	// Migration: add source_agent column (added in schema v5)
	db.Exec("ALTER TABLE memories ADD COLUMN source_agent TEXT NOT NULL DEFAULT ''")
	// Migration: add knowledge_kind column (added in schema v5)
	db.Exec("ALTER TABLE memories ADD COLUMN knowledge_kind TEXT NOT NULL DEFAULT ''")
		// Migration: add index on memory_paths for faster context_for_files lookups (schema v6)
		db.Exec("CREATE INDEX IF NOT EXISTS idx_memory_paths_mid ON memory_paths(memory_id)")
	return nil
}

func (idx *MemoryIndex) Clear() error {
	db, err := idx.connect()
	if err != nil { return err }
	tx, err := db.Begin()
	if err != nil { return fmt.Errorf("clear begin: %w", err) }
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM memory_paths"); err != nil { return fmt.Errorf("clear memory_paths: %w", err) }
	if _, err := tx.Exec("DELETE FROM relations"); err != nil { return fmt.Errorf("clear relations: %w", err) }
	if _, err := tx.Exec("DELETE FROM memory_fts"); err != nil { return fmt.Errorf("clear memory_fts: %w", err) }
	if _, err := tx.Exec("DELETE FROM memories"); err != nil { return fmt.Errorf("clear memories: %w", err) }
	return tx.Commit()
}

func (idx *MemoryIndex) Upsert(card *memory.MemoryCard) error {
	db, err := idx.connect()
	if err != nil { return err }
	tx, err := db.Begin()
	if err != nil { return fmt.Errorf("upsert begin: %w", err) }
	defer tx.Rollback()
	if err := doUpsert(tx, card); err != nil { return err }
	return tx.Commit()
}

func doUpsert(tx *sql.Tx, card *memory.MemoryCard) error {
	// Default priority for legacy cards
	if card.Priority < 1 || card.Priority > 5 {
		card.Priority = 3
	}
	tagsJSON, err := json.Marshal(card.Tags)
	if err != nil { return fmt.Errorf("marshal tags: %w", err) }
	modsJSON, err := json.Marshal(card.Scope.Modules)
	if err != nil { return fmt.Errorf("marshal modules: %w", err) }
	pathsJSON, err := json.Marshal(card.Scope.Paths)
	if err != nil { return fmt.Errorf("marshal paths: %w", err) }
	q := `INSERT INTO memories(id,type,status,title,summary,source_kind,confidence,priority,tags_json,modules_json,paths_json,expires_at,updated_at,source_agent,knowledge_kind) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET type=excluded.type,status=excluded.status,title=excluded.title,summary=excluded.summary,source_kind=excluded.source_kind,confidence=excluded.confidence,priority=excluded.priority,tags_json=excluded.tags_json,modules_json=excluded.modules_json,paths_json=excluded.paths_json,expires_at=excluded.expires_at,updated_at=excluded.updated_at,source_agent=excluded.source_agent,knowledge_kind=excluded.knowledge_kind`
	_, err = tx.Exec(q, card.ID, card.Type, card.Status, card.Title, card.Summary, card.Source.Kind, card.Confidence, card.Priority, string(tagsJSON), string(modsJSON), string(pathsJSON), card.ExpiresAt, card.UpdatedAt, card.SourceAgent, card.KnowledgeKind)
	if err != nil { return fmt.Errorf("upsert: %w", err) }
	if _, err := tx.Exec("DELETE FROM memory_fts WHERE id=?", card.ID); err != nil { return fmt.Errorf("upsert fts delete: %w", err) }
	if _, err := tx.Exec("INSERT INTO memory_fts(id,title,summary,content,tags,modules,paths) VALUES(?,?,?,?,?,?,?)", card.ID, ftsPreprocess(card.Title), ftsPreprocess(card.Summary), ftsPreprocess(card.Content), ftsPreprocess(strings.Join(card.Tags," ")), ftsPreprocess(strings.Join(card.Scope.Modules," ")), ftsPreprocess(strings.Join(card.Scope.Paths," "))); err != nil { return fmt.Errorf("upsert fts insert: %w", err) }
	if _, err := tx.Exec("DELETE FROM memory_paths WHERE memory_id=?", card.ID); err != nil { return fmt.Errorf("upsert paths delete: %w", err) }
	for _, p := range card.Scope.Paths {
		if _, err := tx.Exec("INSERT OR IGNORE INTO memory_paths(memory_id,path) VALUES(?,?)", card.ID, p); err != nil { return fmt.Errorf("upsert path insert: %w", err) }
	}
	if _, err := tx.Exec("DELETE FROM relations WHERE source_id=?", card.ID); err != nil { return fmt.Errorf("upsert relations delete: %w", err) }
	for rel, targets := range card.Relations {
		for _, t := range targets {
			if _, err := tx.Exec("INSERT OR IGNORE INTO relations(source_id,relation,target_id) VALUES(?,?,?)", card.ID, rel, t); err != nil { return fmt.Errorf("upsert relation insert: %w", err) }
		}
	}
	return nil
}

// Delete removes a single memory card from the index (all related tables).
func (idx *MemoryIndex) Delete(id string) error {
	db, err := idx.connect()
	if err != nil { return err }
	if err := idx.Initialize(); err != nil { return err }
	tx, err := db.Begin()
	if err != nil { return fmt.Errorf("delete begin: %w", err) }
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM memory_paths WHERE memory_id=?", id); err != nil { return fmt.Errorf("delete paths: %w", err) }
	if _, err := tx.Exec("DELETE FROM relations WHERE source_id=? OR target_id=?", id, id); err != nil { return fmt.Errorf("delete relations: %w", err) }
	if _, err := tx.Exec("DELETE FROM memory_fts WHERE id=?", id); err != nil { return fmt.Errorf("delete fts: %w", err) }
	if _, err := tx.Exec("DELETE FROM memories WHERE id=?", id); err != nil { return fmt.Errorf("delete memory: %w", err) }
	return tx.Commit()
}

// ListExpired returns memory IDs whose status is expired - for purge.
func (idx *MemoryIndex) ListExpired() ([]string, error) {
	if err := idx.Initialize(); err != nil { return nil, err }
	db, err := idx.connect()
	if err != nil { return nil, err }
	rows, err := db.Query("SELECT id FROM memories WHERE status='expired' ORDER BY updated_at DESC")
	if err != nil { return nil, fmt.Errorf("list expired: %w", err) }
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil { return nil, err }
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (idx *MemoryIndex) startAutoExpire() {
	idx.expireOnce.Do(func() {
		idx.stopExpire = make(chan struct{})
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					idx.runAutoExpire()
				case <-idx.stopExpire:
					return
				}
			}
		}()
	})
}

func (idx *MemoryIndex) runAutoExpire() {
	db, err := idx.connect()
	if err != nil {
		return
	}
	db.Exec("UPDATE memories SET status='expired' WHERE status='active' AND expires_at != '' AND expires_at <= datetime('now')")
}

func (idx *MemoryIndex) Search(query string, filters map[string]any, limit int) ([]map[string]any, error) {
	if err := idx.Initialize(); err != nil { return nil, err }
	ftsQuery := toFTSQuery(query)
	if ftsQuery == "" { return nil, nil }
	statuses := []string{"active"}
	var pathFilters []string
	if filters != nil {
		if s, ok := filters["status"]; ok {
			switch v := s.(type) {
			case string: statuses = []string{v}
			case []string: statuses = v
			}
		}
		if p, ok := filters["paths"]; ok {
			switch v := p.(type) {
			case string: pathFilters = []string{v}
			case []string: pathFilters = v
			}
		}
	}
	db, err := idx.connect()
	if err != nil { return nil, err }
	args := []any{ftsQuery}
	for _, s := range statuses { args = append(args, s) }
	for _, p := range pathFilters { args = append(args, p) }
	args = append(args, limit)
	sp := strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",")
	where := "m.status IN (" + sp + ")"
	if len(pathFilters) > 0 {
		pp := strings.TrimSuffix(strings.Repeat("?,", len(pathFilters)), ",")
		where += " AND EXISTS(SELECT 1 FROM memory_paths mp WHERE mp.memory_id=m.id AND mp.path IN (" + pp + "))"
	}
	where += " AND (m.expires_at = '' OR m.expires_at > datetime('now'))"
	q := fmt.Sprintf("SELECT m.id,m.type,m.status,m.title,m.summary,m.source_kind,m.confidence,m.priority,m.updated_at,m.access_count,m.last_accessed_at FROM memory_fts JOIN memories m ON m.id=memory_fts.id WHERE memory_fts MATCH ? AND %s ORDER BY rank ASC,m.updated_at DESC LIMIT ?", where)
	rows, err := db.Query(q, args...)
	if err != nil { return nil, fmt.Errorf("search: %w", err) }
	defer rows.Close()
	var results []map[string]any
	for rows.Next() {
		var id, tp, st, title, summary, sk, upd, lastAcc string
		var conf float64
		var priority int
		var accessCount int
		rows.Scan(&id,&tp,&st,&title,&summary,&sk,&conf,&priority,&upd,&accessCount,&lastAcc)
		ep := EffectivePriority(priority, lastAcc)
		results = append(results, map[string]any{"id":id,"type":tp,"status":st,"title":title,"summary":summary,"confidence":conf,"priority":priority,"source_hint":sk,"matched_by":[]string{"fts"},"updated_at":upd,"access_count":accessCount,"last_accessed_at":lastAcc,"effective_priority":ep})
	}
	return results, rows.Err()
}

func (idx *MemoryIndex) Recent(limit, offset int, filters map[string]any) ([]map[string]any, error) {
	if err := idx.Initialize(); err != nil { return nil, err }
	db, err := idx.connect()
	if err != nil { return nil, err }
	if limit <= 0 { limit = 20 }

	where := ""
	args := []any{}
	hasStatusFilter := false
	if filters != nil {
		if tp, ok := filters["type"].(string); ok && tp != "" {
			where += " AND type = ?"
			args = append(args, tp)
		}
		if st, ok := filters["status"].(string); ok && st != "" {
			where += " AND status = ?"
			args = append(args, st)
			hasStatusFilter = true
		}
		if pr, ok := filters["priority"].(int); ok && pr > 0 {
			where += " AND priority >= ?"
			args = append(args, pr)
		}
	}
	// Default: exclude expired unless explicitly requested
	if !hasStatusFilter {
		where += " AND status != 'expired'"
	}
	// Always filter out time-expired cards
	where += " AND (expires_at = '' OR expires_at > datetime('now'))"
	query := "SELECT id,type,status,priority,title,summary,source_kind,confidence,updated_at,access_count,last_accessed_at FROM memories WHERE 1=1" + where + " ORDER BY updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := db.Query(query, args...)
	if err != nil { return nil, fmt.Errorf("recent: %w", err) }
	defer rows.Close()
	var results []map[string]any
	for rows.Next() {
		var id, tp, st, title, summary, sk, upd, lastAcc string
		var priority int
		var conf float64
		var accessCount int
		rows.Scan(&id,&tp,&st,&priority,&title,&summary,&sk,&conf,&upd,&accessCount,&lastAcc)
		ep := EffectivePriority(priority, lastAcc)
		results = append(results, map[string]any{"id":id,"type":tp,"status":st,"priority":priority,"title":title,"summary":summary,"confidence":conf,"source_hint":sk,"matched_by":[]string{"recent"},"updated_at":upd,"access_count":accessCount,"last_accessed_at":lastAcc,"effective_priority":ep})
	}
	return results, rows.Err()
}

func (idx *MemoryIndex) Count(filters map[string]any) (int, error) {
	if err := idx.Initialize(); err != nil { return 0, err }
	db, err := idx.connect()
	if err != nil { return 0, err }

	where := ""
	args := []any{}
	if filters != nil {
		if tp, ok := filters["type"].(string); ok && tp != "" {
			where += " AND type = ?"
			args = append(args, tp)
		}
		if st, ok := filters["status"].(string); ok && st != "" {
			where += " AND status = ?"
			args = append(args, st)
		}
		if pr, ok := filters["priority"].(int); ok && pr > 0 {
			where += " AND priority >= ?"
			args = append(args, pr)
		}
	}
	// Always exclude time-expired cards
	where += " AND (expires_at = '' OR expires_at > datetime('now'))"
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM memories WHERE 1=1"+where, args...).Scan(&count)
	if err != nil { return 0, fmt.Errorf("count: %w", err) }
	return count, nil
}

// GetMemory looks up a single memory card by ID from the SQLite index.
// Returns a structured map (without content field) or nil if not found.
func (idx *MemoryIndex) GetMemory(id string) (map[string]any, error) {
	if err := idx.Initialize(); err != nil { return nil, err }
	db, err := idx.connect()
	if err != nil { return nil, err }

	var (
		dbID, tp, st, title, summary, sk        string
		tagsJSON, modsJSON, pathsJSON, expires, upd string
		sourceAgent, knowledgeKind, lastAccessedAt string
		conf     float64
		priority int
		accessCount int
	)
	err = db.QueryRow(
		"SELECT id,type,status,title,summary,source_kind,confidence,priority,tags_json,modules_json,paths_json,expires_at,updated_at,source_agent,knowledge_kind,access_count,last_accessed_at FROM memories WHERE id=?", id,
	).Scan(&dbID, &tp, &st, &title, &summary, &sk, &conf, &priority, &tagsJSON, &modsJSON, &pathsJSON, &expires, &upd, &sourceAgent, &knowledgeKind, &accessCount, &lastAccessedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}

	var tags, mods, paths []string
	json.Unmarshal([]byte(tagsJSON), &tags)
	json.Unmarshal([]byte(modsJSON), &mods)
	json.Unmarshal([]byte(pathsJSON), &paths)

	return map[string]any{
		"id": dbID, "type": tp, "status": st,
		"title": title, "summary": summary,
		"source_hint": sk,
		"confidence": conf, "priority": priority,
		"tags": tags, "modules": mods, "paths": paths,
		"expires_at": expires, "updated_at": upd,
		"access_count": accessCount, "last_accessed_at": lastAccessedAt,
		"source_agent": sourceAgent, "knowledge_kind": knowledgeKind,
	}, nil
}

// FindReferrers returns all memory IDs that have a relation pointing to the given target.
func (idx *MemoryIndex) FindReferrers(targetID string) ([]string, error) {
	if err := idx.Initialize(); err != nil { return nil, err }
	db, err := idx.connect()
	if err != nil { return nil, err }
	rows, err := db.Query("SELECT source_id FROM relations WHERE target_id=?", targetID)
	if err != nil { return nil, fmt.Errorf("find referrers: %w", err) }
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil { return nil, err }
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (idx *MemoryIndex) Rebuild() error {
	cards, err := store.DiscoverCards(idx.projectRoot)
	if err != nil { return fmt.Errorf("rebuild: %w", err) }
	idx.Initialize()
	idx.Clear()
	db, err := idx.connect()
	if err != nil { return err }
	tx, err := db.Begin()
	if err != nil { return fmt.Errorf("rebuild begin: %w", err) }
	defer tx.Rollback()
	for _, c := range cards {
		if err := doUpsert(tx, c); err != nil { return fmt.Errorf("rebuild %s: %w", c.ID, err) }
	}
	return tx.Commit()
}

func (idx *MemoryIndex) SearchByPaths(paths []string, limit int) ([]map[string]any, error) {
	if err := idx.Initialize(); err != nil { return nil, err }
	if limit <= 0 { limit = 20 }
	db, err := idx.connect()
	if err != nil { return nil, err }

	args := []any{}
	conditions := []string{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" { continue }
		conditions = append(conditions,
			"EXISTS(SELECT 1 FROM memory_paths mp WHERE mp.memory_id=m.id AND (mp.path LIKE '%' || ? OR ? LIKE '%' || mp.path))")
		args = append(args, p, p)
	}
	if len(conditions) == 0 { return nil, nil }

	q := "SELECT DISTINCT m.id,m.type,m.status,m.title,m.summary,m.source_kind,m.confidence,m.priority,m.updated_at" +
		" FROM memories m" +
		" WHERE m.status='active'" +
		" AND (m.expires_at = '' OR m.expires_at > datetime('now'))" +
		" AND (" + strings.Join(conditions, " OR ") + ")" +
		" ORDER BY m.priority DESC, m.updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(q, args...)
	if err != nil { return nil, fmt.Errorf("search_by_paths: %w", err) }
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id, tp, st, title, summary, sk, upd string
		var conf float64
		var priority int
		rows.Scan(&id, &tp, &st, &title, &summary, &sk, &conf, &priority, &upd)
		results = append(results, map[string]any{"id": id, "type": tp, "status": st, "title": title, "summary": summary, "confidence": conf, "priority": priority, "source_hint": sk, "matched_by": []string{"paths"}, "updated_at": upd})
	}
	return results, rows.Err()
}

// RecordAccess increments access_count and updates last_accessed_at for the given memory IDs.
func (idx *MemoryIndex) RecordAccess(ids []string) error {
	if len(ids) == 0 { return nil }
	db, err := idx.connect()
	if err != nil { return err }
	now := time.Now().Format(time.RFC3339)
	placeholders := make([]string, len(ids))
	for i := range placeholders { placeholders[i] = "?" }
	q := "UPDATE memories SET access_count = access_count + 1, last_accessed_at = ? WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	args := append([]any{now}, toAnySlice(ids)...)
	_, err = db.Exec(q, args...)
	return err
}

func toAnySlice(ss []string) []any {
	result := make([]any, len(ss))
	for i, s := range ss { result[i] = s }
	return result
}

// HotMemories returns active memories ordered by access_count DESC.
func (idx *MemoryIndex) HotMemories(limit int) ([]map[string]any, error) {
	if err := idx.Initialize(); err != nil { return nil, err }
	db, err := idx.connect()
	if err != nil { return nil, err }
	if limit <= 0 { limit = 20 }
	q := "SELECT id,type,status,priority,title,summary,source_kind,confidence,updated_at,access_count,last_accessed_at FROM memories WHERE status='active' AND access_count > 0 AND (expires_at = '' OR expires_at > datetime('now')) ORDER BY access_count DESC LIMIT ?"
	rows, err := db.Query(q, limit)
	if err != nil { return nil, fmt.Errorf("hot: %w", err) }
	defer rows.Close()
	var results []map[string]any
	for rows.Next() {
		var id, tp, st, title, summary, sk, upd, lastAcc string
		var priority int
		var conf float64
		var accessCount int
		rows.Scan(&id, &tp, &st, &priority, &title, &summary, &sk, &conf, &upd, &accessCount, &lastAcc)
		results = append(results, map[string]any{"id": id, "type": tp, "status": st, "priority": priority, "title": title, "summary": summary, "confidence": conf, "source_hint": sk, "matched_by": []string{"hot"}, "updated_at": upd, "access_count": accessCount, "last_accessed_at": lastAcc})
	}
	return results, rows.Err()
}

func toFTSQuery(query string) string {
	// Apply same bigram preprocessing as storage, then split into tokens
	preprocessed := ftsPreprocess(query)
	terms := strings.Fields(preprocessed)
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = "\"" + strings.ReplaceAll(t, "\"", "\"\"") + "\""
	}
	return strings.Join(quoted, " ")
}
