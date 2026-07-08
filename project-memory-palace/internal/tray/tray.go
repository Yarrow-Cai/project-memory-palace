package tray

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"


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

// RemoveRecent removes a project root from the recents list and persists.
func RemoveRecent(root string) {
	mu.Lock()
	defer mu.Unlock()
	root = filepath.Clean(root)
	var filtered []string
	for _, r := range recents {
		if r != root {
			filtered = append(filtered, r)
		}
	}
	recents = filtered
	data, _ := json.Marshal(recents)
	os.MkdirAll(filepath.Dir(recentsPath), 0700)
	os.WriteFile(recentsPath, data, 0644)
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
				openBrowser("http://127.0.0.1:8147")
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
	ws, wsErr := service.NewWorkspace(projectRoot)
	if wsErr != nil || len(ws.ProjectNames()) == 0 {
		ws, _ = service.NewSingleProject(projectRoot)
	}
	service.RegisterAllTools(reg, ws, func(h mcp.ToolHandler) mcp.ToolHandler {
		return func(params map[string]any) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			return h(params)
		}
	})
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
	http.HandleFunc("/api/project/remove", handleProjectRemove)
	http.HandleFunc("/api/projects/recent", handleRecents)
	http.HandleFunc("/api/rules", handleRules)
	http.HandleFunc("/api/count", handleCount)
	http.HandleFunc("/api/disclosure", handleDisclosure)
	fmt.Fprintln(os.Stderr, "API + SSE server started on 127.0.0.1:8147")
	if err := http.ListenAndServe("127.0.0.1:8147", nil); err != nil {
		log.Printf("HTTP server error: %v", err)
	}
}
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default:
		cmd = "xdg-open"
	}
	exec.Command(cmd, append(args, url)...).Start()
}

func parseIntParam(s string, defaultVal int) int {
	if s == "" { return defaultVal }
	n := 0
	for _, c := range s { if c < '0' || c > '9' { return defaultVal }; n = n*10 + int(c-'0') }
	return n
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	html := RenderIndex(projectRoot)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func handleRecent(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	limit := service.ParseIntParam(r.URL.Query().Get("limit"), 20)
	offset := parseIntParam(r.URL.Query().Get("offset"), 0)
	filters := map[string]any{}
	if t := r.URL.Query().Get("type"); t != "" { filters["type"] = t }
	if s := r.URL.Query().Get("status"); s != "" { filters["status"] = s }
	if p := service.ParseIntParam(r.URL.Query().Get("priority"), 0); p > 0 { filters["priority"] = p }
	results, err := svc.ListRecent(limit, offset, filters)
	writeWebJSONList(w, results, err)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	q := r.URL.Query().Get("q")
	if q == "" { writeWebJSONList(w, nil, nil); return }
	results, err := svc.Recall(q, nil, 30)
	writeWebJSONList(w, results, err)
}

func handleOpen(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	id := r.URL.Query().Get("id")
	result, err := svc.OpenMemory(id)
	writeWebJSONRaw(w, result, err)
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeWebJSONRaw(w, nil, fmt.Errorf("POST required"))
		return
	}
	mu.Lock(); defer mu.Unlock()
	id := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")
	result, err := svc.UpdateMemory(id, map[string]any{"status": status})
	writeWebJSONRaw(w, result, err)
}

func handleProject(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	writeWebJSONRaw(w, map[string]any{"root": projectRoot, "recents": recents}, nil)
}

func handleProjectSet(w http.ResponseWriter, r *http.Request) {
	newRoot := r.URL.Query().Get("root")
	if newRoot == "" {
		writeWebJSONRaw(w, nil, fmt.Errorf("root parameter required"))
		return
	}
	mu.Lock()
	svc = service.New(newRoot)
	svc.InitProject()
	projectRoot = newRoot
	saveRecents()
	mu.Unlock()
	writeWebJSONRaw(w, map[string]any{"root": projectRoot, "recents": recents}, nil)
}

func handleProjectRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeWebJSONRaw(w, nil, fmt.Errorf("POST required"))
		return
	}
	root := r.URL.Query().Get("root")
	if root == "" {
		writeWebJSONRaw(w, nil, fmt.Errorf("root parameter required"))
		return
	}
	RemoveRecent(root)
	// 閻喐顒滈崚鐘绘珟妞ゅ湱娲伴惃?.project-memory/ 閺佺増宓侀惄顔肩秿
	memDir := store.MemoryDir(root)
	os.RemoveAll(memDir)
	writeWebJSONRaw(w, map[string]any{"removed": root, "recents": recents}, nil)
}

func handleCount(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	filters := map[string]any{}
	if t := r.URL.Query().Get("type"); t != "" { filters["type"] = t }
	if s := r.URL.Query().Get("status"); s != "" { filters["status"] = s }
	if p := service.ParseIntParam(r.URL.Query().Get("priority"), 0); p > 0 { filters["priority"] = p }
	count, err := svc.Count(filters)
	if err != nil {
		writeWebJSONRaw(w, nil, err)
		return
	}
	writeWebJSONRaw(w, map[string]any{"count": count}, nil)
}

func handleRecents(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	json.NewEncoder(w).Encode(map[string]any{"recents": recents})
}

func handleRules(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	data, err := os.ReadFile(store.RulesPath(svc.ProjectRoot()))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "rules not found"})
		return
	}
	mdPath := store.RulesPath(svc.ProjectRoot())
	mdPath = mdPath[:len(mdPath)-len(".yaml")] + ".md"
	mdData, mdErr := os.ReadFile(mdPath)
	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{}
	if err == nil { response["yaml"] = string(data) }
	if mdErr == nil { response["markdown"] = string(mdData) }
	json.NewEncoder(w).Encode(response)
}

func handleDisclosure(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	mode := r.URL.Query().Get("mode")
	since := r.URL.Query().Get("since")
	var results []map[string]any
	var err error
	switch mode {
	case "first":
		results, err = svc.ListRecent(20, 0, map[string]any{"status": "active", "priority": 3})
	case "subsequent":
		highPri, e1 := svc.ListRecent(15, 0, map[string]any{"status": "active", "priority": 5})
		recent, e2 := svc.ListRecent(15, 0, map[string]any{"status": "active"})
		if e1 != nil { err = e1 }
		if e2 != nil { err = e2 }
		seen := map[string]bool{}
		for _, r := range highPri {
			seen[r["id"].(string)] = true
			results = append(results, r)
		}
		for _, r := range recent {
			if !seen[r["id"].(string)] {
				if since == "" || (r["updated_at"] != nil && service.IsAfterTime(fmt.Sprint(r["updated_at"]), since)) {
					results = append(results, r)
				}
			}
		}
		if len(results) > 15 { results = results[:15] }
	default:
		writeWebJSONRaw(w, nil, fmt.Errorf("mode must be 'first' or 'subsequent'"))
		return
	}
	writeWebJSONList(w, results, err)
}

func writeWebJSONList(w http.ResponseWriter, results []map[string]any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil { json.NewEncoder(w).Encode(map[string]any{"error": err.Error()}); return }
	if results == nil { results = []map[string]any{} }
	json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func writeWebJSONRaw(w http.ResponseWriter, data map[string]any, err error) {
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
	// Try filesystem first for dev, fall back to embedded
	exe, _ := os.Executable()
	for _, p := range []string{
		filepath.Join(filepath.Dir(exe), "..", "web", "index.html"),
		"web/index.html",
	} {
		if b, err := os.ReadFile(p); err == nil && len(b) > 100 {
			return strings.ReplaceAll(string(b), "{{.ProjectRoot}}", root)
		}
	}
	return strings.ReplaceAll(indexHTML, "{{.ProjectRoot}}", root)
}
