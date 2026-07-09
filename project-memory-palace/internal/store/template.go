package store

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadTemplate loads a card template from .project-memory/templates/<name>.yaml
func LoadTemplate(projectRoot, name string) (map[string]any, error) {
	tmplPath := filepath.Join(TemplatesDir(projectRoot), name+".yaml")
	data, err := os.ReadFile(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("template %q not found: %w", name, err)
	}
	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("template %q invalid YAML: %w", name, err)
	}
	return result, nil
}

// ListTemplates returns available template names in .project-memory/templates/
func ListTemplates(projectRoot string) ([]string, error) {
	dir := TemplatesDir(projectRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	return names, nil
}
