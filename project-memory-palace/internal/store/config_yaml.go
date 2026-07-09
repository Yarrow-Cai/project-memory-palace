package store

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProjectConfig is the project-level configuration stored in .project-memory/config.yaml.
type ProjectConfig struct {
	SchemaVersion int      `yaml:"schema_version"`
	Project       string   `yaml:"project"`
	Description   string   `yaml:"description"`
	CreatedAt     string   `yaml:"created_at"`
	WorkspacePath string   `yaml:"workspace_path"`
	Default       bool     `yaml:"default"`
	Modules       []string `yaml:"modules"`
}

// LoadProjectConfig reads .project-memory/config.yaml for the given project root.
// Returns nil config and nil error if the file doesn't exist.
func LoadProjectConfig(projectRoot string) (*ProjectConfig, error) {
	path := ConfigPath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Modules == nil {
		cfg.Modules = []string{}
	}
	return &cfg, nil
}

// SaveProjectConfig writes .project-memory/config.yaml atomically.
func SaveProjectConfig(projectRoot string, cfg *ProjectConfig) error {
	path := ConfigPath(projectRoot)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return os.Rename(tmp, path)
}

// SetDefaultProject marks one project as default (default=true) and clears
// the flag on all other projects in the same workspace.
func SetDefaultProject(workspaceDir string, projectName string) error {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projDir := filepath.Join(workspaceDir, entry.Name())
		cfg, err := LoadProjectConfig(projDir)
		if err != nil || cfg == nil {
			continue
		}
		cfg.Default = (entry.Name() == projectName)
		cfg.WorkspacePath = workspaceDir
		SaveProjectConfig(projDir, cfg)
	}
	return nil
}
