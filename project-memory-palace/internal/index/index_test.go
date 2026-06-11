package index

import (
	"path/filepath"
	"testing"

	"github.com/atop/project-memory-palace/internal/memory"
	"github.com/atop/project-memory-palace/internal/store"
)

func setup(t *testing.T) (string, *MemoryIndex) {
	t.Helper()
	dir := t.TempDir()
	store.EnsureProjectMemory(dir)
	return dir, NewMemoryIndex(dir)
}

func writeCard(t *testing.T, dir, id, typ, title, summary, content, status string, conf float64) {
	t.Helper()
	card := memory.NewCard(typ, title, summary, content, conf)
	card.ID = id
	card.Status = status
	card.CreatedAt = "2026-06-09T12:00:00+08:00"
	card.UpdatedAt = "2026-06-09T12:00:00+08:00"
	store.WriteCard(dir, &card, false)
}

func TestInitialize(t *testing.T) {
	_, idx := setup(t)
	defer idx.Close()
	if err := idx.Initialize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpsertAndSearch(t *testing.T) {
	dir, idx := setup(t)
	defer idx.Close()
	idx.Initialize()
	writeCard(t, dir, "mem_20260609_001", "decision", "Use YAML", "We use YAML cards.", "Content.", "active", 0.86)
	card, _ := store.ReadCard(filepath.Join(store.CardsDir(dir), "2026-06-09_001_decision.yaml"))
	idx.Upsert(card)
	results, err := idx.Search("cards", nil, 5)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if len(results) == 0 { t.Fatal("expected results") }
	if results[0]["id"] != "mem_20260609_001" { t.Fatalf("wrong id: %v", results[0]["id"]) }
}

func TestSearchStatusFilter(t *testing.T) {
	dir, idx := setup(t)
	defer idx.Close()
	idx.Initialize()
	writeCard(t, dir, "mem_20260609_001", "decision", "Active", "Active card.", "Content.", "active", 0.9)
	writeCard(t, dir, "mem_20260609_002", "decision", "Rejected", "Rejected card.", "Content.", "rejected", 0.3)
	card1, _ := store.ReadCard(filepath.Join(store.CardsDir(dir), "2026-06-09_001_decision.yaml"))
	card2, _ := store.ReadCard(filepath.Join(store.CardsDir(dir), "2026-06-09_002_decision.yaml"))
	idx.Upsert(card1); idx.Upsert(card2)
	results, _ := idx.Search("card", nil, 10)
	if len(results) != 1 { t.Fatalf("expected 1 active result, got %d", len(results)) }
	results, _ = idx.Search("card", map[string]any{"status": []string{"active", "rejected"}}, 10)
	if len(results) != 2 { t.Fatalf("expected 2 results, got %d", len(results)) }
}

func TestRecent(t *testing.T) {
	dir, idx := setup(t)
	defer idx.Close()
	idx.Initialize()
	for i := 0; i < 3; i++ {
		id := "mem_20260609_00" + string(rune('0'+i+1))
		writeCard(t, dir, id, "decision", "Test", "Summary", "Content", "active", 0.8)
		card, _ := store.ReadCard(filepath.Join(store.CardsDir(dir), "2026-06-09_00" + string(rune('0'+i+1)) + "_decision.yaml"))
		idx.Upsert(card)
	}
	results, _ := idx.Recent(2)
	if len(results) != 2 { t.Fatalf("expected 2 recent, got %d", len(results)) }
}

func TestRebuild(t *testing.T) {
	dir, idx := setup(t)
	defer idx.Close()
	writeCard(t, dir, "mem_20260609_001", "decision", "Test", "S", "C", "active", 0.8)
	idx.Rebuild()
	results, _ := idx.Search("Test", nil, 5)
	if len(results) == 0 { t.Fatal("expected results after rebuild") }
}

func TestClear(t *testing.T) {
	dir, idx := setup(t)
	defer idx.Close()
	idx.Initialize()
	writeCard(t, dir, "mem_20260609_001", "decision", "Test", "S", "C", "active", 0.8)
	card, _ := store.ReadCard(filepath.Join(store.CardsDir(dir), "2026-06-09_001_decision.yaml"))
	idx.Upsert(card)
	idx.Clear()
	results, _ := idx.Recent(10)
	if len(results) != 0 { t.Fatal("expected 0 results after clear") }
}
