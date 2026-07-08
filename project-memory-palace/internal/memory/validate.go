package memory

import (
	"fmt"
	"regexp"
	"strings"
)

var idTargetRe = regexp.MustCompile(`^mem_\d{8}_\d{3}$`)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func ValidateCard(card *MemoryCard) error {
	if card.SchemaVersion != SchemaVersion {
		return &ValidationError{"schema_version", fmt.Sprintf("must be %d", SchemaVersion)}
	}
	if !MemoryTypes[card.Type] {
		return &ValidationError{"type", "invalid type"}
	}
	if !MemoryStatuses[card.Status] {
		return &ValidationError{"status", "invalid status"}
	}
	if card.Confidence < 0.0 || card.Confidence > 1.0 {
		return &ValidationError{"confidence", "must be between 0.0 and 1.0"}
	}
	if !idRe.MatchString(card.ID) {
		return &ValidationError{"id", "must match mem_YYYYMMDD_NNN"}
	}
	if strings.TrimSpace(card.Title) == "" {
		return &ValidationError{"title", "must be non-empty"}
	}
	if strings.TrimSpace(card.Summary) == "" {
		return &ValidationError{"summary", "must be non-empty"}
	}
	if strings.TrimSpace(card.Content) == "" {
		return &ValidationError{"content", "must be non-empty"}
	}
	if strings.TrimSpace(card.CreatedAt) == "" {
		return &ValidationError{"created_at", "must be non-empty"}
	}
	if strings.TrimSpace(card.UpdatedAt) == "" {
		return &ValidationError{"updated_at", "must be non-empty"}
	}
	if !SourceKinds[card.Source.Kind] {
		return &ValidationError{"source.kind", "invalid kind"}
	}
	if strings.TrimSpace(card.Source.Description) == "" {
		return &ValidationError{"source.description", "must be non-empty"}
	}
	if card.KnowledgeKind != "" && !KnowledgeKinds[card.KnowledgeKind] {
		return &ValidationError{"knowledge_kind", "must be one of: fact, interpretation, rule"}
	}
	for rel, targets := range card.Relations {
		found := false
		for _, r := range RelationKinds {
			if r == rel {
				found = true
				break
			}
		}
		if !found {
			return &ValidationError{"relations", "unknown relation: " + rel}
		}
		for _, t := range targets {
			if !idTargetRe.MatchString(t) {
				return &ValidationError{"relations", "invalid target: " + t}
			}
			if t == card.ID {
				return &ValidationError{"relations", "self-reference: " + t}
			}
		}
	}
	return nil
}

func ValidatePayload(payload map[string]any) error {
	for _, f := range RememberRequiredFields {
		if _, ok := payload[f]; !ok {
			return &ValidationError{f, "is required"}
		}
	}
	return nil
}
