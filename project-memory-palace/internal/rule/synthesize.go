package rule

import (
	"fmt"
	"os"
	"time"

	"github.com/atop/project-memory-palace/internal/store"
	"gopkg.in/yaml.v3"
)

type AgentRule struct {
	ID           string "yaml:\"id\" json:\"id\""
	SourceMemory string "yaml:\"source_memory\" json:\"source_memory\""
	Title        string "yaml:\"title\" json:\"title\""
	Category     string "yaml:\"category\" json:\"category\""
	Body         string "yaml:\"body\" json:\"body\""
	CreatedAt    string "yaml:\"created_at\" json:\"created_at\""
}

type RulesDocument struct {
	Version        int         "yaml:\"version\" json:\"version\""
	SynthesizedAt  string      "yaml:\"synthesized_at\" json:\"synthesized_at\""
	Rules          []AgentRule "yaml:\"rules\" json:\"rules\""
}

// Synthesize scans all active convention and decision cards and regenerates
// agent-rules.yaml. The file is written atomically (temp + rename).
func Synthesize(projectRoot string) (*RulesDocument, error) {
	cards, err := store.DiscoverCards(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("synthesize: discover: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	doc := &RulesDocument{
		Version:       1,
		SynthesizedAt: now,
		Rules:         make([]AgentRule, 0),
	}

	n := 0
	for _, card := range cards {
		// Only active convention and decision cards become rules.
		if card.Status != "active" {
			continue
		}
		if card.Type != "convention" && card.Type != "decision" {
			continue
		}

		n++
		rule := AgentRule{
			ID:           fmt.Sprintf("rule_%03d", n),
			SourceMemory: card.ID,
			Title:        card.Title,
			Category:     card.Type,
			Body:         card.Content,
			CreatedAt:    now,
		}
		doc.Rules = append(doc.Rules, rule)
	}

	// Serialize to YAML
	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("synthesize: marshal: %w", err)
	}

	targetPath := store.RulesPath(projectRoot)
	tmp, err := os.CreateTemp(store.RulesDir(projectRoot), ".tmp-rules-*")
	if err != nil {
		return nil, fmt.Errorf("synthesize: tempfile: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("synthesize: write temp: %w", err)
	}
	tmp.Close()

	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("synthesize: rename: %w", err)
	}

	return doc, nil
}
