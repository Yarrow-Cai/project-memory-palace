package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atop/project-memory-palace/internal/memory"
)

func testCard(id, cardType, title string) *memory.MemoryCard {
	return &memory.MemoryCard{
		SchemaVersion: 1,
		ID:            id,
		Type:          cardType,
		Status:        "active",
		Confidence:    0.8,
		Title:         title,
		Summary:       title + " summary",
		Content:       title + " content",
		Source:        memory.SourceInfo{Kind: "manual", Description: "test"},
		Scope:         memory.ScopeInfo{Project: "test"},
		Tags:          []string{},
		Relations:     map[string][]string{},
		CreatedAt:     "2026-06-09T10:00:00Z",
		UpdatedAt:     "2026-06-09T10:00:00Z",
	}
}

// ---------- paths ----------

func TestMemoryDir(t *testing.T) {
	got := MemoryDir("/foo")
	sep := string(filepath.Separator)
	if !strings.HasSuffix(got, sep+".project-memory") {
		t.Errorf("MemoryDir = %s, want suffix .project-memory", got)
	}
	if !strings.HasPrefix(got, "/foo") && !strings.HasPrefix(got, `\foo`) {
		t.Errorf("MemoryDir = %s, want prefix /foo or \\foo", got)
	}
}

func TestCardsDir(t *testing.T) {
	got := CardsDir("/foo")
	if !strings.HasSuffix(got, filepath.Join(".project-memory", "cards")) {
		t.Errorf("CardsDir = %s, unexpected", got)
	}
}

func TestRulesDir(t *testing.T) {
	got := RulesDir("/foo")
	if !strings.HasSuffix(got, filepath.Join(".project-memory", "rules")) {
		t.Errorf("RulesDir = %s, unexpected", got)
	}
}

func TestConfigPath(t *testing.T) {
	got := ConfigPath("/foo")
	if !strings.HasSuffix(got, filepath.Join(".project-memory", "config.yaml")) {
		t.Errorf("ConfigPath = %s, unexpected", got)
	}
}

func TestRulesPath(t *testing.T) {
	got := RulesPath("/foo")
	if !strings.HasSuffix(got, filepath.Join(".project-memory", "rules", "agent-rules.yaml")) {
		t.Errorf("RulesPath = %s, unexpected", got)
	}
}

func TestIndexPath(t *testing.T) {
	got := IndexPath("/foo")
	if !strings.HasSuffix(got, filepath.Join(".project-memory", "index.sqlite3")) {
		t.Errorf("IndexPath = %s, unexpected", got)
	}
}

// ---------- ensure / assert ----------

func TestEnsureProjectMemory_CreatesAll(t *testing.T) {
	root := t.TempDir()

	if err := EnsureProjectMemory(root); err != nil {
		t.Fatalf("EnsureProjectMemory: %v", err)
	}

	dirs := []string{MemoryDir(root), CardsDir(root), RulesDir(root)}
	for _, d := range dirs {
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			t.Errorf("expected dir %s to exist", d)
		}
	}

	files := []string{ConfigPath(root), RulesPath(root)}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected file %s to exist", f)
		}
	}

	// Second call should be safe.
	if err := EnsureProjectMemory(root); err != nil {
		t.Fatalf("second EnsureProjectMemory: %v", err)
	}
}

func TestAssertMemoryLayout_OK(t *testing.T) {
	root := t.TempDir()
	// deliberately missing index.sqlite3 until ensure creates the dirs,
	// but AssertMemoryLayout checks for the file — ensure does not create it.
	// We create it manually.
	if err := EnsureProjectMemory(root); err != nil {
		t.Fatal(err)
	}
	// IndexPath is checked by Assert but not created by Ensure; touch it.
	if err := os.WriteFile(IndexPath(root), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	if err := AssertMemoryLayout(root); err != nil {
		t.Errorf("AssertMemoryLayout should pass: %v", err)
	}
}

func TestAssertMemoryLayout_MissingDir(t *testing.T) {
	root := t.TempDir()
	if err := AssertMemoryLayout(root); err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestAssertMemoryLayout_MissingFile(t *testing.T) {
	root := t.TempDir()
	if err := EnsureProjectMemory(root); err != nil {
		t.Fatal(err)
	}
	// IndexPath is missing — should fail.
	if err := AssertMemoryLayout(root); err == nil {
		t.Error("expected error for missing index.sqlite3")
	}
}

// ---------- identity ----------

func TestNextCardIdentity_FirstCard(t *testing.T) {
	root := t.TempDir()
	if err := EnsureCardsDir(root); err != nil {
		t.Fatal(err)
	}

	id, seq, err := NextCardIdentity(root, "2026-06-09")
	if err != nil {
		t.Fatal(err)
	}
	if seq != 1 {
		t.Errorf("seq = %d, want 1", seq)
	}
	if id != "mem_20260609_001" {
		t.Errorf("id = %s, want mem_20260609_001", id)
	}
}

func TestNextCardIdentity_ExistingCards(t *testing.T) {
	root := t.TempDir()
	if err := EnsureCardsDir(root); err != nil {
		t.Fatal(err)
	}

	// Simulate two existing cards for the same date.
	files := []string{
		filepath.Join(CardsDir(root), "2026-06-09_001_decision.yaml"),
		filepath.Join(CardsDir(root), "2026-06-09_002_decision.yaml"),
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("dummy"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	id, seq, err := NextCardIdentity(root, "2026-06-09")
	if err != nil {
		t.Fatal(err)
	}
	if seq != 3 {
		t.Errorf("seq = %d, want 3", seq)
	}
	if id != "mem_20260609_003" {
		t.Errorf("id = %s, want mem_20260609_003", id)
	}
}

func TestNextCardIdentity_DifferentDate(t *testing.T) {
	root := t.TempDir()
	if err := EnsureCardsDir(root); err != nil {
		t.Fatal(err)
	}

	f := filepath.Join(CardsDir(root), "2026-06-08_005_decision.yaml")
	if err := os.WriteFile(f, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}

	// Different date — should start from 1.
	id, seq, err := NextCardIdentity(root, "2026-06-09")
	if err != nil {
		t.Fatal(err)
	}
	if seq != 1 {
		t.Errorf("seq = %d, want 1 (different date)", seq)
	}
	if id != "mem_20260609_001" {
		t.Errorf("id = %s, want mem_20260609_001", id)
	}
}

// ---------- CardFilename ----------

func TestCardFilename_Valid(t *testing.T) {
	card := testCard("mem_20260609_003", "decision", "Some Decision")
	fn := CardFilename(card)
	if fn != "2026-06-09_003_decision.yaml" {
		t.Errorf("CardFilename = %s, want 2026-06-09_003_decision.yaml", fn)
	}
}

func TestCardFilename_InvalidID(t *testing.T) {
	card := testCard("bad-id", "decision", "Bad")
	fn := CardFilename(card)
	if fn != "" {
		t.Errorf("CardFilename = %q, want empty for invalid ID", fn)
	}
}

// ---------- write / read ----------

func TestWriteReadCard_RoundTrip(t *testing.T) {
	root := t.TempDir()
	card := testCard("mem_20260609_001", "decision", "Round Trip")

	writtenPath, err := WriteCard(root, card, false)
	if err != nil {
		t.Fatalf("WriteCard: %v", err)
	}

	expected := filepath.Join(CardsDir(root), "2026-06-09_001_decision.yaml")
	if writtenPath != expected {
		t.Errorf("path = %s, want %s", writtenPath, expected)
	}

	// Read back.
	got, err := ReadCard(writtenPath)
	if err != nil {
		t.Fatalf("ReadCard: %v", err)
	}

	if got.ID != card.ID || got.Title != card.Title || got.Type != card.Type {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, card)
	}
}

func TestWriteCard_OverwriteProtection(t *testing.T) {
	root := t.TempDir()
	card := testCard("mem_20260609_001", "decision", "First")

	if _, err := WriteCard(root, card, false); err != nil {
		t.Fatal(err)
	}

	// Same card, no overwrite — should fail.
	if _, err := WriteCard(root, card, false); err == nil {
		t.Error("expected error: file already exists")
	}
}

func TestWriteCard_Overwrite(t *testing.T) {
	root := t.TempDir()
	cardA := testCard("mem_20260609_001", "decision", "First")
	cardB := testCard("mem_20260609_001", "decision", "Second")

	if _, err := WriteCard(root, cardA, false); err != nil {
		t.Fatal(err)
	}

	// Overwrite should succeed.
	path, err := WriteCard(root, cardB, true)
	if err != nil {
		t.Fatalf("WriteCard overwrite: %v", err)
	}

	got, err := ReadCard(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Second" {
		t.Errorf("title = %s, want Second (overwrite)", got.Title)
	}
}

func TestWriteCard_InvalidID(t *testing.T) {
	root := t.TempDir()
	card := testCard("bad-id", "decision", "Bad")

	if _, err := WriteCard(root, card, false); err == nil {
		t.Error("expected error for invalid card ID")
	}
}

func TestWriteCard_ValidationError(t *testing.T) {
	root := t.TempDir()
	card := testCard("mem_20260609_001", "decision", "Bad")
	card.Confidence = 2.0 // out of range

	if _, err := WriteCard(root, card, false); err == nil {
		t.Error("expected validation error for confidence > 1.0")
	}
}

// ---------- discover ----------

func TestDiscoverCards_Ordered(t *testing.T) {
	root := t.TempDir()

	cards := []*memory.MemoryCard{
		testCard("mem_20260610_001", "decision", "Tuesday"),
		testCard("mem_20260609_001", "decision", "Monday"),
		testCard("mem_20260609_002", "design",    "Monday Design"),
	}

	for _, c := range cards {
		if _, err := WriteCard(root, c, false); err != nil {
			t.Fatal(err)
		}
	}

	discovered, err := DiscoverCards(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(discovered) != 3 {
		t.Fatalf("got %d cards, want 3", len(discovered))
	}

	// Should be sorted by ID ascending.
	if discovered[0].ID != "mem_20260609_001" {
		t.Errorf("[0] = %s, want mem_20260609_001", discovered[0].ID)
	}
	if discovered[1].ID != "mem_20260609_002" {
		t.Errorf("[1] = %s, want mem_20260609_002", discovered[1].ID)
	}
	if discovered[2].ID != "mem_20260610_001" {
		t.Errorf("[2] = %s, want mem_20260610_001", discovered[2].ID)
	}
}

func TestDiscoverCards_Empty(t *testing.T) {
	root := t.TempDir()
	if err := EnsureCardsDir(root); err != nil {
		t.Fatal(err)
	}

	cards, err := DiscoverCards(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 0 {
		t.Errorf("expected 0 cards, got %d", len(cards))
	}
}

func TestDiscoverCards_NoDirYet(t *testing.T) {
	root := t.TempDir()
	// No cards dir — should return empty, not error.
	cards, err := DiscoverCards(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 0 {
		t.Errorf("expected 0 cards, got %d", len(cards))
	}
}

// ---------- RemoveCard ----------

func TestRemoveCard(t *testing.T) {
	root := t.TempDir()
	card := testCard("mem_20260609_001", "decision", "Remove Me")

	path, err := WriteCard(root, card, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := RemoveCard(path); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}
}

func TestRemoveCard_NotExist(t *testing.T) {
	root := t.TempDir()
	// Removing a non-existent file should not error.
	if err := RemoveCard(filepath.Join(CardsDir(root), "nonexistent.yaml")); err != nil {
		t.Errorf("expected nil for non-existent file, got %v", err)
	}
}

// ---------- EnsureCardsDir ----------

func TestEnsureCardsDir(t *testing.T) {
	root := t.TempDir()
	if err := EnsureCardsDir(root); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(CardsDir(root)); err != nil || !info.IsDir() {
		t.Error("cards dir should exist after EnsureCardsDir")
	}
}

