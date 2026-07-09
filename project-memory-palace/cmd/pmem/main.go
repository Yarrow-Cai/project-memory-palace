// pmem - Project Memory Palace: unified CLI, tray, and MCP server.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/atop/project-memory-palace/internal/audit"
	"github.com/atop/project-memory-palace/internal/mcp"
	"github.com/atop/project-memory-palace/internal/service"
	"github.com/atop/project-memory-palace/internal/store"
	"github.com/atop/project-memory-palace/internal/tray"
	"gopkg.in/yaml.v3"
)

var projectRoot string

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) <= 1 {
		hideConsole()
		tray.Run(".")
		return 0
	}

	flag.StringVar(&projectRoot, "project-root", ".", "Project root directory")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: pmem [--project-root <dir>] <command> [args...]")
		fmt.Fprintln(os.Stderr, "commands: init, remember, search, open, recent, update, delete, purge, rebuild-index, audit, verify, export, serve-mcp, serve-web, synthesize-rules, disclose")
		return 1
	}

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "init":
		return cmdInit(cmdArgs)
	case "remember":
		return cmdRemember(cmdArgs)
	case "search":
		return cmdSearch(cmdArgs)
	case "open":
		return cmdOpen(cmdArgs)
	case "recent":
		return cmdRecent(cmdArgs)
	case "update":
		return cmdUpdate(cmdArgs)
	case "delete":
		return cmdDelete(cmdArgs)
	case "purge":
		return cmdPurge(cmdArgs)
	case "rebuild-index":
		return cmdRebuildIndex(cmdArgs)
	case "serve-mcp":
		return cmdServeMCP(cmdArgs)
	case "serve-web":
		return cmdServeWeb(cmdArgs)
	case "synthesize-rules":
		return cmdSynthesizeRules(cmdArgs)
	case "audit":
		return cmdAudit(cmdArgs)
	case "disclose":
		return cmdDisclose(cmdArgs)
	case "verify":
		return cmdVerify(cmdArgs)
	case "export":
		return cmdExport(cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n", cmd)
		return 1
	}
}

func newService() (*service.MemoryService, error) {
	return service.New(projectRoot), nil
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.Parse(args)
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if err := svc.InitProject(); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	tray.AddRecent(projectRoot)
	fmt.Printf("initialized: true\nproject-root: %s\n", projectRoot)
	return 0
}

func cmdRemember(args []string) int {
	fs := flag.NewFlagSet("remember", flag.ContinueOnError)
	filePath := fs.String("file", "", "Path to YAML card file")
	tmplName := fs.String("template", "", "Template name from .project-memory/templates/<name>.yaml. 模板字段填充缺失值，显式提供的字段优先级更高。")
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if *filePath == "" { fmt.Fprintln(os.Stderr, "error: --file is required"); return 1 }
	data, err := os.ReadFile(*filePath)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	var payload map[string]any
	if err := yaml.Unmarshal(data, &payload); err != nil { fmt.Fprintf(os.Stderr, "error: invalid YAML: %v\n", err); return 1 }
	if *tmplName != "" {
		payload["template"] = *tmplName
	}
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	result, err := svc.Remember(payload)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	fmt.Println(result["notification"])
	return 0
}

func cmdSearch(args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "Max results")
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if fs.NArg() < 1 { fmt.Fprintln(os.Stderr, "error: query is required"); return 1 }
	query := fs.Arg(0)
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	results, err := svc.Recall(query, nil, *limit)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	out := map[string]any{"query": query, "count": len(results), "results": results}
	data, _ := yaml.Marshal(out)
	fmt.Print(string(data))
	return 0
}

func cmdOpen(args []string) int {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if fs.NArg() < 1 { fmt.Fprintln(os.Stderr, "error: id is required"); return 1 }
	id := fs.Arg(0)
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	card, err := svc.OpenMemory(id)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	data, _ := yaml.Marshal(card)
	fmt.Print(string(data))
	return 0
}

func cmdRecent(args []string) int {
	fs := flag.NewFlagSet("recent", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "Max results")
	offset := fs.Int("offset", 0, "Offset")
	memType := fs.String("type", "", "Filter by memory type")
	status := fs.String("status", "", "Filter by status")
	priority := fs.Int("priority", 0, "Filter by minimum priority (1-5)")
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	filters := map[string]any{}
	if *memType != "" { filters["type"] = *memType }
	if *status != "" { filters["status"] = *status }
	if *priority > 0 { filters["priority"] = *priority }
	results, err := svc.ListRecent(*limit, *offset, filters)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	out := map[string]any{"count": len(results), "results": results}
	data, _ := yaml.Marshal(out)
	fmt.Print(string(data))
	return 0
}

func cmdUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	status := fs.String("status", "", "New status (active, stale, superseded, rejected)")
	confidence := fs.Float64("confidence", -1, "New confidence (0.0-1.0)")
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if fs.NArg() < 1 { fmt.Fprintln(os.Stderr, "error: id is required"); return 1 }
	id := fs.Arg(0)
	if *status == "" && *confidence == -1 { fmt.Fprintln(os.Stderr, "error: at least one of --status or --confidence is required"); return 1 }
	updates := map[string]any{}
	if *status != "" { updates["status"] = *status }
	if *confidence != -1 { updates["confidence"] = *confidence }
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	result, err := svc.UpdateMemory(id, updates)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	fmt.Println(result["notification"])
	return 0
}

func cmdDelete(args []string) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if fs.NArg() < 1 { fmt.Fprintln(os.Stderr, "error: id is required"); return 1 }
	id := fs.Arg(0)
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	result, err := svc.DeleteMemory(id)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	data, _ := yaml.Marshal(result)
	fmt.Print(string(data))
	return 0
}

func cmdPurge(args []string) int {
	fs := flag.NewFlagSet("purge", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	result, err := svc.PurgeExpired()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	data, _ := yaml.Marshal(result)
	fmt.Print(string(data))
	return 0
}

func cmdRebuildIndex(args []string) int {
	fs := flag.NewFlagSet("rebuild-index", flag.ContinueOnError)
	_ = fs.Parse(args)
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if err := svc.RebuildIndex(); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	fmt.Printf("index-rebuilt: true\nproject-root: %s\n", projectRoot)
	return 0
}

func cmdAudit(args []string) int {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	_ = fs.Parse(args)
	findings, err := audit.AuditProject(projectRoot)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	out := map[string]any{"audit": findings}
	data, _ := yaml.Marshal(out)
	fmt.Print(string(data))
	return 0
}

func cmdDisclose(args []string) int {
	mode := flag.String("mode", "first", "Disclosure mode: first or subsequent")
	since := flag.String("since", "", "ISO timestamp for subsequent mode")
	fs := flag.NewFlagSet("disclose", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if fs.NArg() > 0 { *mode = fs.Arg(0) }
	if *mode == "subsequent" && fs.NArg() > 1 { *since = fs.Arg(1) }
	if *mode == "subsequent" && *since == "" { fmt.Fprintln(os.Stderr, "error: --since is required for subsequent mode"); return 1 }

	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	results, err := svc.Disclosure(*mode, *since)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }

	out := map[string]any{"mode": *mode, "count": len(results), "results": results}
	data, _ := yaml.Marshal(out)
	fmt.Print(string(data))
	return 0
}


func cmdSynthesizeRules(args []string) int {
	fs := flag.NewFlagSet("synthesize-rules", flag.ContinueOnError)
	_ = fs.Parse(args)
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	doc, err := svc.SynthesizeRules()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	fmt.Printf("rules-synthesized: true\nproject-root: %s\nrule-count: %d\n", projectRoot, len(doc.Rules))
	return 0
}

func cmdServeMCP(args []string) int {
	if len(args) > 0 { projectRoot = args[0] }

	var ws *service.WorkspaceService
	var err error
	ws, err = service.NewWorkspace(projectRoot)
	if err != nil || len(ws.ProjectNames()) == 0 {
		ws, err = service.NewSingleProject(projectRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}

	reg := mcp.NewToolRegistry()
	service.RegisterAllTools(reg, ws, nil)

	srv := &mcp.StdioServer{Registry: reg, Reader: os.Stdin, Writer: os.Stdout}
	defer ws.Close()
	fmt.Fprintln(os.Stderr, "MCP server started (workspace mode, projects:", strings.Join(ws.ProjectNames(), ", "), ")")
	if err := srv.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "mcp error: %v\n", err)
		return 1
	}
	return 0
}


func cmdVerify(args []string) int {
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	report, err := svc.VerifyIntegrity()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	data, _ := yaml.Marshal(report)
	fmt.Print(string(data))
	return 0
}


func cmdExport(args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	format := fs.String("format", "yaml", "Output format: yaml or json")
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	cards, err := svc.ExportCards()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	var data []byte
	if *format == "json" {
		data, _ = json.MarshalIndent(map[string]any{"cards": cards, "count": len(cards)}, "", "  ")
	} else {
		data, _ = yaml.Marshal(map[string]any{"cards": cards, "count": len(cards)})
	}
	fmt.Println(string(data))
	return 0
}

func cmdServeWeb(args []string) int {
	if len(args) > 0 { projectRoot = args[0] }

	var ws *service.WorkspaceService
	var wsErr error
	ws, wsErr = service.NewWorkspace(projectRoot)
	if wsErr != nil || len(ws.ProjectNames()) == 0 {
		ws, wsErr = service.NewSingleProject(projectRoot)
		if wsErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", wsErr)
			return 1
		}
	}
	tray.AddRecent(projectRoot)

	// REST API routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(tray.RenderIndex(projectRoot)))
	})
	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	http.HandleFunc("/api/recent", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONList(w, nil, err); return }
		limit := service.ParseIntParam(r.URL.Query().Get("limit"), 20)
		offset := service.ParseIntParam(r.URL.Query().Get("offset"), 0)
		filters := map[string]any{}
		if t := r.URL.Query().Get("type"); t != "" { filters["type"] = t }
		if s := r.URL.Query().Get("status"); s != "" { filters["status"] = s }
		if p := service.ParseIntParam(r.URL.Query().Get("priority"), 0); p > 0 { filters["priority"] = p }
		results, err := svc.ListRecent(limit, offset, filters)
		writeWebJSONList(w, results, err)
	})
	http.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONList(w, nil, err); return }
		q := r.URL.Query().Get("q")
		if q == "" { writeWebJSONList(w, nil, nil); return }
		results, err := svc.Recall(q, nil, 30)
		writeWebJSONList(w, results, err)
	})
	http.HandleFunc("/api/context", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONList(w, nil, err); return }
		pathStr := r.URL.Query().Get("paths")
		if pathStr == "" { writeWebJSONList(w, nil, nil); return }
		paths := strings.Split(pathStr, ",")
		limit := service.ParseIntParam(r.URL.Query().Get("limit"), 20)
		results, err := svc.ContextForFiles(paths, limit)
		writeWebJSONList(w, results, err)
	})
	http.HandleFunc("/api/hot", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONList(w, nil, err); return }
		limit := service.ParseIntParam(r.URL.Query().Get("limit"), 20)
		results, err := svc.HotMemories(limit)
		writeWebJSONList(w, results, err)
	})
http.HandleFunc("/api/decay", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONList(w, nil, err); return }
		limit := service.ParseIntParam(r.URL.Query().Get("limit"), 50)
		results, err := svc.DecayMemories(limit)
		writeWebJSONList(w, results, err)
	})
	http.HandleFunc("/api/open", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONRaw(w, nil, err); return }
		id := r.URL.Query().Get("id")
		result, err := svc.OpenMemory(id)
		writeWebJSONRaw(w, result, err)
	})
	http.HandleFunc("/api/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeWebJSONRaw(w, nil, fmt.Errorf("POST required"))
			return
		}
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONRaw(w, nil, err); return }
		id := r.URL.Query().Get("id")
		updates := map[string]any{}
		if s := r.URL.Query().Get("status"); s != "" { updates["status"] = s }
		if s := r.URL.Query().Get("confidence"); s != "" {
			if f, err := strconv.ParseFloat(s, 64); err == nil { updates["confidence"] = f }
		}
		if s := r.URL.Query().Get("tags"); s != "" { updates["tags"] = strings.Split(s, ",") }
		if s := r.URL.Query().Get("source_agent"); s != "" { updates["source_agent"] = s }
		if s := r.URL.Query().Get("knowledge_kind"); s != "" { updates["knowledge_kind"] = s }
		if s := r.URL.Query().Get("expires_at"); s != "" { updates["expires_at"] = s }
		if s := r.URL.Query().Get("priority"); s != "" {
			if p, err := strconv.Atoi(s); err == nil { updates["priority"] = p }
		}
		result, err := svc.UpdateMemory(id, updates)
		writeWebJSONRaw(w, result, err)
	})
	http.HandleFunc("/api/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeWebJSONRaw(w, nil, fmt.Errorf("POST required"))
			return
		}
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONRaw(w, nil, err); return }
		id := r.URL.Query().Get("id")
		if id == "" { writeWebJSONRaw(w, nil, fmt.Errorf("id parameter required")); return }
		result, err := svc.DeleteMemory(id)
		writeWebJSONRaw(w, result, err)
	})
	http.HandleFunc("/api/purge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeWebJSONRaw(w, nil, fmt.Errorf("POST required"))
			return
		}
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONRaw(w, nil, err); return }
		result, err := svc.PurgeExpired()
		writeWebJSONRaw(w, result, err)
	})
	http.HandleFunc("/api/vacuum", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeWebJSONRaw(w, nil, fmt.Errorf("POST required"))
			return
		}
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONRaw(w, nil, err); return }
		if err := svc.Vacuum(); err != nil {
			writeWebJSONRaw(w, nil, err)
			return
		}
		writeWebJSONRaw(w, map[string]any{"status": "ok", "message": "database vacuumed successfully"}, nil)
	})
	http.HandleFunc("/api/count", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONRaw(w, nil, err); return }
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
	})
	http.HandleFunc("/api/disclosure", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONList(w, nil, err); return }
		mode := r.URL.Query().Get("mode")
		since := r.URL.Query().Get("since")
		results, err := svc.Disclosure(mode, since)
		writeWebJSONList(w, results, err)
	})
	http.HandleFunc("/api/project/remove", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeWebJSONRaw(w, nil, fmt.Errorf("POST required"))
			return
		}
		root := r.URL.Query().Get("root")
		if root == "" {
			writeWebJSONRaw(w, nil, fmt.Errorf("root parameter required"))
			return
		}
		tray.RemoveRecent(root)
		memDir := store.MemoryDir(root)
		if err := os.RemoveAll(memDir); err != nil {
			writeWebJSONRaw(w, nil, fmt.Errorf("failed to remove project data: %w", err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"removed": root, "recents": tray.RecentList()})
	})
http.HandleFunc("/api/project", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"root": projectRoot, "recents": tray.RecentList()})
	})
	http.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		projects, err := ws.ListProjects()
		writeWebJSONList(w, projects, err)
	})
	http.HandleFunc("/api/patterns", func(w http.ResponseWriter, r *http.Request) {
		minProjects := service.ParseIntParam(r.URL.Query().Get("min_projects"), 2)
		results, err := ws.ExtractPatterns(minProjects)
		writeWebJSONList(w, results, err)
	})
	http.HandleFunc("/api/project/set", func(w http.ResponseWriter, r *http.Request) {
		newRoot := r.URL.Query().Get("root")
		if newRoot == "" {
			writeWebJSONRaw(w, nil, fmt.Errorf("root parameter required"))
			return
		}
		projectRoot = newRoot
		ws, _ = service.NewWorkspace(newRoot)
		if ws == nil || len(ws.ProjectNames()) == 0 {
			ws, _ = service.NewSingleProject(newRoot)
		}
		tray.AddRecent(newRoot)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"root": projectRoot, "recents": tray.RecentList()})
	})
	http.HandleFunc("/api/projects/recent", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"recents": tray.RecentList()})
	})
http.HandleFunc("/api/rules", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONRaw(w, nil, err); return }
		yamlPath := store.RulesPath(svc.ProjectRoot())
		data, yamlErr := os.ReadFile(yamlPath)
		mdPath := strings.TrimSuffix(yamlPath, ".yaml") + ".md"
		mdData, mdErr := os.ReadFile(mdPath)
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{"yaml_exists": yamlErr == nil}
		if yamlErr == nil { response["yaml"] = string(data) }
		if mdErr == nil { response["markdown"] = string(mdData) }
		json.NewEncoder(w).Encode(response)
	})
	http.HandleFunc("/api/relations", func(w http.ResponseWriter, r *http.Request) {
		svc, _, err := ws.Resolve("")
		if err != nil { writeWebJSONRaw(w, nil, err); return }
		id := r.URL.Query().Get("id")
		if id == "" { writeWebJSONRaw(w, nil, fmt.Errorf("id parameter required")); return }
		direction := r.URL.Query().Get("direction")
		if direction == "" { direction = "both" }
		depth := service.ParseIntParam(r.URL.Query().Get("depth"), 1)
		result, err := svc.GetRelations(id, direction, depth)
		writeWebJSONRaw(w, result, err)
	})

	http.HandleFunc("/api/coverage", func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		if project != "" {
			svc, _, err := ws.Resolve(project)
			if err != nil { writeWebJSONRaw(w, nil, err); return }
			results, err := svc.CoverageStats()
			writeWebJSONList(w, results, err)
			return
		}
		result, err := ws.WorkspaceCoverage()
		writeWebJSONRaw(w, result, err)
	})

	http.HandleFunc("/api/workspace/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeWebJSONRaw(w, nil, fmt.Errorf("POST required"))
			return
		}
		added := ws.RefreshWorkspace()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"added":          added,
			"total_projects": len(ws.ProjectNames()),
		})
	})

	fmt.Fprintf(os.Stderr, "Web UI server starting at http://127.0.0.1:8147\n")
	fmt.Fprintf(os.Stderr, "Project root: %s\n", projectRoot)
	defer ws.Close()
	if err := http.ListenAndServe("127.0.0.1:8147", nil); err != nil {
		log.Printf("HTTP server error: %v", err)
		return 1
	}
	return 0
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

