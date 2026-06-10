// pmem-mcp exposes Project Memory Palace tools via MCP over stdio.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/atop/project-memory-palace/internal/mcp"
	"github.com/atop/project-memory-palace/internal/service"
	"github.com/atop/project-memory-palace/internal/store"
)

func main() {
	projectRoot := "."
	if len(os.Args) > 1 { projectRoot = os.Args[1] }

	reg := mcp.NewToolRegistry()

	reg.Register("remember", "Write one durable project memory card.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_root": map[string]any{"type": "string"},
			"memory": map[string]any{"type": "object"},
		},
	}, func(params map[string]any) (any, error) {
		root := getRoot(params, projectRoot)
		mem, ok := params["memory"].(map[string]any)
		if !ok { return nil, fmt.Errorf("memory parameter required") }
		return service.New(root).Remember(mem)
	})

	reg.Register("recall", "Retrieve relevant memory summaries.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_root": map[string]any{"type": "string"},
			"query": map[string]any{"type": "string"},
			"filters": map[string]any{"type": "object"},
			"limit": map[string]any{"type": "integer"},
		},
	}, func(params map[string]any) (any, error) {
		root := getRoot(params, projectRoot)
		query := getStr(params, "query")
		filters, _ := params["filters"].(map[string]any)
		limit := getInt(params, "limit", 5)
		store.AssertMemoryLayout(root)
		results, err := service.New(root).Recall(query, filters, limit)
		if err != nil { return nil, err }
		return map[string]any{"results": results}, nil
	})

	reg.Register("open_memory", "Open one full memory card by ID.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_root": map[string]any{"type": "string"},
			"id": map[string]any{"type": "string"},
		},
	}, func(params map[string]any) (any, error) {
		root := getRoot(params, projectRoot)
		id := getStr(params, "id")
		store.AssertMemoryLayout(root)
		return service.New(root).OpenMemory(id)
	})

	reg.Register("update_memory", "Update an existing memory card.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_root": map[string]any{"type": "string"},
			"id": map[string]any{"type": "string"},
			"updates": map[string]any{"type": "object"},
		},
	}, func(params map[string]any) (any, error) {
		root := getRoot(params, projectRoot)
		id := getStr(params, "id")
		updates, ok := params["updates"].(map[string]any)
		if !ok { return nil, fmt.Errorf("updates parameter required") }
		store.AssertMemoryLayout(root)
		return service.New(root).UpdateMemory(id, updates)
	})

	reg.Register("list_recent", "List recently created or updated memories.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_root": map[string]any{"type": "string"},
			"limit": map[string]any{"type": "integer"},
		},
	}, func(params map[string]any) (any, error) {
		root := getRoot(params, projectRoot)
		limit := getInt(params, "limit", 10)
		store.AssertMemoryLayout(root)
		results, err := service.New(root).ListRecent(limit)
		if err != nil { return nil, err }
		return map[string]any{"results": results}, nil
	})

	fmt.Fprintln(os.Stderr, "MCP server started")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		var raw map[string]any
		json.Unmarshal(line, &raw)

		// Handle initialize
		if raw["method"] == "initialize" {
			id, _ := raw["id"].(json.Number)
			resp := mcp.NewResponse(id, map[string]any{
				"protocolVersion": "0.1.0",
				"capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{"name": "project-memory-palace", "version": "0.2.0"},
			})
			data, _ := json.Marshal(resp)
			os.Stdout.Write(append(data, '\n'))
			continue
		}

		srv := &mcp.StdioServer{Registry: reg, Reader: os.Stdin, Writer: os.Stdout}
		if err := srv.HandleOne(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			// Re-initialize scanner after HandleOne consumed a line
			scanner = bufio.NewScanner(os.Stdin)
		}
	}
}

func getRoot(params map[string]any, def string) string {
	if r, ok := params["project_root"].(string); ok && r != "" { return r }
	return def
}

func getStr(params map[string]any, key string) string {
	if v, ok := params[key].(string); ok { return v }
	return ""
}

func getInt(params map[string]any, key string, def int) int {
	switch v := params[key].(type) {
	case float64: return int(v)
	case int: return v
	case json.Number: n, _ := v.Int64(); return int(n)
	default: return def
	}
}

