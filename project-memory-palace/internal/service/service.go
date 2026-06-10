package service

import (
	"fmt"
	"time"

	"github.com/atop/project-memory-palace/internal/index"
	"github.com/atop/project-memory-palace/internal/memory"
	"github.com/atop/project-memory-palace/internal/store"
)

// MemoryService provides the core business logic: create, search, open,
// update, and list memory cards.  It coordinates the YAML card store and
// the SQLite search index.
type MemoryService struct {
	projectRoot string
	idx         *index.MemoryIndex
}

// New returns an uninitialised MemoryService.  Call InitProject before
// any data operation.
func New(projectRoot string) *MemoryService {
	return &MemoryService{
		projectRoot: projectRoot,
		idx:         index.NewMemoryIndex(projectRoot),
	}
}

// ProjectRoot returns the absolute project root that this service operates on.
func (s *MemoryService) ProjectRoot() string { return s.projectRoot }

// InitProject creates the .project-memory directory tree with default
// config/rules files and initialises the SQLite search index.
func (s *MemoryService) InitProject() error {
	if err := store.EnsureProjectMemory(s.projectRoot); err != nil {
		return fmt.Errorf("init project: %w", err)
	}
	return s.idx.Initialize()
}

// Remember creates a new memory card from a payload map.  Required fields are
// type, title, summary, content (checked via memory.ValidatePayload).  The
// function generates a unique ID with a 3-attempt write loop, writes the
// YAML atomically, and upserts the card into the search index.
//
// If the index upsert fails the already-written card file is removed
// (rollback).
func (s *MemoryService) Remember(payload map[string]any) (map[string]any, error) {
	if err := memory.ValidatePayload(payload); err != nil {
		return nil, fmt.Errorf("validate payload: %w", err)
	}

	card := buildCard(payload)

	// 3-attempt ID / write loop.
	dateStr := time.Now().Format("2006-01-02")
	var writtenPath string
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		id, _, err := store.NextCardIdentity(s.projectRoot, dateStr)
		if err != nil {
			return nil, fmt.Errorf("next identity: %w", err)
		}
		card.ID = id

		writtenPath, err = store.WriteCard(s.projectRoot, &card, false)
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err
	}
	if writtenPath == "" {
		return nil, fmt.Errorf("write card after 3 attempts: %w", lastErr)
	}

	// Upsert index, rollback card on failure.
	if err := s.idx.Upsert(&card); err != nil {
		store.RemoveCard(writtenPath)
		return nil, fmt.Errorf("index upsert: %w", err)
	}

	return cardToMap(&card), nil
}

// Recall searches the index for memory summaries matching query via LIKE on
// title and summary.  limit <= 0 is capped at 20.
func (s *MemoryService) Recall(query string, filters map[string]any, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.idx.Search(query, filters, limit)
}

// OpenMemory returns the full memory card data for the given ID.  It scans
// YAML cards on disk (not the index).
func (s *MemoryService) OpenMemory(memoryID string) (map[string]any, error) {
	cards, err := store.DiscoverCards(s.projectRoot)
	if err != nil {
		return nil, fmt.Errorf("discover cards: %w", err)
	}
	for _, c := range cards {
		if c.ID == memoryID {
			return cardToMap(c), nil
		}
	}
	return nil, &MemoryNotFoundError{ID: memoryID}
}

// ListRecent returns the most recently updated memory summaries from the
// index.  limit <= 0 is capped at 20.
func (s *MemoryService) ListRecent(limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.idx.Recent(limit)
}

// UpdateMemory applies a set of allowed update fields (status, confidence,
// tags, relations) to an existing memory card, rewrites the YAML, and
// upserts the index.
func (s *MemoryService) UpdateMemory(memoryID string, updates map[string]any) (map[string]any, error) {
	cards, err := store.DiscoverCards(s.projectRoot)
	if err != nil {
		return nil, fmt.Errorf("discover cards: %w", err)
	}
	var card *memory.MemoryCard
	for _, c := range cards {
		if c.ID == memoryID {
			card = c
			break
		}
	}
	if card == nil {
		return nil, &MemoryNotFoundError{ID: memoryID}
	}

	// Validate and apply each update key.
	for k, v := range updates {
		if !memory.UpdateAllowedFields[k] {
			return nil, fmt.Errorf("invalid update field: %s", k)
		}
		switch k {
		case "status":
			sVal, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("status must be a string")
			}
			if !memory.MemoryStatuses[sVal] {
				return nil, fmt.Errorf("invalid status: %s", sVal)
			}
			card.Status = sVal
		case "confidence":
			f, ok := toFloat64(v)
			if !ok {
				return nil, fmt.Errorf("confidence must be a number")
			}
			if f < 0.0 || f > 1.0 {
				return nil, fmt.Errorf("confidence must be between 0.0 and 1.0")
			}
			card.Confidence = f
		case "tags":
			card.Tags = toStringList(v)
		case "relations":
			relMap, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("relations must be a map")
			}
			newRels := make(map[string][]string)
			for rk, rv := range relMap {
				newRels[rk] = toStringList(rv)
			}
			card.Relations = newRels
		}
	}

	card.UpdatedAt = time.Now().Format(time.RFC3339)

	if _, err := store.WriteCard(s.projectRoot, card, true); err != nil {
		return nil, fmt.Errorf("write card: %w", err)
	}

	// card is already *memory.MemoryCard, pass directly.
	if err := s.idx.Upsert(card); err != nil {
		return nil, fmt.Errorf("index upsert: %w", err)
	}

	return cardToMap(card), nil
}

// RebuildIndex drops and recreates the SQLite index from the YAML cards on
// disk.
func (s *MemoryService) RebuildIndex() error {
	return s.idx.Rebuild()
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// buildCard constructs a MemoryCard from a raw payload map.  Fields not
// present in the payload are left at their defaults (from memory.NewCard).
// Confidence is capped at MaxConfidenceNoSource when no source object is
// supplied.
func buildCard(payload map[string]any) memory.MemoryCard {
	typ, _ := payload["type"].(string)
	title, _ := payload["title"].(string)
	summary, _ := payload["summary"].(string)
	content, _ := payload["content"].(string)

	confidence := memory.DefaultConfidence
	if v, ok := toFloat64(payload["confidence"]); ok {
		confidence = v
	}
	if _, hasSource := payload["source"]; !hasSource {
		if confidence > memory.MaxConfidenceNoSource {
			confidence = memory.MaxConfidenceNoSource
		}
	}

	card := memory.NewCard(typ, title, summary, content, confidence)

	// Source override.
	if src, ok := payload["source"].(map[string]any); ok {
		if k, ok := src["kind"].(string); ok && memory.SourceKinds[k] {
			card.Source.Kind = k
		}
		if d, ok := src["description"].(string); ok {
			card.Source.Description = d
		}
	}

	// Scope.
	if sc, ok := payload["scope"].(map[string]any); ok {
		if p, ok := sc["project"].(string); ok {
			card.Scope.Project = p
		}
		if mods, ok := sc["modules"]; ok {
			card.Scope.Modules = toStringList(mods)
		}
		if paths, ok := sc["paths"]; ok {
			card.Scope.Paths = toStringList(paths)
		}
	}

	// Tags.
	if tags, ok := payload["tags"]; ok {
		card.Tags = toStringList(tags)
	}

	// Relations.
	if rels, ok := payload["relations"]; ok {
		if relMap, ok := rels.(map[string]any); ok {
			out := make(map[string][]string)
			for rk, rv := range relMap {
				out[rk] = toStringList(rv)
			}
			card.Relations = out
		}
	}

	return card
}

// cardToMap serialises a MemoryCard into a map suitable for JSON / API
// responses.
func cardToMap(card *memory.MemoryCard) map[string]any {
	return map[string]any{
		"schema_version": card.SchemaVersion,
		"id":             card.ID,
		"type":           card.Type,
		"status":         card.Status,
		"confidence":     card.Confidence,
		"title":          card.Title,
		"summary":        card.Summary,
		"content":        card.Content,
		"source": map[string]any{
			"kind":        card.Source.Kind,
			"description": card.Source.Description,
			"files":       card.Source.Files,
			"commits":     card.Source.Commits,
		},
		"scope": map[string]any{
			"project": card.Scope.Project,
			"modules": card.Scope.Modules,
			"paths":   card.Scope.Paths,
		},
		"tags":      card.Tags,
		"relations": card.Relations,
		"created_at": card.CreatedAt,
		"updated_at": card.UpdatedAt,
	}
}

// mapToCard converts a generic map back into a MemoryCard.  Useful in tests
// and for deserialising persisted representations.
func mapToCard(data map[string]any) memory.MemoryCard {
	card := memory.MemoryCard{}
	if v, ok := toFloat64(data["schema_version"]); ok {
		card.SchemaVersion = int(v)
	}
	if s, ok := data["id"].(string); ok {
		card.ID = s
	}
	if s, ok := data["type"].(string); ok {
		card.Type = s
	}
	if s, ok := data["status"].(string); ok {
		card.Status = s
	}
	if v, ok := toFloat64(data["confidence"]); ok {
		card.Confidence = v
	}
	if s, ok := data["title"].(string); ok {
		card.Title = s
	}
	if s, ok := data["summary"].(string); ok {
		card.Summary = s
	}
	if s, ok := data["content"].(string); ok {
		card.Content = s
	}
	if src, ok := data["source"].(map[string]any); ok {
		if s, ok := src["kind"].(string); ok {
			card.Source.Kind = s
		}
		if s, ok := src["description"].(string); ok {
			card.Source.Description = s
		}
		card.Source.Files = toStringList(src["files"])
		card.Source.Commits = toStringList(src["commits"])
	}
	if scope, ok := data["scope"].(map[string]any); ok {
		if s, ok := scope["project"].(string); ok {
			card.Scope.Project = s
		}
		card.Scope.Modules = toStringList(scope["modules"])
		card.Scope.Paths = toStringList(scope["paths"])
	}
	card.Tags = toStringList(data["tags"])
	if rels, ok := data["relations"].(map[string]any); ok {
		out := make(map[string][]string)
		for rk, rv := range rels {
			out[rk] = toStringList(rv)
		}
		card.Relations = out
	}
	if s, ok := data["created_at"].(string); ok {
		card.CreatedAt = s
	}
	if s, ok := data["updated_at"].(string); ok {
		card.UpdatedAt = s
	}
	return card
}

// buildNotification creates a human-readable notification string for a
// remembered card.
func buildNotification(card *memory.MemoryCard) string {
	return fmt.Sprintf("Remembered %s '%s' [%s]", card.Type, card.Title, card.ID)
}

// --------------------------------------------------------------------------
// Type-conversion helpers
// --------------------------------------------------------------------------

// toFloat64 attempts to cast v to float64.  Accepts float64, int, and int64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// toStringList converts a []string or []any to []string.
func toStringList(v any) []string {
	switch list := v.(type) {
	case []string:
		return list
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// anyToStringSlice is an alias for toStringList kept for readability when
// the input is untyped.
func anyToStringSlice(raw any) []string {
	return toStringList(raw)
}