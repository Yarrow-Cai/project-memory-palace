package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/atop/project-memory-palace/internal/store"
)

// helper creates a working MemoryService rooted at a temp directory.
func testService(t *testing.T) *MemoryService {
	t.Helper()
	svc := New(t.TempDir())
	if err := svc.InitProject(); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	return svc
}

func validPayload() map[string]any {
	return map[string]any{
		"type":    "decision",
		"title":   "Choose Architecture",
		"summary": "Decided on hex arch.",
		"content": "Full rationale for the architecture decision.",
		"source": map[string]any{
			"kind":        "conversation",
			"description": "Design review meeting.",
		},
		"confidence": 0.8,
		"scope": map[string]any{
			"modules": []string{"core"},
		},
		"tags": []string{"architecture"},
	}
}

func TestServiceInitProject(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	if svc.ProjectRoot() == "" {
		t.Fatal("ProjectRoot should be non-empty after InitProject")
	}
}

func TestRememberAndRecall(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	payload := validPayload()

	card, err := svc.Remember(payload)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	id, _ := card["id"].(string)
	if id == "" {
		t.Fatal("expected non-empty id in returned card")
	}
	if title, _ := card["title"].(string); title != "Choose Architecture" {
		t.Fatalf("title = %q, want %q", title, "Choose Architecture")
	}

	results, err := svc.Recall("Architecture", nil, 10)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Recall result count = %d, want 1", len(results))
	}
	if results[0]["id"] != id {
		t.Fatalf("Recall result id = %v, want %v", results[0]["id"], id)
	}
}

func TestOpenMemory(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	payload := validPayload()

	card, err := svc.Remember(payload)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	id := card["id"].(string)

	opened, err := svc.OpenMemory(id)
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if opened["id"] != id {
		t.Fatalf("OpenMemory id = %v, want %v", opened["id"], id)
	}
	if opened["content"] == nil || opened["content"] == "" {
		t.Fatal("OpenMemory should include full content")
	}
}

func TestOpenMemoryNotFound(t *testing.T) {
	svc := testService(t)
	defer svc.Close()

	_, err := svc.OpenMemory("mem_20990101_001")
	if err == nil {
		t.Fatal("expected error for non-existent memory")
	}
	mnf, ok := err.(*MemoryNotFoundError)
	if !ok {
		t.Fatalf("expected MemoryNotFoundError, got %T: %v", err, err)
	}
	if mnf.ID != "mem_20990101_001" {
		t.Fatalf("MemoryNotFoundError.ID = %q, want %q", mnf.ID, "mem_20990101_001")
	}
}

func TestListRecent(t *testing.T) {
	svc := testService(t)
	defer svc.Close()

	for i, title := range []string{"First", "Second"} {
		p := validPayload()
		p["title"] = title
		if _, err := svc.Remember(p); err != nil {
			t.Fatalf("Remember %d: %v", i, err)
		}
	}

	results, err := svc.ListRecent(5, 0, nil)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("ListRecent count = %d, want 2", len(results))
	}
	if results[0]["title"] == "" {
		t.Fatal("ListRecent should show the most recent")
	}
}

func TestUpdateMemory(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	card, err := svc.Remember(validPayload())
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	id := card["id"].(string)

	updated, err := svc.UpdateMemory(id, map[string]any{"status": "stale"})
	if err != nil {
		t.Fatalf("UpdateMemory status: %v", err)
	}
	if updated["status"] != "stale" {
		t.Fatalf("status = %v, want stale", updated["status"])
	}

	opened, err := svc.OpenMemory(id)
	if err != nil {
		t.Fatalf("OpenMemory after update: %v", err)
	}
	if opened["status"] != "stale" {
		t.Fatalf("persisted status = %v, want stale", opened["status"])
	}

	updated2, err := svc.UpdateMemory(id, map[string]any{
		"confidence": 0.95,
		"tags":       []string{"updated", "important"},
	})
	if err != nil {
		t.Fatalf("UpdateMemory tags: %v", err)
	}
	if c, _ := updated2["confidence"].(float64); c != 0.95 {
		t.Fatalf("confidence = %v, want 0.95", c)
	}

	results, err := svc.Recall("Architecture", map[string]any{"status": []string{"active","stale"}}, 10)
	if err != nil {
		t.Fatalf("Recall after update: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected Recall to find the updated card")
	}
	if results[0]["status"] != "stale" {
		t.Fatalf("index status = %v, want stale", results[0]["status"])
	}
}

func TestRebuildIndex(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	payload := validPayload()
	card, err := svc.Remember(payload)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	id := card["id"].(string)

	if err := svc.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}

	results, err := svc.Recall("Architecture", nil, 10)
	if err != nil {
		t.Fatalf("Recall after rebuild: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("recall count after rebuild = %d, want 1", len(results))
	}
	if results[0]["id"] != id {
		t.Fatalf("recall id = %v, want %v", results[0]["id"], id)
	}
}

func TestRememberMissingFields(t *testing.T) {
	svc := testService(t)
	defer svc.Close()

	payload := map[string]any{"type": "decision"}
	_, err := svc.Remember(payload)
	if err == nil {
		t.Fatal("expected validation error for missing title/summary/content")
	}
}

func TestUpdateNoChanges(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	card, err := svc.Remember(validPayload())
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	id := card["id"].(string)

	updated, err := svc.UpdateMemory(id, map[string]any{})
	if err != nil {
		t.Fatalf("UpdateMemory with empty updates: %v", err)
	}
	if updated["status"] != "active" {
		t.Fatalf("status changed with empty updates: %v", updated["status"])
	}
}
func TestRememberWithTemplate(t *testing.T) {
	svc := testService(t)
	defer svc.Close()

	// Create a template file in the project's templates dir
	tmplDir := store.TemplatesDir(svc.ProjectRoot())
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmplContent := `type: convention
title: "Template Default Title"
summary: "Template default summary"
content: "Template default content"
confidence: 0.9
tags:
  - template-tag
`
	if err := os.WriteFile(filepath.Join(tmplDir, "test_template.yaml"), []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Payload provides type + title + summary + content (overrides template),
	// but NOT tags or confidence — those should come from template
	payload := map[string]any{
		"template": "test_template",
		"type":     "decision",
		"title":    "My Custom Title",
		"summary":  "My custom summary",
		"content":  "My custom content",
		"source": map[string]any{
			"kind":        "conversation",
			"description": "Test",
		},
	}

	card, err := svc.Remember(payload)
	if err != nil {
		t.Fatalf("Remember with template: %v", err)
	}

	// Explicit payload fields should take precedence
	if card["type"] != "decision" {
		t.Errorf("type = %v, want decision (explicit overrides template)", card["type"])
	}
	if card["title"] != "My Custom Title" {
		t.Errorf("title = %v, want 'My Custom Title'", card["title"])
	}

	// Template default fields should be applied
	tags, _ := card["tags"].([]string)
	if len(tags) != 1 || tags[0] != "template-tag" {
		t.Errorf("tags from template = %v, want [template-tag]", tags)
	}
	conf, _ := card["confidence"].(float64)
	if conf != 0.9 {
		t.Errorf("confidence from template = %v, want 0.9", conf)
	}
}

func TestRememberWithTemplateNotFound(t *testing.T) {
	svc := testService(t)
	defer svc.Close()

	payload := map[string]any{
		"template": "nonexistent_template",
		"type":     "decision",
		"title":    "Test",
		"summary":  "Test",
		"content":  "Test",
		"source": map[string]any{
			"kind":        "conversation",
			"description": "Test",
		},
	}

	_, err := svc.Remember(payload)
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestListTemplatesMethod(t *testing.T) {
	svc := testService(t)
	defer svc.Close()

	tmplDir := store.TemplatesDir(svc.ProjectRoot())
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create one template
	if err := os.WriteFile(filepath.Join(tmplDir, "alpha.yaml"), []byte("title: A\n"), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := svc.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(names) != 1 || names[0] != "alpha" {
		t.Errorf("ListTemplates = %v, want [alpha]", names)
	}
}
