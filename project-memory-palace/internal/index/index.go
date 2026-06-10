package index

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "regexp"
    "strings"

    "github.com/atop/project-memory-palace/internal/memory"
    "github.com/atop/project-memory-palace/internal/store"
    _ "modernc.org/sqlite"
)

var ftsTokenRe = regexp.MustCompile(`[^\W_]+`)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY, type TEXT NOT NULL, status TEXT NOT NULL,
    title TEXT NOT NULL, summary TEXT NOT NULL, source_kind TEXT NOT NULL,
    confidence REAL NOT NULL, tags_json TEXT NOT NULL, modules_json TEXT NOT NULL,
    paths_json TEXT NOT NULL, updated_at TEXT NOT NULL
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

type MemoryIndex struct { projectRoot string; dbPath string }

func NewMemoryIndex(projectRoot string) *MemoryIndex {
    return &MemoryIndex{projectRoot: projectRoot, dbPath: store.IndexPath(projectRoot)}
}

func (idx *MemoryIndex) Connect() (*sql.DB, error) { return sql.Open("sqlite", idx.dbPath) }

func (idx *MemoryIndex) Initialize() error {
    db, err := idx.Connect()
    if err != nil { return err }
    defer db.Close()
    _, err = db.Exec(schemaDDL)
    return err
}

func (idx *MemoryIndex) Clear() error {
    db, err := idx.Connect()
    if err != nil { return err }
    defer db.Close()
    tx, _ := db.Begin()
    defer tx.Rollback()
    tx.Exec("DELETE FROM memory_paths")
    tx.Exec("DELETE FROM relations")
    tx.Exec("DELETE FROM memory_fts")
    tx.Exec("DELETE FROM memories")
    return tx.Commit()
}

func (idx *MemoryIndex) Upsert(card *memory.MemoryCard) error {
    db, err := idx.Connect()
    if err != nil { return err }
    defer db.Close()
    tx, _ := db.Begin()
    defer tx.Rollback()
    if err := doUpsert(tx, card); err != nil { return err }
    return tx.Commit()
}

func doUpsert(tx *sql.Tx, card *memory.MemoryCard) error {
    tagsJSON, _ := json.Marshal(card.Tags)
    modsJSON, _ := json.Marshal(card.Scope.Modules)
    pathsJSON, _ := json.Marshal(card.Scope.Paths)
    q :=`INSERT INTO memories(id,type,status,title,summary,source_kind,confidence,tags_json,modules_json,paths_json,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET type=excluded.type,status=excluded.status,title=excluded.title,summary=excluded.summary,source_kind=excluded.source_kind,confidence=excluded.confidence,tags_json=excluded.tags_json,modules_json=excluded.modules_json,paths_json=excluded.paths_json,updated_at=excluded.updated_at`
    _, err := tx.Exec(q, card.ID, card.Type, card.Status, card.Title, card.Summary, card.Source.Kind, card.Confidence, string(tagsJSON), string(modsJSON), string(pathsJSON), card.UpdatedAt)
    if err != nil { return fmt.Errorf("upsert: %w", err) }
    tx.Exec("DELETE FROM memory_fts WHERE id=?", card.ID)
    tx.Exec("INSERT INTO memory_fts(id,title,summary,content,tags,modules,paths) VALUES(?,?,?,?,?,?,?)", card.ID, card.Title, card.Summary, card.Content, strings.Join(card.Tags," "), strings.Join(card.Scope.Modules," "), strings.Join(card.Scope.Paths," "))
    tx.Exec("DELETE FROM memory_paths WHERE memory_id=?", card.ID)
    for _, p := range card.Scope.Paths { tx.Exec("INSERT OR IGNORE INTO memory_paths(memory_id,path) VALUES(?,?)", card.ID, p) }
    tx.Exec("DELETE FROM relations WHERE source_id=?", card.ID)
    for rel, targets := range card.Relations {
        for _, t := range targets { tx.Exec("INSERT OR IGNORE INTO relations(source_id,relation,target_id) VALUES(?,?,?)", card.ID, rel, t) }
    }
    return nil
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
    db, err := idx.Connect()
    if err != nil { return nil, err }
    defer db.Close()
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
    q := fmt.Sprintf("SELECT m.id,m.type,m.status,m.title,m.summary,m.source_kind,m.confidence,m.updated_at FROM memory_fts JOIN memories m ON m.id=memory_fts.id WHERE memory_fts MATCH ? AND %s ORDER BY rank ASC,m.updated_at DESC LIMIT ?", where)
    rows, err := db.Query(q, args...)
    if err != nil { return nil, fmt.Errorf("search: %w", err) }
    defer rows.Close()
    var results []map[string]any
    for rows.Next() {
        var id, tp, st, title, summary, sk, upd string
        var conf float64
        rows.Scan(&id,&tp,&st,&title,&summary,&sk,&conf,&upd)
        results = append(results, map[string]any{"id":id,"type":tp,"status":st,"title":title,"summary":summary,"confidence":conf,"source_hint":sk,"matched_by":[]string{"fts"},"updated_at":upd})
    }
    return results, rows.Err()
}

func (idx *MemoryIndex) Recent(limit int) ([]map[string]any, error) {
    if err := idx.Initialize(); err != nil { return nil, err }
    db, err := idx.Connect()
    if err != nil { return nil, err }
    defer db.Close()
    rows, err := db.Query("SELECT id,type,status,title,summary,source_kind,confidence,updated_at FROM memories ORDER BY updated_at DESC LIMIT ?", limit)
    if err != nil { return nil, fmt.Errorf("recent: %w", err) }
    defer rows.Close()
    var results []map[string]any
    for rows.Next() {
        var id, tp, st, title, summary, sk, upd string
        var conf float64
        rows.Scan(&id,&tp,&st,&title,&summary,&sk,&conf,&upd)
        results = append(results, map[string]any{"id":id,"type":tp,"status":st,"title":title,"summary":summary,"confidence":conf,"source_hint":sk,"matched_by":[]string{"recent"},"updated_at":upd})
    }
    return results, rows.Err()
}

func (idx *MemoryIndex) Rebuild() error {
    cards, err := store.DiscoverCards(idx.projectRoot)
    if err != nil { return fmt.Errorf("rebuild: %w", err) }
    idx.Initialize()
    idx.Clear()
    db, err := idx.Connect()
    if err != nil { return err }
    defer db.Close()
    tx, _ := db.Begin()
    defer tx.Rollback()
    for _, c := range cards {
        if err := doUpsert(tx, c); err != nil { return fmt.Errorf("rebuild %s: %w", c.ID, err) }
    }
    return tx.Commit()
}

func toFTSQuery(query string) string {
    terms := ftsTokenRe.FindAllString(query, -1)
    if len(terms) == 0 { return "" }
    quoted := make([]string, len(terms))
    for i, t := range terms { quoted[i] = "\"" + strings.ReplaceAll(t, "\"", "\"\"") + "\"" }
    return strings.Join(quoted, " ")
}
