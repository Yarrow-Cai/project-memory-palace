package memory

import (
	"testing"
	)

func validCard() MemoryCard {
	return MemoryCard{
		SchemaVersion: SchemaVersion,
		ID:            "mem_20260609_001",
		Type:          "decision",
		Status:        "active",
		Confidence:    0.86,
		Title:         "Test Memory",
		Summary:       "A test memory card.",
		Content:       "Full content body.",
		Source: SourceInfo{
			Kind:        "conversation",
			Description: "User confirmed during design.",
		},
		Scope: ScopeInfo{Project: "test", Modules: []string{"core"}},
		Tags:      []string{"test"},
		Relations: map[string][]string{},
		CreatedAt: "2026-06-09T12:00:00+08:00",
		UpdatedAt: "2026-06-09T12:00:00+08:00",
	}
}

func TestValidCard(t *testing.T) {
	card := validCard()
	if err := ValidateCard(&card); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestInvalidSchemaVersion(t *testing.T) {
	card := validCard()
	card.SchemaVersion = 2
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for schema_version != 1")
	}
}

func TestInvalidType(t *testing.T) {
	card := validCard()
	card.Type = "bad_type"
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestInvalidStatus(t *testing.T) {
	card := validCard()
	card.Status = "bad_status"
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestConfidenceOutOfRange(t *testing.T) {
	card := validCard()
	card.Confidence = 1.5
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for confidence > 1.0")
	}
}

func TestConfidenceNegative(t *testing.T) {
	card := validCard()
	card.Confidence = -0.1
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for confidence < 0")
	}
}

func TestInvalidID(t *testing.T) {
	card := validCard()
	card.ID = "bad"
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for invalid ID")
	}
}

func TestMissingFields(t *testing.T) {
	card := validCard()
	card.Title = ""
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestInvalidSourceKind(t *testing.T) {
	card := validCard()
	card.Source.Kind = "bad"
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for bad source kind")
	}
}

func TestEmptySourceDescription(t *testing.T) {
	card := validCard()
	card.Source.Description = ""
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for empty source description")
	}
}

func TestSelfRelation(t *testing.T) {
	card := validCard()
	card.Relations["related_to"] = []string{card.ID}
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for self-relation")
	}
}

func TestInvalidRelationTarget(t *testing.T) {
	card := validCard()
	card.Relations["related_to"] = []string{"bad-id"}
	if err := ValidateCard(&card); err == nil {
		t.Fatal("expected error for invalid relation target")
	}
}

func TestNewCard(t *testing.T) {
	card := NewCard("decision", "Title", "Summary", "Content", 0.8)
	if card.SchemaVersion != SchemaVersion {
		t.Fatalf("expected schema_version %d", SchemaVersion)
	}
	if card.ID == "" {
		t.Fatal("NewCard should generate ID")
	}
	if card.Status != "active" {
		t.Fatalf("expected status active, got %s", card.Status)
	}
	if card.Type != "decision" {
		t.Fatalf("expected type decision, got %s", card.Type)
	}
}

func TestToSummary(t *testing.T) {
	card := validCard()
	summary := card.ToSummary()
	if summary["id"] != card.ID {
		t.Fatal("summary should include id")
	}
	if _, exists := summary["content"]; exists {
		t.Fatal("summary should NOT include content")
	}
	if _, exists := summary["source"]; exists && summary["source"] != card.Source.Kind {
		t.Fatal("summary should NOT include full source")
	}
}

func TestValidatePayload(t *testing.T) {
	payload := map[string]any{"type": "decision", "title": "T", "summary": "S", "content": "C"}
	if err := ValidatePayload(payload); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	delete(payload, "title")
	if err := ValidatePayload(payload); err == nil {
		t.Fatal("expected error for missing title")
	}
}

