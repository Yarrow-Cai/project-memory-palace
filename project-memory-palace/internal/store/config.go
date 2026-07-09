package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PMemConfig holds persistent workspace preferences.
type PMemConfig struct {
	Workspace      string `json:"workspace"`
	DefaultProject string `json:"default_project,omitempty"`
}

func configPath() string {
	return filepath.Join(os.Getenv("APPDATA"), "project-memory-palace", "config.json")
}

// LoadConfig reads the config file. Returns zero-value config if file missing.
func LoadConfig() (*PMemConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &PMemConfig{}, nil
		}
		return nil, fmt.Errorf("load config: %w", err)
	}
	var cfg PMemConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig atomically writes the config file.
func SaveConfig(cfg *PMemConfig) error {
	dir := filepath.Dir(configPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return os.Rename(tmp, configPath())
}
