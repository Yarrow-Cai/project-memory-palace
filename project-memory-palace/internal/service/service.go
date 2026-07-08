package service

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/atop/project-memory-palace/internal/index"
	"github.com/atop/project-memory-palace/internal/memory"
	"github.com/atop/project-memory-palace/internal/rule"
	"github.com/atop/project-memory-palace/internal/store"
)

const rememberIDAttempts = 3

type MemoryService struct {
	projectRoot string
	idx         *index.MemoryIndex
	initDone    atomic.Bool
}

func New(projectRoot string) *MemoryService {
	return &MemoryService{projectRoot: projectRoot, idx: index.NewMemoryIndex(projectRoot)}
}

func (s *MemoryService) ProjectRoot() string { return s.projectRoot }

// Close closes the underlying index (and its SQLite connection).
// After Close, the service must not be used without reinitialization.
func (s *MemoryService) Close() error { return s.idx.Close() }

func (s *MemoryService) InitProject() error {
	if s.initDone.Load() { return nil }
	if err := store.EnsureProjectMemory(s.projectRoot); err != nil { return fmt.Errorf("init project: %w", err) }
	if err := s.idx.Initialize(); err != nil { return err }
	s.initDone.Store(true)
	return nil
}

func (s *MemoryService) Remember(payload map[string]any) (map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	if err := memory.ValidatePayload(payload); err != nil { return nil, err }
	dateStr := time.Now().Format("2006-01-02")
	var lastErr error
	for attempt := 0; attempt < rememberIDAttempts; attempt++ {
		cardID, _, err := store.NextCardIdentity(s.projectRoot, dateStr)
		if err != nil { return nil, fmt.Errorf("remember: %w", err) }
		card := buildCard(cardID, payload)
		path, err := store.WriteCard(s.projectRoot, &card, false)
		if err != nil { lastErr = err; continue }
		if err := s.idx.Upsert(&card); err != nil { store.RemoveCard(path); return nil, fmt.Errorf("remember: index error: %w", err) }
		result := cardToMap(&card)
		result["path"] = path
		result["notification"] = buildNotification(&card)
		return result, nil
	}
	return nil, fmt.Errorf("remember: failed after %d attempts: %w", rememberIDAttempts, lastErr)
}

func (s *MemoryService) Recall(query string, filters map[string]any, limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	results, err := s.idx.Search(query, filters, limit)
	if err != nil { return nil, err }
	if len(results) > 0 {
		_ = s.idx.RecordAccess(extractIDs(results))
	}
	return results, nil
}

func (s *MemoryService) OpenMemory(memoryID string) (map[string]any, error) {
	// Fast path: check SQLite index first (O(1)), then read single YAML file
	meta, _ := s.idx.GetMemory(memoryID)
	if meta == nil {
		return nil, &MemoryNotFoundError{ID: memoryID}
	}
	card := &memory.MemoryCard{
		ID:   memoryID,
		Type: meta["type"].(string),
	}
	filename := store.CardFilename(card)
	if filename == "" {
		return nil, &MemoryNotFoundError{ID: memoryID}
	}
	filePath := filepath.Join(store.CardsDir(s.projectRoot), filename)
	cardObj, err := store.ReadCard(filePath)
	if err != nil {
		return nil, &MemoryNotFoundError{ID: memoryID}
	}
	result := cardToMap(cardObj)
	_ = s.idx.RecordAccess([]string{cardObj.ID})
	return result, nil
}

func (s *MemoryService) ListRecent(limit, offset int, filters map[string]any) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	results, err := s.idx.Recent(limit, offset, filters)
	if err != nil { return nil, err }
	if len(results) > 0 {
		_ = s.idx.RecordAccess(extractIDs(results))
	}
	return results, nil
}

func (s *MemoryService) Count(filters map[string]any) (int, error) {
	if err := s.InitProject(); err != nil { return 0, err }
	return s.idx.Count(filters)
}

func (s *MemoryService) UpdateMemory(memoryID string, updates map[string]any) (map[string]any, error) {
	existing, err := s.OpenMemory(memoryID)
	if err != nil { return nil, err }
	if len(updates) == 0 { return existing, nil }
	changed := false
	if status, ok := updates["status"].(string); ok {
		if !memory.MemoryStatuses[status] { return nil, fmt.Errorf("invalid status: %s", status) }
		if status != existing["status"] { changed = true; existing["status"] = status }
	}
	if conf, ok := updates["confidence"]; ok {
		f, ok := toFloat64(conf)
		if !ok || f < 0 || f > 1 { return nil, fmt.Errorf("confidence must be 0.0-1.0") }
		if f != existing["confidence"] { changed = true; existing["confidence"] = f }
	}
	if tags, ok := updates["tags"]; ok {
		l, err := toStringList(tags)
		if err != nil { return nil, fmt.Errorf("tags: %w", err) }
		changed = true; existing["tags"] = l
	}
	if rels, ok := updates["relations"].(map[string]any); ok {
		cur, _ := existing["relations"].(map[string][]string)
		if cur == nil { cur = make(map[string][]string) }
		for k, v := range rels {
			targets := toStringSlice(v)
			for _, t := range targets {
				if !has(cur[k], t) { cur[k] = append(cur[k], t); changed = true }
			}
		}
		existing["relations"] = cur
	}
	if exp, ok := updates["expires_at"].(string); ok {
		if exp != existing["expires_at"] { changed = true; existing["expires_at"] = exp }
	}
	if !changed { return existing, nil }
	existing["updated_at"] = now()
	card := mapToCard(existing)
	if _, err := store.WriteCard(s.projectRoot, card, true); err != nil { return nil, err }
	if err := s.idx.Upsert(card); err != nil { return nil, err }
	return cardToMap(card), nil
}

func (s *MemoryService) RebuildIndex() error { return s.idx.Rebuild() }

// DeleteMemory deletes a single memory card (YAML file + SQLite index).
func (s *MemoryService) DeleteMemory(id string) (map[string]any, error) {
	// Find the card to get its file path
	card, err := s.OpenMemory(id)
	if err != nil { return nil, err }
	cardObj := mapToCard(card)
	filename := store.CardFilename(cardObj)
	if filename == "" { return nil, fmt.Errorf("invalid card ID: %s", id) }
	filePath := filepath.Join(store.CardsDir(s.projectRoot), filename)
	// Delete YAML file
	if err := store.RemoveCard(filePath); err != nil { return nil, fmt.Errorf("remove card: %w", err) }
	// Delete from index
	if err := s.idx.Delete(id); err != nil { return nil, fmt.Errorf("delete index: %w", err) }
	return map[string]any{"deleted": id, "status": "ok"}, nil
}

// PurgeExpired deletes all memories with status "expired".
func (s *MemoryService) PurgeExpired() (map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	ids, err := s.idx.ListExpired()
	if err != nil { return nil, err }
	deleted := []string{}
	failed := []string{}
	for _, id := range ids {
		if _, err := s.DeleteMemory(id); err != nil {
			failed = append(failed, id)
		} else {
			deleted = append(deleted, id)
		}
	}
	return map[string]any{"deleted": deleted, "deleted_count": len(deleted), "failed": failed, "status": "ok"}, nil
}

func (s *MemoryService) SynthesizeRules() (*rule.RulesDocument, error) {
	return rule.Synthesize(s.projectRoot)
}

// ContextForFiles returns memories associated with the given file paths.
func (s *MemoryService) ContextForFiles(paths []string, limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	return s.idx.SearchByPaths(paths, limit)
}

// HotMemories returns active memories sorted by access count descending.
func (s *MemoryService) HotMemories(limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	return s.idx.HotMemories(limit)
}

func extractIDs(results []map[string]any) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		if id, ok := r["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func has(slice []string, item string) bool {
	for _, s := range slice { if s == item { return true } }
	return false
}

