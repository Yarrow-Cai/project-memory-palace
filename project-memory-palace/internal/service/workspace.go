package service

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// WorkspaceService manages multiple project MemoryServices within a workspace
// directory, routing MCP tool calls to the correct project.
type WorkspaceService struct {
	mu           sync.RWMutex
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

	// Start background poller for auto-discovery of new projects
	go ws.pollNewProjects()
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

// Resolve returns the MemoryService for a project name (or default if empty).
// Exported wrapper for callers outside the service package.
func (ws *WorkspaceService) Resolve(project string) (*MemoryService, string, error) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.resolve(project)
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

// RecallAll searches across ALL projects and returns merged results.
// Each result includes a "project" field. Results are sorted by relevance
// (priority DESC, updated_at DESC). Limits total results across all projects.
func (ws *WorkspaceService) RecallAll(query string, filters map[string]any, limit int) ([]map[string]any, error) {
	var all []map[string]any
	for name, svc := range ws.projects {
		results, err := svc.Recall(query, filters, limit)
		if err != nil {
			log.Printf("pmem: recall_all skipping project %s: %v", name, err)
			continue
		}
		for _, r := range results {
			r["project"] = name
			all = append(all, r)
		}
	}
	// Sort by effective priority (decay-aware), then updated_at DESC
	computeEffective := func(priority int, lastAccessedAt string) float64 {
		if lastAccessedAt == "" { return float64(priority) }
		t, err := time.Parse(time.RFC3339, lastAccessedAt)
		if err != nil { return float64(priority) }
		days := time.Since(t).Hours() / 24
		switch {
		case days < 7:   return float64(priority) * 1.0
		case days < 30:  return float64(priority) * 0.85
		case days < 60:  return float64(priority) * 0.6
		case days < 180: return float64(priority) * 0.4
		default:         return float64(priority) * 0.25
		}
	}
	sort.Slice(all, func(i, j int) bool {
		pi, _ := all[i]["priority"].(int)
		pj, _ := all[j]["priority"].(int)
		lai, _ := all[i]["last_accessed_at"].(string)
		laj, _ := all[j]["last_accessed_at"].(string)
		epi := computeEffective(pi, lai)
		epj := computeEffective(pj, laj)
		if epi != epj { return epi > epj }
		ui, _ := all[i]["updated_at"].(string)
		uj, _ := all[j]["updated_at"].(string)
		return ui > uj
	})
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// AutoDetect determines which project file paths belong to.
// Returns project name with most path matches.
func (ws *WorkspaceService) AutoDetect(paths []string) string {
	bestProj := ""
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
	if bestProj == "" {
		return ws.defaultProj // fallback
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

// RefreshWorkspace re-scans workspaceDir for new subdirectories containing
// .project-memory/. Creates and initializes a MemoryService for each NEW
// project found. Does NOT remove existing projects. Returns the list of
// newly added project names.
func (ws *WorkspaceService) RefreshWorkspace() []string {
	entries, err := os.ReadDir(ws.workspaceDir)
	if err != nil {
		log.Printf("pmem: refresh_workspace: %v", err)
		return nil
	}
	var added []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if _, exists := ws.projects[dirName]; exists {
			continue
		}
		projDir := filepath.Join(ws.workspaceDir, dirName)
		projMemoryDir := filepath.Join(projDir, ".project-memory")
		if info, err := os.Stat(projMemoryDir); err == nil && info.IsDir() {
			svc := New(projDir)
			ws.projects[dirName] = svc
			if ws.defaultProj == "" {
				ws.defaultProj = dirName
			}
			added = append(added, dirName)
		}
	}
	return added
}

// pollNewProjects periodically scans workspaceDir for new .project-memory/
// subdirectories. Runs every 30 seconds. Never exits.
func (ws *WorkspaceService) pollNewProjects() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		added := ws.RefreshWorkspace()
		for _, name := range added {
			log.Printf("pmem: auto-discovered new project: %s", name)
		}
	}
}

// WorkspaceCoverage returns aggregated coverage stats across all projects.
func (ws *WorkspaceService) WorkspaceCoverage() (map[string]any, error) {
	type projModule struct {
		Module      string `json:"module"`
		Count       int    `json:"card_count"`
		LastUpdated string `json:"last_updated"`
	}
	type projData struct {
		Name    string       `json:"name"`
		Modules []projModule `json:"modules"`
	}
	var projects []projData
	totalCards := 0
	moduleSet := make(map[string]bool)
	for name, svc := range ws.projects {
		stats, err := svc.CoverageStats()
		if err != nil {
			continue
		}
		pd := projData{Name: name}
		for _, s := range stats {
			modName, _ := s["module"].(string)
			count, _ := s["card_count"].(int)
			lu, _ := s["last_updated"].(string)
			pd.Modules = append(pd.Modules, projModule{Module: modName, Count: count, LastUpdated: lu})
			totalCards += count
			if modName != "" {
				moduleSet[modName] = true
			}
		}
		projects = append(projects, pd)
	}
	return map[string]any{
		"total_cards":   totalCards,
		"total_modules": len(moduleSet),
		"projects":      projects,
	}, nil
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

// ExtractPatterns scans all projects for cards with type="pattern",
// groups them by tag similarity, and returns reusable patterns.
func (ws *WorkspaceService) ExtractPatterns(minProjects int) ([]map[string]any, error) {
	if minProjects < 1 { minProjects = 1 }
	type patternInfo struct {
		title, summary string
		tags           []string
		project        string
	}
	var allPatterns []patternInfo
	for name, svc := range ws.projects {
		cards, err := svc.ExportCards()
		if err != nil {
			log.Printf("pmem: extract_patterns skipping project %s: %v", name, err)
			continue
		}
		for _, card := range cards {
			tp, _ := card["type"].(string)
			tpLower := strings.ToLower(tp)
			if tpLower != "pattern" && !strings.Contains(tpLower, "pattern") { continue }
			title, _ := card["title"].(string)
			summary, _ := card["summary"].(string)
			tags := toStringSlice(card["tags"])
			allPatterns = append(allPatterns, patternInfo{title, summary, tags, name})
		}
	}
	type patternGroup struct {
		title, summary string
		tags, sourceProjects []string
		projectCount int
	}
	var groups []patternGroup
	used := make([]bool, len(allPatterns))
	for i, p := range allPatterns {
		if used[i] { continue }
		group := patternGroup{p.title, p.summary, p.tags, []string{p.project}, 0}
		used[i] = true
		for j := i + 1; j < len(allPatterns); j++ {
			if used[j] { continue }
			q := allPatterns[j]
			if tagsOverlap(p.tags, q.tags) {
				group.sourceProjects = append(group.sourceProjects, q.project)
				group.tags = mergeTags(group.tags, q.tags)
				used[j] = true
			}
		}
		seen := make(map[string]bool)
		for _, proj := range group.sourceProjects { seen[proj] = true }
		group.sourceProjects = make([]string, 0, len(seen))
		for proj := range seen { group.sourceProjects = append(group.sourceProjects, proj) }
		group.projectCount = len(group.sourceProjects)
		groups = append(groups, group)
	}
	var filtered []map[string]any
	for _, g := range groups {
		if g.projectCount >= minProjects {
			filtered = append(filtered, map[string]any{
				"title": g.title, "summary": g.summary, "tags": g.tags,
				"source_projects": g.sourceProjects, "project_count": g.projectCount,
			})
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		ci, _ := filtered[i]["project_count"].(int)
		cj, _ := filtered[j]["project_count"].(int)
		return ci > cj
	})
	return filtered, nil
}

// SetWorkspace switches to a new workspace directory, re-scanning for projects.
func (ws *WorkspaceService) SetWorkspace(path string, defaultProj string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("set workspace: %w", err)
	}
	newProjects := make(map[string]*MemoryService)
	var firstProj string
	for _, entry := range entries {
		if !entry.IsDir() { continue }
		dirName := entry.Name()
		projDir := filepath.Join(path, dirName)
		projMemoryDir := filepath.Join(projDir, ".project-memory")
		if info, err := os.Stat(projMemoryDir); err == nil && info.IsDir() {
			newProjects[dirName] = New(projDir)
			if firstProj == "" { firstProj = dirName }
		}
	}
	if len(newProjects) == 0 {
		return fmt.Errorf("set workspace: no projects found in %s", path)
	}
	ws.workspaceDir = path
	ws.projects = newProjects
	if defaultProj != "" && newProjects[defaultProj] != nil {
		ws.defaultProj = defaultProj
	} else {
		ws.defaultProj = firstProj
	}
	return nil
}

// SetDefaultProject sets the default project for the current workspace.
func (ws *WorkspaceService) SetDefaultProject(name string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if _, ok := ws.projects[name]; !ok {
		return fmt.Errorf("project %q not found", name)
	}
	ws.defaultProj = name
	return nil
}

func tagsOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 { return false }
	set := make(map[string]bool, len(a))
	for _, t := range a { set[strings.ToLower(t)] = true }
	for _, t := range b { if set[strings.ToLower(t)] { return true } }
	return false
}

func mergeTags(a, b []string) []string {
	set := make(map[string]bool)
	for _, t := range a { set[t] = true }
	for _, t := range b { set[t] = true }
	result := make([]string, 0, len(set))
	for t := range set { result = append(result, t) }
	return result
}
