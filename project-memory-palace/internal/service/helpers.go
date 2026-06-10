package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/atop/project-memory-palace/internal/memory"
)

func now() string {
	return time.Now().Format(time.RFC3339)
}

func buildCard(cardID string, payload map[string]any) memory.MemoryCard {
	card := memory.NewCard(
		fmt.Sprint(payload["type"]),
		fmt.Sprint(payload["title"]),
		fmt.Sprint(payload["summary"]),
		fmt.Sprint(payload["content"]),
		toFloat64OrDefault(payload["confidence"], memory.DefaultConfidence),
	)
	card.ID = cardID
	nowStr := now()
	card.CreatedAt = nowStr
	card.UpdatedAt = nowStr

	if status, ok := payload["status"].(string); ok {
		card.Status = status
	}
	if src, ok := payload["source"].(map[string]any); ok {
		if k, ok := src["kind"].(string); ok { card.Source.Kind = k }
		if d, ok := src["description"].(string); ok { card.Source.Description = d }
		card.Source.Files = toStringSlice(src["files"])
		card.Source.Commits = toStringSlice(src["commits"])
	}
	if scope, ok := payload["scope"].(map[string]any); ok {
		if p, ok := scope["project"].(string); ok { card.Scope.Project = p }
		card.Scope.Modules = toStringSlice(scope["modules"])
		card.Scope.Paths = toStringSlice(scope["paths"])
	}
	card.Tags = toStringSlice(payload["tags"])
	if rels, ok := payload["relations"].(map[string]any); ok {
		for k, v := range rels { card.Relations[k] = toStringSlice(v) }
	}
	// Cap confidence if no source provided
	if _, ok := payload["source"]; !ok || payload["source"] == nil {
		if card.Confidence > memory.MaxConfidenceNoSource {
			card.Confidence = memory.MaxConfidenceNoSource
		}
	}
	return card
}

func cardToMap(card *memory.MemoryCard) map[string]any {
	return map[string]any{
		"schema_version": card.SchemaVersion,
		"id": card.ID, "type": card.Type, "status": card.Status,
		"confidence": card.Confidence, "title": card.Title,
		"summary": card.Summary, "content": card.Content,
		"source": map[string]any{
			"kind": card.Source.Kind, "description": card.Source.Description,
			"files": card.Source.Files, "commits": card.Source.Commits,
		},
		"scope": map[string]any{
			"project": card.Scope.Project,
			"modules": card.Scope.Modules, "paths": card.Scope.Paths,
		},
		"tags": card.Tags, "relations": card.Relations,
		"created_at": card.CreatedAt, "updated_at": card.UpdatedAt,
	}
}

func mapToCard(m map[string]any) *memory.MemoryCard {
	card := &memory.MemoryCard{
		SchemaVersion: 1, Relations: make(map[string][]string),
	}
	if v, ok := m["id"].(string); ok { card.ID = v }
	if v, ok := m["type"].(string); ok { card.Type = v }
	if v, ok := m["status"].(string); ok { card.Status = v }
	if v, ok := m["title"].(string); ok { card.Title = v }
	if v, ok := m["summary"].(string); ok { card.Summary = v }
	if v, ok := m["content"].(string); ok { card.Content = v }
	if v, ok := m["confidence"].(float64); ok { card.Confidence = v }
	if v, ok := m["created_at"].(string); ok { card.CreatedAt = v }
	if v, ok := m["updated_at"].(string); ok { card.UpdatedAt = v }
	card.Tags = toStringSlice(m["tags"])
	if src, ok := m["source"].(map[string]any); ok {
		if k, ok := src["kind"].(string); ok { card.Source.Kind = k }
		if d, ok := src["description"].(string); ok { card.Source.Description = d }
		card.Source.Files = toStringSlice(src["files"])
		card.Source.Commits = toStringSlice(src["commits"])
	}
	if scope, ok := m["scope"].(map[string]any); ok {
		if p, ok := scope["project"].(string); ok { card.Scope.Project = p }
		card.Scope.Modules = toStringSlice(scope["modules"])
		card.Scope.Paths = toStringSlice(scope["paths"])
	}
	if rels, ok := m["relations"].(map[string]any); ok {
		for k, v := range rels { card.Relations[k] = toStringSlice(v) }
	}
	return card
}

func buildNotification(card *memory.MemoryCard) string {
	var b strings.Builder
	b.WriteString("Project memory written:\n")
	fmt.Fprintf(&b, "- ID: %s\n", card.ID)
	fmt.Fprintf(&b, "- Type: %s\n", card.Type)
	fmt.Fprintf(&b, "- Summary: %s\n", card.Summary)
	fmt.Fprintf(&b, "- Source: %s - %s\n", card.Source.Kind, card.Source.Description)
	if s := card.Relations["supersedes"]; len(s) > 0 {
		fmt.Fprintf(&b, "- Supersedes: %s\n", strings.Join(s, ", "))
	}
	if s := card.Relations["superseded_by"]; len(s) > 0 {
		fmt.Fprintf(&b, "- Superseded by: %s\n", strings.Join(s, ", "))
	}
	b.WriteString("- Future use: use recall to retrieve this summary, then open_memory for details.")
	return b.String()
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64: return n, true
	case float32: return float64(n), true
	case int: return float64(n), true
	case int64: return float64(n), true
	case json.Number: f, err := n.Float64(); return f, err == nil
	default: return 0, false
	}
}

func toFloat64OrDefault(v any, defaultVal float64) float64 {
	if v == nil { return defaultVal }
	if f, ok := toFloat64(v); ok { return f }
	return defaultVal
}

func toStringList(v any) ([]string, error) {
	switch list := v.(type) {
	case []string: return list, nil
	case []any:
		result := make([]string, len(list))
		for i, item := range list { result[i] = fmt.Sprint(item) }
		return result, nil
	case string:
		if list == "" { return []string{}, nil }
		return []string{list}, nil
	default: return nil, fmt.Errorf("expected a list of strings")
	}
}

func toStringSlice(v any) []string {
	switch list := v.(type) {
	case []string: return list
	case []any:
		result := make([]string, len(list))
		for i, item := range list { result[i] = fmt.Sprint(item) }
		return result
	default: return nil
	}
}
