package store

import (
	"errors"
	"fmt"
	"os"
)

const defaultConfigYAML = "# Project Memory Palace Configuration\nschema_version: 1\nproject: \"\"\ncreated_at: \"\"\n"

const defaultRulesYAML = "# Agent Rules - synthesized from memory cards\nversion: 1\nrules: []\n"

// EnsureProjectMemory creates the .project-memory directory tree and default
// config/rule files if they do not already exist. It is safe to call repeatedly.
func EnsureProjectMemory(projectRoot string) error {
	dirs := []string{MemoryDir(projectRoot), CardsDir(projectRoot), RulesDir(projectRoot), TemplatesDir(projectRoot)}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	files := map[string]string{
		ConfigPath(projectRoot): defaultConfigYAML,
		RulesPath(projectRoot):  defaultRulesYAML,
	}
	for path, content := range files {
		if _, err := os.Stat(path); err == nil {
			continue // already exists
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// AssertMemoryLayout checks that every required directory and file exists under
// the project memory root. Returns nil when the layout is intact.
func AssertMemoryLayout(projectRoot string) error {
	dirs := []string{MemoryDir(projectRoot), CardsDir(projectRoot), RulesDir(projectRoot)}
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil {
			return fmt.Errorf("missing directory %s: %w", d, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", d)
		}
	}

	files := []string{
		ConfigPath(projectRoot),
		RulesPath(projectRoot),
		IndexPath(projectRoot),
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("missing file %s: %w", f, err)
		}
	}
	return nil
}

// RemoveCard deletes a card file from disk. Path must be the full path returned
// by WriteCard or discovered via DiscoverCards.
func RemoveCard(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// EnsureCardsDir ensures the cards directory exists under the project root.
func EnsureCardsDir(projectRoot string) error {
	return os.MkdirAll(CardsDir(projectRoot), 0755)
}
