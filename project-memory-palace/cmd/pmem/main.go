// pmem - Project Memory Palace: unified CLI, tray, and MCP server.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"syscall"

	"github.com/atop/project-memory-palace/internal/audit"
	"github.com/atop/project-memory-palace/internal/mcp"
	"github.com/atop/project-memory-palace/internal/service"
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
		fmt.Fprintln(os.Stderr, "commands: init, remember, search, open, recent, update, rebuild-index, audit, serve-mcp, serve-web, synthesize-rules")
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
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	if *filePath == "" { fmt.Fprintln(os.Stderr, "error: --file is required"); return 1 }
	data, err := os.ReadFile(*filePath)
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	var payload map[string]any
	if err := yaml.Unmarshal(data, &payload); err != nil { fmt.Fprintf(os.Stderr, "error: invalid YAML: %v\n", err); return 1 }
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
	if err := fs.Parse(args); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	results, err := svc.ListRecent(*limit)
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

func hideConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	freeConsole := kernel32.NewProc("FreeConsole")
	freeConsole.Call()
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
	projectRoot = "."
	if len(args) > 0 { projectRoot = args[0] }
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	svc.InitProject()
	tray.AddRecent(projectRoot)

	reg := mcp.NewToolRegistry()
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
		return svc.Remember(mem)
	})
	reg.Register("recall", "Retrieve relevant memory summaries.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
			"filters": map[string]any{"type": "object"},
			"limit": map[string]any{"type": "integer"},
		},
	}, func(params map[string]any) (any, error) {
		query := ""
		if v, ok := params["query"].(string); ok { query = v }
		filters, _ := params["filters"].(map[string]any)
		limit := 5
		if v, ok := params["limit"].(float64); ok { limit = int(v) }
		results, err := svc.Recall(query, filters, limit)
		if err != nil { return nil, err }
		return map[string]any{"results": results}, nil
	})
	reg.Register("open_memory", "Open one full memory card by ID.", map[string]any{
		"type": "object",
		"properties": map[string]any{"id": map[string]any{"type": "string"}},
	}, func(params map[string]any) (any, error) {
		id := ""
		if v, ok := params["id"].(string); ok { id = v }
		return svc.OpenMemory(id)
	})
	reg.Register("update_memory", "Update an existing memory card.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string"},
			"updates": map[string]any{"type": "object"},
		},
	}, func(params map[string]any) (any, error) {
		id := ""
		if v, ok := params["id"].(string); ok { id = v }
		updates, ok := params["updates"].(map[string]any)
		if !ok { return nil, fmt.Errorf("updates parameter required") }
		return svc.UpdateMemory(id, updates)
	})
	reg.Register("list_recent", "List recently created or updated memories.", map[string]any{
		"type": "object",
		"properties": map[string]any{"limit": map[string]any{"type": "integer"}},
	}, func(params map[string]any) (any, error) {
		limit := 10
		if v, ok := params["limit"].(float64); ok { limit = int(v) }
		results, err := svc.ListRecent(limit)
		if err != nil { return nil, err }
		return map[string]any{"results": results}, nil
	})
	reg.Register("synthesize_rules", "Regenerate agent-rules.yaml from active convention and decision cards. Returns the full rules document so agents can inject them into context.", map[string]any{
		"type": "object", "properties": map[string]any{},
	}, func(params map[string]any) (any, error) {
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

	srv := &mcp.StdioServer{Registry: reg, Reader: os.Stdin, Writer: os.Stdout}
	fmt.Fprintln(os.Stderr, "MCP server started")
	if err := srv.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "mcp error: %v\n", err)
		return 1
	}
	return 0
}

func cmdServeWeb(args []string) int {
	projectRoot = "."
	if len(args) > 0 { projectRoot = args[0] }
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	svc.InitProject()
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
		results, err := svc.ListRecent(50)
		writeWebJSONList(w, results, err)
	})
	http.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" { writeWebJSONList(w, nil, nil); return }
		results, err := svc.Recall(q, nil, 30)
		writeWebJSONList(w, results, err)
	})
	http.HandleFunc("/api/open", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		result, err := svc.OpenMemory(id)
		writeWebJSONRaw(w, result, err)
	})
	http.HandleFunc("/api/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		id := r.URL.Query().Get("id")
		status := r.URL.Query().Get("status")
		result, err := svc.UpdateMemory(id, map[string]any{"status": status})
		writeWebJSONRaw(w, result, err)
	})
	http.HandleFunc("/api/project", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"root": projectRoot, "recents": tray.RecentList()})
	})
	http.HandleFunc("/api/project/set", func(w http.ResponseWriter, r *http.Request) {
		newRoot := r.URL.Query().Get("root")
		if newRoot == "" { http.Error(w, "root parameter required", 400); return }
		projectRoot = newRoot
		svc = service.New(projectRoot)
		svc.InitProject()
		tray.AddRecent(newRoot)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"root": projectRoot, "recents": tray.RecentList()})
	})
	http.HandleFunc("/api/projects/recent", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"recents": tray.RecentList()})
	})

	fmt.Fprintf(os.Stderr, "Web UI server starting at http://127.0.0.1:8147\n")
	fmt.Fprintf(os.Stderr, "Project root: %s\n", projectRoot)
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
