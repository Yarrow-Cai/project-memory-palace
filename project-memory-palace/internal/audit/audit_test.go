package audit

import (
	
	"testing"

	"github.com/atop/project-memory-palace/internal/memory"
	"github.com/atop/project-memory-palace/internal/store"
)

// helper writes a card YAML file under the given project root and returns
// the written card.  It panics on error (test helper).
func writeTestCard(t *testing.T, root string, card *memory.MemoryCard) {
	t.Helper()
	if _, err := store.WriteCard(root, card, false); err != nil {
		t.Fatalf("WriteCard: %v", err)
	}
}

func testCard(id, title string, confidence float64, status string, sourceKind string, tags []string, modules []string) *memory.MemoryCard {
	return &memory.MemoryCard{
		SchemaVersion: 1,
		ID:            id,
		Type:          "decision",
		Status:        status,
		Confidence:    confidence,
		Title:         title,
		Summary:       title + " summary.",
		Content:       title + " content.",
		Source:        memory.SourceInfo{Kind: sourceKind, Description: "test source"},
		Scope:         memory.ScopeInfo{Modules: modules},
		Tags:          tags,
		Relations:     map[string][]string{},
		CreatedAt:     "2026-06-09T12:00:00Z",
		UpdatedAt:     "2026-06-09T12:00:00Z",
	}
}

func emptyProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := store.EnsureCardsDir(root); err != nil {
		t.Fatal(err)
	}
	return root
}

// ---------- empty ----------

func TestEmptyProject(t *testing.T) {
	root := emptyProject(t)
	report, err := AuditProject(root)
	if err != nil {
		t.Fatalf("AuditProject: %v", err)
	}
	if len(report) != 0 {
		t.Fatalf("expected 0 issues, got %d", len(report))
	}
}

// ---------- low confidence ----------

func TestLowConfidence(t *testing.T) {
	root := emptyProject(t)
	card := testCard("mem_20260610_001", "Low Conf", 0.4, "active", "manual", []string{"tag"}, []string{"core"})
	writeTestCard(t, root, card)

	report, err := AuditProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(report))
	}
	found := false
	for _, iss := range report[0]["issues"].([]string) {
		if iss == "low_confidence" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected low_confidence in issues, got %v", report[0]["issues"])
	}
}

// ---------- duplicate title ----------

func TestDuplicateTitle(t *testing.T) {
	root := emptyProject(t)
	card1 := testCard("mem_20260610_001", "Same Title", 0.9, "active", "manual", []string{"tag"}, []string{"core"})
	card2 := testCard("mem_20260610_002", "Same Title", 0.8, "active", "conversation", []string{"tag"}, []string{"core"})
	writeTestCard(t, root, card1)
	writeTestCard(t, root, card2)

	report, err := AuditProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) < 1 {
		t.Fatalf("expected at least 1 issue report, got %d", len(report))
	}
	// The second card should have a duplicate issue.
	hasDup := false
	for _, entry := range report {
		for _, iss := range entry["issues"].([]string) {
			if iss == "possible_duplicate:mem_20260610_001" {
				hasDup = true
			}
		}
	}
	if !hasDup {
		t.Fatalf("expected possible_duplicate issue, got %+v", report)
	}
}

// ---------- stale ----------

func TestStaleStatus(t *testing.T) {
	root := emptyProject(t)
	card := testCard("mem_20260610_001", "Stale Card", 0.9, "stale", "manual", []string{"tag"}, []string{"core"})
	writeTestCard(t, root, card)

	report, err := AuditProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(report))
	}
	found := false
	for _, iss := range report[0]["issues"].([]string) {
		if iss == "stale" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected stale in issues, got %v", report[0]["issues"])
	}
}
