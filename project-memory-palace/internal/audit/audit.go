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

		if len(issues) > 0 {
			report = append(report, map[string]any{
				"id":     card.ID,
				"title":  card.Title,
				"status": card.Status,
				"issues": issues,
			})
		}
	}

	return report, nil
}