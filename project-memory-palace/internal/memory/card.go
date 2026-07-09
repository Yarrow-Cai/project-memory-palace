package memory

import (
	"fmt"
	"regexp"
	"time"
)

type SourceInfo struct {
	Kind        string   `yaml:"kind" json:"kind"`
	Description string   `yaml:"description" json:"description"`
	Files       []string `yaml:"files" json:"files"`
	Commits     []string `yaml:"commits" json:"commits"`
}

type ScopeInfo struct {
	Project string   `yaml:"project" json:"project"`
	Modules []string `yaml:"modules" json:"modules"`
	Paths   []string `yaml:"paths" json:"paths"`
}

type MemoryCard struct {
	SchemaVersion int                 `yaml:"schema_version" json:"schema_version"`
	ID            string              `yaml:"id" json:"id"`
	Type          string              `yaml:"type" json:"type"`
	Status        string              `yaml:"status" json:"status"`
	Confidence    float64             `yaml:"confidence" json:"confidence"`
	Title         string              `yaml:"title" json:"title"`
	Summary       string              `yaml:"summary" json:"summary"`
	Content       string              `yaml:"content" json:"content"`
	Source        SourceInfo          `yaml:"source" json:"source"`
	Scope         ScopeInfo           `yaml:"scope" json:"scope"`
	Tags          []string            `yaml:"tags" json:"tags"`
	Relations     map[string][]string `yaml:"relations" json:"relations"`
	Priority      int                 `yaml:"priority" json:"priority"` // 1-5, default 3
	ExpiresAt     string              `yaml:"expires_at" json:"expires_at"` // ISO timestamp, empty = never expires
	SourceAgent   string              `yaml:"source_agent" json:"source_agent"`
	KnowledgeKind string              `yaml:"knowledge_kind" json:"knowledge_kind"`
	VerifiedBy    string              `yaml:"verified_by,omitempty" json:"verified_by,omitempty"`
	OutdatedBy    string              `yaml:"outdated_by,omitempty" json:"outdated_by,omitempty"`
	CreatedAt     string              `yaml:"created_at" json:"created_at"`
	UpdatedAt     string              `yaml:"updated_at" json:"updated_at"`
}

func NewCard(cardType, title, summary, content string, confidence float64) MemoryCard {
	return MemoryCard{
		SchemaVersion: SchemaVersion,
		ID:            fmt.Sprintf("mem_%s_001", time.Now().Format("20060102")),
		Type:          cardType,
		Status:        "active",
		Confidence:    confidence,
		Priority:      3,
		Title:         title,
		Summary:       summary,
		Content:       content,
		Source:        SourceInfo{Kind: "analysis", Description: "Source was not supplied by caller."},
		Tags:          []string{},
		Relations:     map[string][]string{},
		CreatedAt:     time.Now().Format(time.RFC3339),
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}
}

func (c *MemoryCard) ToSummary() map[string]any {
	return map[string]any{
		"id": c.ID, "type": c.Type, "status": c.Status,
		"title": c.Title, "summary": c.Summary,
		"confidence": c.Confidence, "source_hint": c.Source.Kind,
		"matched_by": []string{}, "updated_at": c.UpdatedAt,
	}
}

var idRe = regexp.MustCompile(`^mem_\d{8}_\d{3}$`)
