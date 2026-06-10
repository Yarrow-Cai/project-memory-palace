package service

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/atop/project-memory-palace/internal/index"
	"github.com/atop/project-memory-palace/internal/memory"
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

func (s *MemoryService) InitProject() error {
	if s.initDone.Load() {
		return nil
	}
	if err := store.EnsureProjectMemory(s.projectRoot); err != nil {
		return fmt.Errorf("init project: %w", err)
	}
	if err := s.idx.Initialize(); err != nil {
		return err
	}
	s.initDone.Store(true)
	return nil
}

func (s *MemoryService) Remember(payload map[string]any) (map[string]any, error) {
	if err := s.InitProject(); err != nil {
		return nil, err
	}
	if err := memory.ValidatePayload(payload); err != nil {
		return nil, err
	}
	dateStr := time.Now().Format("2006-01-02")
	var lastErr error
	for attempt := 0; attempt < rememberIDAttempts; attempt++ {
		cardID, _, err := store.NextCardIdentity(s.projectRoot, dateStr)
		if err != nil {
			return nil, fmt.Errorf("remember: %w", err)
		}
		card := buildCard(cardID, payload)
		path, err := store.WriteCard(s.projectRoot, &card, false)
		if err != nil {
			lastErr = err
			continue
		}
		if err := s.idx.Upsert(&card); err != nil {
			store.RemoveCard(path)
			return nil, fmt.Errorf("remember: index error: %w", err)
		}
		result := cardToMap(&card)
		result["path"] = path
		result["notification"] = buildNotification(&card)
		return result, nil
	}
	return nil, fmt.Errorf("remember: failed after %d attempts: %w", rememberIDAttempts, lastErr)
}

func (s *MemoryService) Recall(query string, filters map[string]any, limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil {
		return nil, err
	}
	return s.idx.Search(query, filters, limit)
}

func (s *MemoryService) OpenMemory(memoryID string) (map[string]any, error) {
	if err := store.AssertMemoryLayout(s.projectRoot); err != nil {
		return nil, err
	}
	cards, err := store.DiscoverCards(s.projectRoot)
	if err != nil {
		return nil, err
	}
	for _, card := range cards {
		if card.ID == memoryID {
			return cardToMap(card), nil
		}
	}
	return nil, &MemoryNotFoundError{ID: memoryID}
}

func (s *MemoryService) ListRecent(limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil {
		return nil, err
	}
	return s.idx.Recent(limit)
}

func (s *MemoryService) UpdateMemory(memoryID string, updates map[string]any) (map[string]any, error) {
	existing, err := s.OpenMemory(memoryID)
	if err != nil {
		return nil, err
	}
	if len(updates) == 0 {
		return existing, nil
	}
	changed := false
	if status, ok := updates["status"].(string); ok {
		if !memory.MemoryStatuses[status] {
			return nil, fmt.Errorf("invalid status: %s", status)
		}
		if status != existing["status"] {
			changed = true
			existing["status"] = status
		}
	}
	if conf, ok := updates["confidence"]; ok {
		f, ok := toFloat64(conf)
		if !ok || f < 0 || f > 1 {
			return nil, fmt.Errorf("confidence must be 0.0-1.0")
		}
		if f != existing["confidence"] {
			changed = true
			existing["confidence"] = f
		}
	}
	if tags, ok := updates["tags"]; ok {
		l, err := toStringList(tags)
		if err != nil {
			return nil, fmt.Errorf("tags: %w", err)
		}
		changed = true
		existing["tags"] = l
	}
	if rels, ok := updates["relations"].(map[string]any); ok {
		cur, _ := existing["relations"].(map[string][]string)
		if cur == nil {
			cur = make(map[string][]string)
		}
		for k, v := range rels {
			targets := toStringSlice(v)
			for _, t := range targets {
				if !has(cur[k], t) {
					cur[k] = append(cur[k], t)
					changed = true
				}
			}
		}
		existing["relations"] = cur
	}
	if !changed {
		return existing, nil
	}
	existing["updated_at"] = now()
	card := mapToCard(existing)
	if _, err := store.WriteCard(s.projectRoot, card, true); err != nil {
		return nil, err
	}
	if err := s.idx.Upsert(card); err != nil {
		return nil, err
	}
	return cardToMap(card), nil
}

func (s *MemoryService) RebuildIndex() error { return s.idx.Rebuild() }

func has(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
