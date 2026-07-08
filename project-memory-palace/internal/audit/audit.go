package audit

import (
	"strings"

	"github.com/atop/project-memory-palace/internal/store"
)

// AuditProject scans all YAML memory cards in the project and returns a
// report of issues found.  An empty slice means the project is clean.
//
// Checks performed:
//   - low_confidence           — confidence <= 0.5
//   - missing_tags             — no tags assigned
//   - missing_scope            — neither modules nor paths are set
//   - high_confidence_inference — analysis-sourced card with confidence > 0.7
//   - possible_duplicate       — another card with the same lowercased title
//   - stale                    — status is "stale"
//   - agent_conflict           — two active cards from different agents sharing a scope path
//   - topic_conflict           — two active cards from different agents with overlapping title words but no shared paths
//   - semantic_conflict        — two active cards explicitly linked by a "contradicts" relation
//   - scope_overlap            — two active cards from different agents sharing paths/modules with different knowledge_kind
func AuditProject(projectRoot string) ([]map[string]any, error) {
	cards, err := store.DiscoverCards(projectRoot)
	if err != nil {
		return nil, err
	}

	seenTitles := map[string]string{}
	var report []map[string]any

	// Collect cross-card data for conflict detection
	pathToCards := map[string][]string{}
	cardAgents := map[string]string{}
	cardActive := map[string]bool{}
	cardTitles := map[string]string{}
	cardPaths := map[string][]string{}
	cardModules := map[string][]string{}
	cardKnowledgeKind := map[string]string{}

	for _, card := range cards {
		var issues []string

		if card.Confidence <= 0.5 {
			issues = append(issues, "low_confidence")
		}
		if len(card.Tags) == 0 {
			issues = append(issues, "missing_tags")
		}
		if len(card.Scope.Modules) == 0 && len(card.Scope.Paths) == 0 {
			issues = append(issues, "missing_scope")
		}
		inferenceKinds := map[string]bool{"analysis": true, "experiment": true, "observation": true, "inference": true}
		if inferenceKinds[card.Source.Kind] && card.Confidence > 0.7 {
			issues = append(issues, "high_confidence_inference")
		}

		tk := strings.ToLower(strings.TrimSpace(card.Title))
		if existing, ok := seenTitles[tk]; ok {
			issues = append(issues, "possible_duplicate:"+existing)
		} else {
			seenTitles[tk] = card.ID
		}

		if card.Status == "stale" {
			issues = append(issues, "stale")
		}

		// Collect for cross-card conflict analysis
		for _, p := range card.Scope.Paths {
			pathToCards[p] = append(pathToCards[p], card.ID)
		}
		cardAgents[card.ID] = card.SourceAgent
		cardActive[card.ID] = card.Status == "active"
		cardTitles[card.ID] = card.Title
		cardPaths[card.ID] = card.Scope.Paths
		cardModules[card.ID] = card.Scope.Modules
		cardKnowledgeKind[card.ID] = card.KnowledgeKind

		if len(issues) > 0 {
			report = append(report, map[string]any{
				"id":     card.ID,
				"title":  card.Title,
				"status": card.Status,
				"issues": issues,
			})
		}
	}

	// Agent conflict detection: two active cards sharing a path but
	// authored by different (non-empty) source agents.
	seenConflict := map[string]bool{}
	for _, mids := range pathToCards {
		if len(mids) < 2 {
			continue
		}
		for i := 0; i < len(mids); i++ {
			for j := i + 1; j < len(mids); j++ {
				mid1, mid2 := mids[i], mids[j]
				if !cardActive[mid1] || !cardActive[mid2] {
					continue
				}
				a1, a2 := cardAgents[mid1], cardAgents[mid2]
				if a1 == "" || a2 == "" || a1 == a2 {
					continue
				}
				// Conflict between mid1 and mid2
				pairKey1 := mid1 + ":" + mid2
				pairKey2 := mid2 + ":" + mid1
				if !seenConflict[pairKey1] && !seenConflict[pairKey2] {
					seenConflict[pairKey1] = true
					report = append(report, map[string]any{
						"id":     mid1,
						"title":  cardTitles[mid1],
						"status": "active",
						"issues": []string{"agent_conflict:" + mid2},
					})
					report = append(report, map[string]any{
						"id":     mid2,
						"title":  cardTitles[mid2],
						"status": "active",
						"issues": []string{"agent_conflict:" + mid1},
					})
				}
			}
		}
	}

	// Topic conflict detection: two active cards from different non-empty
	// agents whose titles share >= 2 significant words but do NOT share
	// any scope paths.
	for i := 0; i < len(cards); i++ {
		for j := i + 1; j < len(cards); j++ {
			mid1, mid2 := cards[i].ID, cards[j].ID
			if !cardActive[mid1] || !cardActive[mid2] {
				continue
			}
			a1, a2 := cardAgents[mid1], cardAgents[mid2]
			if a1 == "" || a2 == "" || a1 == a2 {
				continue
			}
			// Skip if they share any paths (already caught by agent_conflict)
			if shareAny(cardPaths[mid1], cardPaths[mid2]) {
				continue
			}
			// Check word overlap in titles
			if wordOverlap(cardTitles[mid1], cardTitles[mid2]) {
				pairKey1 := mid1 + ":" + mid2
				pairKey2 := mid2 + ":" + mid1
				if !seenConflict[pairKey1] && !seenConflict[pairKey2] {
					seenConflict[pairKey1] = true
					report = append(report, map[string]any{
						"id":     mid1,
						"title":  cardTitles[mid1],
						"status": "active",
						"issues": []string{"topic_conflict:" + mid2},
					})
					report = append(report, map[string]any{
						"id":     mid2,
						"title":  cardTitles[mid2],
						"status": "active",
						"issues": []string{"topic_conflict:" + mid1},
					})
				}
			}
		}
	}

	// Semantic conflict detection: two active cards explicitly linked
	// by a "contradicts" relation.
	for _, card := range cards {
		if !cardActive[card.ID] {
			continue
		}
		for _, targetID := range card.Relations["contradicts"] {
			if !cardActive[targetID] {
				continue
			}
			pairKey1 := card.ID + ":" + targetID
			pairKey2 := targetID + ":" + card.ID
			if !seenConflict[pairKey1] && !seenConflict[pairKey2] {
				seenConflict[pairKey1] = true
				report = append(report, map[string]any{
					"id":     card.ID,
					"title":  cardTitles[card.ID],
					"status": "active",
					"issues": []string{"semantic_conflict:" + targetID},
				})
				report = append(report, map[string]any{
					"id":     targetID,
					"title":  cardTitles[targetID],
					"status": "active",
					"issues": []string{"semantic_conflict:" + card.ID},
				})
			}
		}
	}

	// Scope overlap detection: two active cards from different non-empty
	// agents that share any scope paths or modules (partial overlap)
	// and have different knowledge_kind.
	for i := 0; i < len(cards); i++ {
		for j := i + 1; j < len(cards); j++ {
			mid1, mid2 := cards[i].ID, cards[j].ID
			if !cardActive[mid1] || !cardActive[mid2] {
				continue
			}
			a1, a2 := cardAgents[mid1], cardAgents[mid2]
			if a1 == "" || a2 == "" || a1 == a2 {
				continue
			}
			// Must have different knowledge_kind
			k1, k2 := cardKnowledgeKind[mid1], cardKnowledgeKind[mid2]
			if k1 == k2 || k1 == "" || k2 == "" {
				continue
			}
			// Check for partial scope overlap (paths or modules)
			overlaps := shareAny(cardPaths[mid1], cardPaths[mid2]) ||
				shareAny(cardModules[mid1], cardModules[mid2])
			if !overlaps {
				continue
			}
			pairKey1 := mid1 + ":" + mid2
			pairKey2 := mid2 + ":" + mid1
			if !seenConflict[pairKey1] && !seenConflict[pairKey2] {
				seenConflict[pairKey1] = true
				report = append(report, map[string]any{
					"id":     mid1,
					"title":  cardTitles[mid1],
					"status": "active",
					"issues": []string{"scope_overlap:" + mid2},
				})
				report = append(report, map[string]any{
					"id":     mid2,
					"title":  cardTitles[mid2],
					"status": "active",
					"issues": []string{"scope_overlap:" + mid1},
				})
			}
		}
	}

	return report, nil
}

// shareAny returns true if the two string slices have at least one
// element in common.
func shareAny(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if set[s] && s != "" {
			return true
		}
	}
	return false
}

// wordOverlap returns true if the two strings share at least 2
// significant words (length > 2, case-insensitive).
func wordOverlap(a, b string) bool {
	wordsA := significantWords(a)
	wordsB := significantWords(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return false
	}

	set := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		set[w] = true
	}

	count := 0
	for _, w := range wordsB {
		if set[w] {
			count++
			if count >= 2 {
				return true
			}
		}
	}
	return false
}

// significantWords splits a string on whitespace, lowercases each token,
// and returns only tokens longer than 2 characters (excluding empty).
func significantWords(s string) []string {
	parts := strings.Fields(s)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(p)
		if len(p) > 2 {
			out = append(out, p)
		}
	}
	return out
}
