package tray

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/atop/project-memory-palace/internal/mcp"
	"github.com/atop/project-memory-palace/internal/store"
	"github.com/atop/project-memory-palace/internal/service"
	"github.com/getlantern/systray"
	_ "embed"
	"log"
)

//go:embed templates/index.html
var indexHTML string

//go:embed icon.png
var iconPNG []byte

func buildICO() []byte {
	pngData := iconPNG
	pngSize := len(pngData)
	buf := make([]byte, 22+pngSize)
	buf[0], buf[1] = 0, 0
	buf[2], buf[3] = 1, 0
	buf[4], buf[5] = 1, 0
	buf[6] = 32
	buf[7] = 32
	buf[8] = 0
	buf[9] = 0
	buf[10], buf[11] = 1, 0
	buf[12], buf[13] = 32, 0
	buf[14], buf[15], buf[16], buf[17] = byte(pngSize), byte(pngSize>>8), byte(pngSize>>16), byte(pngSize>>24)
	buf[18], buf[19], buf[20], buf[21] = 22, 0, 0, 0
	copy(buf[22:], pngData)
	return buf
}

var (
	mu          sync.Mutex
	svc         *service.MemoryService
	projectRoot string
	mcpCmd      *exec.Cmd
	mcpRunning  bool
	recentsPath string
	recents     []string
	iconICO     = buildICO()
)

func init() {
	recentsPath = filepath.Join(os.Getenv("APPDATA"), "project-memory-palace", "recents.json")
	os.MkdirAll(filepath.Dir(recentsPath), 0700)
	loadRecents()
}

func loadRecents() {
	data, err := os.ReadFile(recentsPath)
	if err != nil { recents = []string{}; return }
	json.Unmarshal(data, &recents)
	if recents == nil { recents = []string{} }
}

func saveRecents() {
	seen := map[string]bool{}
	var deduped []string
	deduped = append(deduped, projectRoot)
	seen[projectRoot] = true
	for _, r := range recents {
		if !seen[r] && len(deduped) < 20 {
			deduped = append(deduped, r)
			seen[r] = true
		}
	}
	recents = deduped
	os.MkdirAll(filepath.Dir(recentsPath), 0700)
	data, _ := json.Marshal(recents)
	os.WriteFile(recentsPath, data, 0644)
}

// AddRecent records a project root path in the shared recents list
// (persisted to APPDATA/project-memory-palace/recents.json).
func AddRecent(root string) {
	seen := map[string]bool{}
	root = filepath.Clean(root)
	seen[root] = true
	for _, r := range recents {
		if !seen[r] && len(seen) < 20 {
			seen[r] = true
		}
	}
	var deduped []string
	deduped = append(deduped, root)
	for _, r := range recents {
		if r != root { deduped = append(deduped, r) }
	}
	recents = deduped
	os.MkdirAll(filepath.Dir(recentsPath), 0700)
	data, _ := json.Marshal(recents)
	os.WriteFile(recentsPath, data, 0644)
}

// RecentList returns a copy of the shared recents list.
func RecentList() []string {
	out := make([]string, len(recents))
	copy(out, recents)
	return out
}

func Run(root string) {
	projectRoot = root
	svc = service.New(projectRoot)
	svc.InitProject()
	go startAPI()
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(iconICO)
	systray.SetTitle("Project Memory Palace")
	systray.SetTooltip("Project Memory Palace")

	mShow := systray.AddMenuItem("Show", "Open in browser")
	systray.AddSeparator()
	mCopyMCP := systray.AddMenuItem("Copy MCP Config", "Copy SSE MCP configuration to clipboard")
	mMCPStatus := systray.AddMenuItem("MCP: Stopped", "")
	mMCPStatus.Disable()
	mMCPToggle := systray.AddMenuItem("Start MCP", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit application")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				exec.Command("cmd", "/c", "start", "http://127.0.0.1:8147").Start()
			case <-mCopyMCP.ClickedCh:
				copyMCPConfig()
			case <-mMCPToggle.ClickedCh:
				toggleMCP(mMCPStatus, mMCPToggle)
			case <-mQuit.ClickedCh:
				if mcpCmd != nil { mcpCmd.Process.Kill() }
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	if mcpCmd != nil { mcpCmd.Process.Kill() }
}

func copyMCPConfig() {
	exe, _ := os.Executable()
	cfg := mcp.BuildMCPConfig(exe, projectRoot)
	cmd := exec.Command("clip")
	cmd.Stdin = strings.NewReader(cfg)
	cmd.Run()
}

func toggleMCP(si, bi *systray.MenuItem) {
	if mcpRunning {
		if mcpCmd != nil { mcpCmd.Process.Kill(); mcpCmd = nil }
		mcpRunning = false
		si.SetTitle("MCP: Stopped")
		bi.SetTitle("Start MCP")
	} else {
		exe, _ := os.Executable()
		mcpCmd = exec.Command(exe, "serve-mcp", projectRoot)
		mcpCmd.Stdout = os.Stderr
		mcpCmd.Stderr = os.Stderr
		if err := mcpCmd.Start(); err != nil {
			si.SetTitle("MCP: Error - " + err.Error())
			return
		}
		mcpRunning = true
		si.SetTitle("MCP: Running")
		bi.SetTitle("Stop MCP")
	}
}

func startAPI() {
	reg := mcp.NewToolRegistry()
	registerMCPTools(reg)
	sseServer := mcp.NewSSEServer(reg)

	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/sse", sseServer.HandleSSE)
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/message", sseServer.HandleMessage)
	http.HandleFunc("/api/recent", handleRecent)
	http.HandleFunc("/api/search", handleSearch)
	http.HandleFunc("/api/open", handleOpen)
	http.HandleFunc("/api/update", handleUpdate)
	http.HandleFunc("/api/project", handleProject)
	http.HandleFunc("/api/project/set", handleProjectSet)
	http.HandleFunc("/api/projects/recent", handleRecents)
	fmt.Fprintln(os.Stderr, "API + SSE server started on 127.0.0.1:8147")
	if err := http.ListenAndServe("127.0.0.1:8147", nil); err != nil {
		log.Printf("HTTP server error: %v", err)
	}
}

func registerMCPTools(reg *mcp.ToolRegistry) {
	reg.Register("init_project", "Initialize Project Memory Palace for this project. Call this FIRST — returns project context (active rules, recent activity, next-step guide) in one shot. Creates .project-memory/ directory tree. Safe to call repeatedly.", map[string]any{
		"type": "object", "properties": map[string]any{},
	}, func(params map[string]any) (any, error) {
		mu.Lock(); defer mu.Unlock()
		if err := svc.InitProject(); err != nil { return nil, err }
		saveRecents()
		result := map[string]any{"status": "initialized", "project_root": projectRoot}
		data, err := os.ReadFile(store.RulesPath(svc.ProjectRoot()))
		if err == nil {
			var doc map[string]any
			if yaml.Unmarshal(data, &doc) == nil {
				result["rules"] = doc["rules"]
			}
		}
		recent, _ := svc.ListRecent(5)
		result["recent"] = recent
		result["next"] = []string{
			"1. recall query=<keyword> - search project memory by topic or file path",
			"2. open_memory id=<id> - get full card details when summary isn't enough",
			"3. remember memory={...} - persist new knowledge after completing work",
		}
		return result, nil
	})

	reg.Register("remember", "Write one durable project memory card. Required: type, title, summary, content. Include source to achieve confidence > 0.5.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"memory": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{
						"type": "string",
						"description": "Memory type: project_goal, design, decision, change_reason, bugfix, module, convention, or open_question",
						"enum": []string{"project_goal", "design", "decision", "change_reason", "bugfix", "module", "convention", "open_question"},
					},
					"title": map[string]any{
						"type": "string",
						"description": "Memory title — concise and descriptive",
					},
					"summary": map[string]any{
						"type": "string",
						"description": "One-sentence summary for search results",
					},
					"content": map[string]any{
						"type": "string",
						"description": "Full content — explain the decision, convention, or finding in detail",
					},
					"confidence": map[string]any{
						"type": "number",
						"description": "Confidence 0.0-1.0. NOTE: capped at 0.5 unless source is provided (default: 0.5)",
						"minimum": float64(0),
						"maximum": float64(1),
					},
					"status": map[string]any{
						"type": "string",
						"description": "Memory status (default: active)",
						"enum": []string{"active", "stale", "superseded", "rejected"},
					},
					"tags": map[string]any{
						"type": "array",
						"items": map[string]any{"type": "string"},
						"description": "Categorization tags",
					},
					"source": map[string]any{
						"type": "object",
						"description": "Source information. REQUIRED for confidence > 0.5.",
						"properties": map[string]any{
							"kind": map[string]any{
								"type": "string",
								"description": "Source kind",
								"enum": []string{"conversation", "file", "commit", "manual", "test", "analysis"},
							},
							"description": map[string]any{
								"type": "string",
								"description": "Human-readable source description",
							},
							"files": map[string]any{
								"type": "array",
								"items": map[string]any{"type": "string"},
							},
							"commits": map[string]any{
								"type": "array",
								"items": map[string]any{"type": "string"},
							},
						},
						"required": []string{"kind", "description"},
					},
					"scope": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"project": map[string]any{"type": "string"},
							"modules": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
					"relations": map[string]any{
						"type": "object",
						"description": "Relations to other memories, e.g. {\"supersedes\": [\"mem_20260101_001\"]}",
					},
				},
				"required": []string{"type", "title", "summary", "content"},
			},
		},
		"required": []string{"memory"},
	}, func(params map[string]any) (any, error) {
		mem, ok := params["memory"].(map[string]any)
		if !ok { return nil, fmt.Errorf("memory parameter required") }
		mu.Lock(); defer mu.Unlock()
		return svc.Remember(mem)
	})

	reg.Register("recall", "Level 2: Search memories by keyword or file path. Returns summaries only. Filter by path for file-specific context. Has more? Increase limit. Need details? Use open_memory.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search keyword or phrase. Supports both English and Chinese.",
			},
			"filters": map[string]any{
				"type": "object",
				"description": "Optional filters: status (string), paths (array of strings).",
			},
			"limit": map[string]any{"type": "integer"},
		},
	}, func(params map[string]any) (any, error) {
		query := getStr(params, "query")
		filters, _ := params["filters"].(map[string]any)
		limit := getInt(params, "limit", 3)
		mu.Lock(); defer mu.Unlock()
		results, err := svc.Recall(query, filters, limit)
		if err != nil { return nil, err }
		return map[string]any{"results": results}, nil
	})

	reg.Register("open_memory", "Level 3: Get full card content by ID. Only call after recall returns a summary you need detail on.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string"},
		},
	}, func(params map[string]any) (any, error) {
		id := getStr(params, "id")
		mu.Lock(); defer mu.Unlock()
		return svc.OpenMemory(id)
	})

	reg.Register("update_memory", "Update an existing memory card. Use to mark memories as stale, change confidence, add tags, or update relations.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "Memory card ID (e.g. 'mem_20260612_001')."},
			"updates": map[string]any{
				"type": "object",
				"description": "Fields to update: status, confidence, tags, relations.",
			},
		},
		"required": []string{"id", "updates"},
	}, func(params map[string]any) (any, error) {
		id := getStr(params, "id")
		updates, ok := params["updates"].(map[string]any)
		if !ok { return nil, fmt.Errorf("updates parameter required") }
		mu.Lock(); defer mu.Unlock()
		return svc.UpdateMemory(id, updates)
	})

	reg.Register("list_recent", "List recently created or updated memories.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{"type": "integer"},
		},
	}, func(params map[string]any) (any, error) {
		limit := getInt(params, "limit", 10)
		mu.Lock(); defer mu.Unlock()
		results, err := svc.ListRecent(limit)
		if err != nil { return nil, err }
		return map[string]any{"results": results}, nil
	})

	reg.Register("synthesize_rules", "Regenerate agent-rules.yaml from active convention and decision cards. Returns the full rules document so agents can inject them into context.", map[string]any{
		"type": "object",
		"properties": map[string]any{},
	}, func(params map[string]any) (any, error) {
		mu.Lock(); defer mu.Unlock()
		doc, err := svc.SynthesizeRules()
		if err != nil { return nil, err }
		rules := make([]map[string]any, len(doc.Rules))
		for i, r := range doc.Rules {
			rules[i] = map[string]any{
				"id": r.ID, "source_memory": r.SourceMemory,
				"title": r.Title, "category": r.Category,
				"body": r.Body, "created_at": r.CreatedAt,
			}
		}
		return map[string]any{
			"version": doc.Version,
			"synthesized_at": doc.SynthesizedAt,
			"rule_count": len(doc.Rules),
			"rules": rules,
		}, nil
	})

}

func getStr(params map[string]any, key string) string {
	if v, ok := params[key].(string); ok { return v }
	return ""
}

func getInt(params map[string]any, key string, def int) int {
	switch v := params[key].(type) {
	case float64: return int(v)
	case int: return v
	default: return def
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	html := strings.ReplaceAll(indexHTML, "{{.ProjectRoot}}", projectRoot)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func handleRecent(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	results, err := svc.ListRecent(50)
	writeJSON(w, results, err)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	q := r.URL.Query().Get("q")
	if q == "" { writeJSON(w, nil, nil); return }
	results, err := svc.Recall(q, nil, 30)
	writeJSON(w, results, err)
}

func handleOpen(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	id := r.URL.Query().Get("id")
	result, err := svc.OpenMemory(id)
	writeJSONRaw(w, result, err)
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, "POST required", 405); return }
	mu.Lock(); defer mu.Unlock()
	id := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")
	result, err := svc.UpdateMemory(id, map[string]any{"status": status})
	writeJSONRaw(w, result, err)
}

func handleProject(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	json.NewEncoder(w).Encode(map[string]any{"root": projectRoot, "recents": recents})
}

func handleProjectSet(w http.ResponseWriter, r *http.Request) {
	newRoot := r.URL.Query().Get("root")
	if newRoot == "" { http.Error(w, "root parameter required", 400); return }
	mu.Lock()
	svc = service.New(newRoot)
	svc.InitProject()
	projectRoot = newRoot
	saveRecents()
	mu.Unlock()
	json.NewEncoder(w).Encode(map[string]any{"root": projectRoot, "recents": recents})
}

func handleRecents(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	json.NewEncoder(w).Encode(map[string]any{"recents": recents})
}

func writeJSON(w http.ResponseWriter, results []map[string]any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil { json.NewEncoder(w).Encode(map[string]any{"error": err.Error()}); return }
	if results == nil { results = []map[string]any{} }
	json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func writeJSONRaw(w http.ResponseWriter, data map[string]any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil { json.NewEncoder(w).Encode(map[string]any{"error": err.Error()}); return }
	json.NewEncoder(w).Encode(data)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

// RenderIndex substitutes the project root into the embedded index.html template.
func RenderIndex(root string) string {
	return strings.ReplaceAll(indexHTML, "{{.ProjectRoot}}", root)
}
