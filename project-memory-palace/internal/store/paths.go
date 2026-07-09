package store

import "path/filepath"

const memoryDirName = ".project-memory"

// MemoryDir returns the project memory root directory path.
func MemoryDir(projectRoot string) string { return filepath.Join(projectRoot, memoryDirName) }

// CardsDir returns the cards subdirectory path.
func CardsDir(projectRoot string) string { return filepath.Join(MemoryDir(projectRoot), "cards") }

// RulesDir returns the rules subdirectory path.
func RulesDir(projectRoot string) string { return filepath.Join(MemoryDir(projectRoot), "rules") }

// ConfigPath returns the path to config.yaml.
func ConfigPath(projectRoot string) string { return filepath.Join(MemoryDir(projectRoot), "config.yaml") }

// RulesPath returns the path to agent-rules.yaml.
func RulesPath(projectRoot string) string { return filepath.Join(RulesDir(projectRoot), "agent-rules.yaml") }

func HistoryDir(projectRoot string) string { return filepath.Join(MemoryDir(projectRoot), "history") }
func TemplatesDir(projectRoot string) string { return filepath.Join(MemoryDir(projectRoot), "templates") }

// IndexPath returns the path to index.sqlite3.
func IndexPath(projectRoot string) string { return filepath.Join(MemoryDir(projectRoot), "index.sqlite3") }
