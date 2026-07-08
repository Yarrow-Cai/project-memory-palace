package audit

import (
	"strings"

	"github.com/atop/project-memory-palace/internal/store"
)

// AuditProject scans all YAML memory cards in the project and returns a
// report of issues found.  An empty slice means the project is clean.
//
// Checks performed:
//   - low_confidence  — confidence <= 0.5
//   - missing_tags    — no tags assigned
//   - missing_scope   — neither modules nor paths are set
//   - high_confidence_inference — analysis-sourced card with confidence > 0.7
//   - possible_duplicate — another card with the same lowercased title
//   - stale           — status is "stale"
func AuditProject(projectRoot string) ([]map[string]any, error) {
	cards, err := store.DiscoverCards(projectRoot)
	if err != nil {
		return nil, err
	}

	seenTitles := map[string]string{}
	var report []map[string]any

	// Collect cross-card data for agent conflict detection
	pathToCards := map[string][]string{}
	cardAgents := map[string]string{}
	cardActive := map[string]bool{}
	cardTitles := map[string]string{}

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
		if card.Source.Kind == "analysis" && card.Confidence > 0.7 {
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

		// Collect for agent conflict analysis
		for _, p := range card.Scope.Paths {
			pathToCards[p] = append(pathToCards[p], card.ID)
		}
		cardAgents[card.ID] = card.SourceAgent
		cardActive[card.ID] = card.Status == "active"
		cardTitles[card.ID] = card.Title

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

	return report, nil
}
