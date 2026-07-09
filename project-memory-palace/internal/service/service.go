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
func (s *MemoryService) ListTemplates() ([]string, error) {
	return store.ListTemplates(s.projectRoot)
}

// Close closes the underlying index (and its SQLite connection).
// After Close, the service must not be used without reinitialization.
func (s *MemoryService) Close() error { return s.idx.Close() }

// Vacuum reclaims unused disk space from the underlying SQLite database.
func (s *MemoryService) Vacuum() error {
	return s.idx.Vacuum()
}

func (s *MemoryService) InitProject() error {
	if s.initDone.Load() { return nil }
	if err := store.EnsureProjectMemory(s.projectRoot); err != nil { return fmt.Errorf("init project: %w", err) }
	if err := s.idx.Initialize(); err != nil { return err }
	s.initDone.Store(true)
	return nil
}

// validateRelationTargets checks that all relation targets exist in the index.
// Returns nil if there are no relations or all targets exist.
func (s *MemoryService) validateRelationTargets(card *memory.MemoryCard) error {
	if len(card.Relations) == 0 {
		return nil
	}
	var missing []string
	for _, targets := range card.Relations {
		for _, target := range targets {
			meta, _ := s.idx.GetMemory(target)
			if meta == nil {
				missing = append(missing, target)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("relation targets not found: %v", missing)
	}
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
		// Template support: load template and merge defaults (payload overrides template)
		if tmplName, ok := payload["template"].(string); ok && tmplName != "" {
			tmpl, err := store.LoadTemplate(s.projectRoot, tmplName)
			if err != nil {
				return nil, fmt.Errorf("template: %w", err)
			}
			for k, v := range tmpl {
				if _, exists := payload[k]; !exists {
					payload[k] = v
				}
			}
			delete(payload, "template")
		}
		card := buildCard(cardID, payload)
		if err := s.validateRelationTargets(&card); err != nil { return nil, err }
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

func (s *MemoryService) RememberBatch(payloads []map[string]any) (map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	var results []map[string]any
	var errors []map[string]any
	dateStr := time.Now().Format("2006-01-02")
	for i, payload := range payloads {
		if err := memory.ValidatePayload(payload); err != nil {
			errors = append(errors, map[string]any{"index": i, "error": err.Error()})
			continue
		}
		var lastErr error
		for attempt := 0; attempt < rememberIDAttempts; attempt++ {
			cardID, _, err := store.NextCardIdentity(s.projectRoot, dateStr)
			if err != nil { lastErr = err; continue }
			card := buildCard(cardID, payload)
			if err := s.validateRelationTargets(&card); err != nil { lastErr = err; continue }
			path, err := store.WriteCard(s.projectRoot, &card, false)
			if err != nil { lastErr = err; continue }
			if err := s.idx.Upsert(&card); err != nil { store.RemoveCard(path); lastErr = err; continue }
			results = append(results, map[string]any{"index": i, "id": cardID, "status": "created"})
			break
		}
		if lastErr != nil {
			errors = append(errors, map[string]any{"index": i, "error": lastErr.Error()})
		}
	}
	return map[string]any{"created": results, "errors": errors, "total": len(payloads), "success": len(results)}, nil
}

func (s *MemoryService) RecallBatch(ids []string) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	var results []map[string]any
	for _, id := range ids {
		card, err := s.OpenMemory(id)
		if err != nil {
			results = append(results, map[string]any{"id": id, "error": err.Error()})
		} else {
			results = append(results, card)
		}
	}
	if len(results) > 0 {
		_ = s.idx.RecordAccess(extractIDs(results))
	}
	return results, nil
}


func (s *MemoryService) Recall(query string, filters map[string]any, limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	results, err := s.idx.Search(query, filters, limit)
	if err != nil { return nil, err }
	if len(results) > 0 {
		_ = s.idx.RecordAccess(extractIDs(results))
	}
	projectName := filepath.Base(s.projectRoot)
	for _, r := range results {
		r["project"] = projectName
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
	
	// Advance access tracking, then merge v0.6 fields from SQLite
	nowStr := now()
	_ = s.idx.RecordAccess([]string{cardObj.ID})
	result["access_count"] = 1
	if ac, ok := meta["access_count"].(int); ok {
		result["access_count"] = ac + 1
	}
	result["last_accessed_at"] = nowStr
	result["effective_priority"] = index.EffectivePriority(cardObj.Priority, nowStr)
	
	// Merge source_agent and knowledge_kind from SQLite if YAML had empty values
	if sa, ok := meta["source_agent"].(string); ok && sa != "" && result["source_agent"] == "" {
		result["source_agent"] = sa
	}
	if kk, ok := meta["knowledge_kind"].(string); ok && kk != "" && result["knowledge_kind"] == "" {
		result["knowledge_kind"] = kk
	}
	
	result["project"] = filepath.Base(s.projectRoot)
	return result, nil
}

func (s *MemoryService) ListRecent(limit, offset int, filters map[string]any) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	results, err := s.idx.Recent(limit, offset, filters)
	if err != nil { return nil, err }
	if len(results) > 0 {
		_ = s.idx.RecordAccess(extractIDs(results))
	}
	projectName := filepath.Base(s.projectRoot)
	for _, r := range results {
		r["project"] = projectName
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
	// Capture old filename for version history before modifying
	oldFilename := store.CardFilename(mapToCard(existing))
	changed := false
	if status, ok := updates["status"].(string); ok {
		if !memory.MemoryStatuses[status] { return nil, fmt.Errorf("invalid status: %s", status) }
		if status != existing["status"] { changed = true; existing["status"] = status }
	}
	if conf, ok := updates["confidence"]; ok {
		f, ok := toFloat64(conf)
		if !ok || f < 0 || f > 1 { return nil, fmt.Errorf("confidence must be 0.0-1.0") }
		if f != existing["confidence"] { changed = true; existing["confidence"] = f }
		// Re-check confidence cap: if source was not explicitly provided, cap at 0.5.
		// A card created without source gets default kind="analysis" and a fixed description.
		// We detect this by checking for the default description string.
		if srcMap, ok := existing["source"].(map[string]any); ok && srcMap != nil {
			desc, _ := srcMap["description"].(string)
			kind, _ := srcMap["kind"].(string)
			files, _ := srcMap["files"].([]string)
			commits, _ := srcMap["commits"].([]string)
			hasFiles := len(files) > 0
			hasCommits := len(commits) > 0
			// Default source from NewCard: kind="analysis", description="Source was not supplied by caller."
			// If source matches this default, cap confidence.
			if kind == "analysis" && desc == "Source was not supplied by caller." && !hasFiles && !hasCommits {
				maxConf := 0.5
				if c, ok := existing["confidence"].(float64); ok && c > maxConf {
					existing["confidence"] = maxConf
				}
			}
		}
	}
	if pr, ok := updates["priority"]; ok {
		p, ok := toFloat64(pr)
		if !ok || p < 1 || p > 5 { return nil, fmt.Errorf("priority must be 1-5") }
		if int(p) != existing["priority"] { changed = true; existing["priority"] = int(p) }
	}
	if tags, ok := updates["tags"]; ok {
		l, err := toStringList(tags)
		if err != nil { return nil, fmt.Errorf("tags: %w", err) }
		if !stringSlicesEqual(existing["tags"], l) { changed = true; existing["tags"] = l }
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
	if sa, ok := updates["source_agent"].(string); ok {
		if sa != existing["source_agent"] { changed = true; existing["source_agent"] = sa }
	}
	if kk, ok := updates["knowledge_kind"].(string); ok {
		if kk != existing["knowledge_kind"] { changed = true; existing["knowledge_kind"] = kk }
	}
	if vb, ok := updates["verified_by"].(string); ok {
		if vb != existing["verified_by"] { changed = true; existing["verified_by"] = vb }
	}
	if ob, ok := updates["outdated_by"].(string); ok {
		if ob != existing["outdated_by"] { changed = true; existing["outdated_by"] = ob }
	}
	if !changed { return existing, nil }
	existing["updated_at"] = now()
card := mapToCard(existing)
	if err := s.validateRelationTargets(card); err != nil { return nil, err }
	// Save version history before overwriting
	if oldFilename != "" {
		_ = store.SaveHistory(s.projectRoot, oldFilename)
	}
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


func (s *MemoryService) ExportCards() ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	cards, err := store.DiscoverCards(s.projectRoot)
	if err != nil { return nil, err }
	var results []map[string]any
	for _, c := range cards {
		results = append(results, cardToMap(c))
	}
	return results, nil
}

func (s *MemoryService) VerifyIntegrity() (map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	cards, err := store.DiscoverCards(s.projectRoot)
	if err != nil { return nil, err }

	yamlIDs := map[string]bool{}
	var orphansYAML []string
	var mismatches []map[string]any

	for _, c := range cards {
		yamlIDs[c.ID] = true
		meta, err := s.idx.GetMemory(c.ID)
		if err != nil || meta == nil {
			orphansYAML = append(orphansYAML, c.ID)
			continue
		}
		// Check key fields match
		if meta["title"] != c.Title {
			mismatches = append(mismatches, map[string]any{"id": c.ID, "field": "title", "yaml": c.Title, "sqlite": meta["title"]})
		}
		if meta["type"] != c.Type {
			mismatches = append(mismatches, map[string]any{"id": c.ID, "field": "type", "yaml": c.Type, "sqlite": meta["type"]})
		}
	}

	// Find SQLite-only orphans
	allRecent, _ := s.idx.Recent(10000, 0, nil)
	var orphansSQLite []string
	for _, r := range allRecent {
		id, _ := r["id"].(string)
		if !yamlIDs[id] {
			orphansSQLite = append(orphansSQLite, id)
		}
	}

	return map[string]any{
		"total_yaml":      len(cards),
		"total_sqlite":    len(allRecent),
		"orphans_yaml":    orphansYAML,
		"orphans_sqlite":  orphansSQLite,
		"mismatches":      mismatches,
		"healthy":         len(orphansYAML) == 0 && len(orphansSQLite) == 0 && len(mismatches) == 0,
	}, nil
}

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
	results, err := s.idx.SearchByPaths(paths, limit)
	if err != nil {
		return nil, err
	}
	projectName := filepath.Base(s.projectRoot)
	for _, r := range results {
		r["project"] = projectName
	}
	return results, nil
}

// HotMemories returns active memories sorted by access count descending.
func (s *MemoryService) HotMemories(limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	return s.idx.HotMemories(limit)
}

// Disclosure returns memories using progressive disclosure strategy.
// mode="first": priority>=3 active, limit 20.
// mode="subsequent": priority>=5 + recently changed, deduped, limit 15.
// When since is non-empty in subsequent mode, all cards are filtered to those updated after since.
func (s *MemoryService) Disclosure(mode, since string) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil {
		return nil, err
	}
	switch mode {
	case "first":
		return s.ListRecent(20, 0, map[string]any{"status": "active", "priority": 3})
	case "subsequent":
		highPri, err := s.ListRecent(15, 0, map[string]any{"status": "active", "priority": 5})
		if err != nil {
			return nil, err
		}
		recent, err := s.ListRecent(15, 0, map[string]any{"status": "active"})
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		var results []map[string]any
		for _, r := range highPri {
			if since == "" || (r["updated_at"] != nil && IsAfterTime(fmt.Sprint(r["updated_at"]), since)) {
				seen[r["id"].(string)] = true
				results = append(results, r)
			}
		}
		for _, r := range recent {
			if !seen[r["id"].(string)] {
				if since == "" || (r["updated_at"] != nil && IsAfterTime(fmt.Sprint(r["updated_at"]), since)) {
					results = append(results, r)
				}
			}
		}
		if len(results) > 15 {
			results = results[:15]
		}
		return results, nil
	default:
		return nil, fmt.Errorf("mode must be 'first' or 'subsequent'")
	}
}

// GetRelations returns relations data for a given card, with optional graph traversal.
func (s *MemoryService) GetRelations(id string, direction string, depth int) (map[string]any, error) {
	if err := s.InitProject(); err != nil {
		return nil, err
	}
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	// Verify card exists
	meta, err := s.idx.GetMemory(id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, fmt.Errorf("card not found: %s", id)
	}

	// Get outgoing relations from the index (relations table)
	outgoing, err := s.idx.GetRelations(id)
	if err != nil {
		return nil, err
	}

	// Get referrers (cards pointing to this one)
	referrerIDs, err := s.idx.FindReferrers(id)
	if err != nil {
		return nil, err
	}

	// Build incoming relations map from referrers
	incoming := make(map[string][]string)
	if direction == "incoming" || direction == "both" || direction == "" {
		for _, refID := range referrerIDs {
			refMeta, _ := s.idx.GetMemory(refID)
			if refMeta == nil {
				continue
			}
			refRels, _ := s.idx.GetRelations(refID)
			for relKind, targets := range refRels {
				for _, target := range targets {
					if target == id {
						incoming[relKind] = append(incoming[relKind], refID)
					}
				}
			}
		}
	}

	result := map[string]any{
		"id":       id,
		"title":    meta["title"],
		"outgoing": outgoing,
		"incoming": incoming,
	}

	// Populate card titles for relation targets
	cardTitles := make(map[string]string)
	cardTitles[id] = fmt.Sprint(meta["title"])
	for _, targets := range outgoing {
		for _, tid := range targets {
			if _, ok := cardTitles[tid]; !ok {
				if tm, _ := s.idx.GetMemory(tid); tm != nil {
					cardTitles[tid] = fmt.Sprint(tm["title"])
				}
			}
		}
	}
	for _, refIDs := range incoming {
		for _, rid := range refIDs {
			if _, ok := cardTitles[rid]; !ok {
				if rm, _ := s.idx.GetMemory(rid); rm != nil {
					cardTitles[rid] = fmt.Sprint(rm["title"])
				}
			}
		}
	}
	result["card_titles"] = cardTitles

	// Graph traversal for depth > 1
	if depth > 1 {
		graph := s.traverseRelations(id, direction, depth)
		result["graph"] = graph
	}

	return result, nil
}

// traverseRelations performs BFS graph traversal from a starting card.
func (s *MemoryService) traverseRelations(startID, direction string, maxDepth int) map[string]any {
	visited := map[string]bool{startID: true}
	nodes := []map[string]any{}
	edges := []map[string]any{}

	type item struct {
		id    string
		depth int
	}
	queue := []item{{id: startID, depth: 0}}

	// Add start node
	if m, _ := s.idx.GetMemory(startID); m != nil {
		nodes = append(nodes, map[string]any{
			"id":    startID,
			"title": m["title"],
			"type":  m["type"],
			"depth": 0,
		})
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= maxDepth {
			continue
		}

		// Outgoing
		if direction == "outgoing" || direction == "both" || direction == "" {
			rels, _ := s.idx.GetRelations(current.id)
			for relKind, targets := range rels {
				for _, tid := range targets {
					if visited[tid] {
						edges = append(edges, map[string]any{
							"from": current.id, "to": tid, "relation": relKind,
							"direction": "outgoing",
						})
						continue
					}
					visited[tid] = true
					tm, _ := s.idx.GetMemory(tid)
					title := ""
					tp := ""
					if tm != nil {
						title = fmt.Sprint(tm["title"])
						tp = fmt.Sprint(tm["type"])
					}
					nodes = append(nodes, map[string]any{
						"id":    tid,
						"title": title,
						"type":  tp,
						"depth": current.depth + 1,
					})
					edges = append(edges, map[string]any{
						"from": current.id, "to": tid, "relation": relKind,
						"direction": "outgoing",
					})
					queue = append(queue, item{id: tid, depth: current.depth + 1})
				}
			}
		}

		// Incoming
		if direction == "incoming" || direction == "both" {
			refs, _ := s.idx.FindReferrers(current.id)
			for _, refID := range refs {
				if visited[refID] {
					// Add edge even if visited
					refRels, _ := s.idx.GetRelations(refID)
					for rk, targets := range refRels {
						for _, t := range targets {
							if t == current.id {
								edges = append(edges, map[string]any{
									"from": refID, "to": current.id, "relation": rk,
									"direction": "incoming",
								})
							}
						}
					}
					continue
				}
				visited[refID] = true
				rm, _ := s.idx.GetMemory(refID)
				title := ""
				tp := ""
				if rm != nil {
					title = fmt.Sprint(rm["title"])
					tp = fmt.Sprint(rm["type"])
				}
				nodes = append(nodes, map[string]any{
					"id":    refID,
					"title": title,
					"type":  tp,
					"depth": current.depth + 1,
				})
				// Find which relation kind was used
				refRels, _ := s.idx.GetRelations(refID)
				relKind := "related_to"
				for rk, targets := range refRels {
					for _, t := range targets {
						if t == current.id {
							relKind = rk
						}
					}
				}
				edges = append(edges, map[string]any{
					"from": refID, "to": current.id, "relation": relKind,
					"direction": "incoming",
				})
				queue = append(queue, item{id: refID, depth: current.depth + 1})
			}
		}
	}

	return map[string]any{
		"nodes": nodes,
		"edges": edges,
	}
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

func (s *MemoryService) DecayMemories(limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	return s.idx.DecayAnalysis(limit)
}

// ListChangesSince returns active cards updated after the given timestamp.
func (s *MemoryService) ListChangesSince(since string, limit int) ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	return s.idx.ListChangesSince(since, limit)
}

// CoverageStats returns per-module card counts for coverage analysis.
func (s *MemoryService) CoverageStats() ([]map[string]any, error) {
	if err := s.InitProject(); err != nil { return nil, err }
	return s.idx.CoverageStats()
}

func has(slice []string, item string) bool {
	for _, s := range slice { if s == item { return true } }
	return false
}
func stringSlicesEqual(a any, b []string) bool {
	existing, ok := a.([]string)
	if !ok { return true }
	if len(existing) != len(b) { return false }
	for i := range existing {
		if existing[i] != b[i] { return false }
	}
	return true
}
