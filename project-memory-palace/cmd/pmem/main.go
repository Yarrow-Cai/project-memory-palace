// pmem - Project Memory Palace: unified CLI, tray, and MCP server.
package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"

	"github.com/atop/project-memory-palace/internal/audit"
	"github.com/atop/project-memory-palace/internal/service"
	"github.com/atop/project-memory-palace/internal/tray"
	"gopkg.in/yaml.v3"
	"github.com/atop/project-memory-palace/internal/mcp"
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
		fmt.Fprintln(os.Stderr, "commands: init, remember, search, open, recent, update, rebuild-index, audit")
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
	if err := svc.SynthesizeRules(); err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	fmt.Printf("rules-synthesized: true\nproject-root: %s\n", projectRoot)
	return 0
}

func cmdServeMCP(args []string) int {
	projectRoot = "."
	if len(args) > 0 { projectRoot = args[0] }
	svc, err := newService()
	if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); return 1 }
	svc.InitProject()

	reg := mcp.NewToolRegistry()
	reg.Register("remember", "Write one durable project memory card.", map[string]any{
		"type": "object",
		"properties": map[string]any{"memory": map[string]any{"type": "object"}},
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
	reg.Register("synthesize_rules", "Regenerate agent-rules.yaml from active cards.", map[string]any{
		"type": "object", "properties": map[string]any{},
	}, func(params map[string]any) (any, error) {
		if err := svc.SynthesizeRules(); err != nil { return nil, err }
		return map[string]any{"status": "ok"}, nil
	})

	srv := &mcp.StdioServer{Registry: reg, Reader: os.Stdin, Writer: os.Stdout}
	for {
		if err := srv.HandleOne(); err != nil {
			if err.Error() == "EOF" { return 0 }
			fmt.Fprintf(os.Stderr, "mcp error: %v\n", err)
			return 1
		}
	}
}
