package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkspaceService manages multiple project MemoryServices within a workspace
// directory, routing MCP tool calls to the correct project.
type WorkspaceService struct {
	workspaceDir string
	projects     map[string]*MemoryService // dirname -> service
	defaultProj  string
}

// NewWorkspace scans workspaceDir for subdirectories containing .project-memory/
// and creates a MemoryService for each. Returns error if no projects found.
func NewWorkspace(workspaceDir string) (*WorkspaceService, error) {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}

	ws := &WorkspaceService{
		workspaceDir: workspaceDir,
		projects:     make(map[string]*MemoryService),
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		projDir := filepath.Join(workspaceDir, dirName)
		projMemoryDir := filepath.Join(projDir, ".project-memory")
		if info, err := os.Stat(projMemoryDir); err == nil && info.IsDir() {
			ws.projects[dirName] = New(projDir)
			if ws.defaultProj == "" {
				ws.defaultProj = dirName
			}
		}
	}

	if len(ws.projects) == 0 {
		return nil, fmt.Errorf("workspace: no projects found in %s", workspaceDir)
	}

	return ws, nil
}

// NewSingleProject creates a workspace from a single project directory.
// Used as fallback when NewWorkspace finds no sub-projects.
func NewSingleProject(projectRoot string) (*WorkspaceService, error) {
	name := filepath.Base(projectRoot)
	ws := &WorkspaceService{
		workspaceDir: filepath.Dir(projectRoot),
		projects:     map[string]*MemoryService{name: New(projectRoot)},
		defaultProj:  name,
	}
	return ws, nil
}

// resolve returns the MemoryService for a project name (or default if empty).
// Case-insensitive matching as fallback.
func (ws *WorkspaceService) resolve(project string) (*MemoryService, string, error) {
	if project == "" {
		project = ws.defaultProj
	}
	// Exact match first
	if svc, ok := ws.projects[project]; ok {
		return svc, project, nil
	}
	// Case-insensitive fallback
	lower := strings.ToLower(project)
	for name, svc := range ws.projects {
		if strings.ToLower(name) == lower {
			return svc, name, nil
		}
	}
	return nil, "", fmt.Errorf("project %q not found (available: %s)", project, strings.Join(ws.ProjectNames(), ", "))
}

// ProjectNames returns all project names.
func (ws *WorkspaceService) ProjectNames() []string {
	names := make([]string, 0, len(ws.projects))
	for name := range ws.projects {
		names = append(names, name)
	}
	return names
}

// ListProjects returns metadata: name, project_root, card_count, is_default for each project.
func (ws *WorkspaceService) ListProjects() ([]map[string]any, error) {
	var projects []map[string]any
	for name, svc := range ws.projects {
		count := 0
		if cnt, err := svc.Count(nil); err == nil {
			count = cnt
		}
		projects = append(projects, map[string]any{
			"name":         name,
			"project_root": svc.ProjectRoot(),
			"card_count":   count,
			"is_default":   name == ws.defaultProj,
		})
	}
	return projects, nil
}

// AutoDetect determines which project file paths belong to.
// Returns project name with most path matches.
func (ws *WorkspaceService) AutoDetect(paths []string) string {
	bestProj := ws.defaultProj
	bestCount := 0
	for name, svc := range ws.projects {
		root := svc.ProjectRoot()
		count := 0
		for _, p := range paths {
			rel, err := filepath.Rel(root, p)
			if err == nil && !strings.HasPrefix(rel, "..") {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestProj = name
		}
	}
	return bestProj
}

// Close closes all project services.
func (ws *WorkspaceService) Close() error {
	var firstErr error
	for _, svc := range ws.projects {
		if err := svc.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// extractProject extracts "project" string from MCP params map.
func extractProject(params map[string]any) string {
	if params == nil {
		return ""
	}
	if v, ok := params["project"].(string); ok {
		return v
	}
	return ""
}
