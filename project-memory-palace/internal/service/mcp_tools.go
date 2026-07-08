package service

import (
	"fmt"
	"os"
	"time"

	"github.com/atop/project-memory-palace/internal/mcp"
	"github.com/atop/project-memory-palace/internal/memory"
	"github.com/atop/project-memory-palace/internal/store"
	"gopkg.in/yaml.v3"
)

// RegisterAllTools registers all MCP tools with the provided registry.
// Each handler is optionally wrapped with wrapHandler (tray layer uses this
// to inject mu.Lock()/defer mu.Unlock() around every handler call).
func RegisterAllTools(reg *mcp.ToolRegistry, svc *MemoryService, projectRoot string, wrapHandler func(mcp.ToolHandler) mcp.ToolHandler) {
	wrap := func(h mcp.ToolHandler) mcp.ToolHandler {
		if wrapHandler != nil {
			return wrapHandler(h)
		}
		return h
	}

	typeEnum := memory.SortedKeys(memory.MemoryTypes)
	statusEnum := memory.SortedKeys(memory.MemoryStatuses)
	sourceKindEnum := memory.SortedKeys(memory.SourceKinds)

	// 1. init_project
	reg.Register("init_project", "Initialize Project Memory Palace for this project. Call this FIRST — returns project context (active rules, recent activity, next-step guide) in one shot. Creates .project-memory/ directory tree. Safe to call repeatedly (idempotent).", map[string]any{
		"type": "object", "properties": map[string]any{},
	}, wrap(func(params map[string]any) (any, error) {
		if err := svc.InitProject(); err != nil {
			return nil, err
		}
		result := map[string]any{"status": "initialized", "project_root": projectRoot}
		// Include active rules inline
		data, err := os.ReadFile(store.RulesPath(svc.ProjectRoot()))
		if err == nil {
			var doc map[string]any
			if yaml.Unmarshal(data, &doc) == nil {
				result["rules"] = doc["rules"]
			}
		}
		recent, _ := svc.ListRecent(5, 0, nil)
		result["recent"] = recent
		result["next"] = []string{
			"1. recall query=<keyword> - search project memory by topic or file path",
			"2. open_memory id=<id> - get full card details when summary is not enough",
			"3. remember memory={...} - persist new knowledge after completing work",
		}
		return result, nil
	}))

	// 2. remember
	reg.Register("remember", "Write one durable project memory card. Required: type, title, summary, content. Include source to achieve confidence > 0.5.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"memory": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{
						"type":        "string",
				"description": "Memory type",
				"enum":        typeEnum,
			},
			"source_agent": map[string]any{
				"type":        "string",
				"description": "创建此记忆的 AI agent 标识 (如 claude-code, codex-cli, hermes-agent)",
			},
			"knowledge_kind": map[string]any{
				"type":        "string",
				"description": "知识类型: fact(事实/不会过时), interpretation(解释/可能过时), rule(规则/应遵循)",
				"enum":        []string{"fact", "interpretation", "rule"},
			},
			"title": map[string]any{
						"type":        "string",
						"description": "Memory title — concise and descriptive",
					},
					"summary": map[string]any{
						"type":        "string",
						"description": "One-sentence summary for search results",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Full content — explain the decision, convention, or finding in detail",
					},
					"confidence": map[string]any{
						"type":        "number",
						"description": "Confidence 0.0-1.0. NOTE: capped at 0.5 unless source is provided (default: 0.5)",
						"minimum":     float64(0),
						"maximum":     float64(1),
					},
					"status": map[string]any{
						"type":        "string",
						"description": "Memory status (default: active)",
						"enum":        statusEnum,
					},
					"tags": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Categorization tags",
					},
					"source": map[string]any{
						"type":        "object",
						"description": "Source information. REQUIRED for confidence > 0.5.",
						"properties": map[string]any{
							"kind": map[string]any{
								"type":        "string",
								"description": "Source kind",
								"enum":        sourceKindEnum,
							},
							"description": map[string]any{
								"type":        "string",
								"description": "Human-readable source description",
							},
							"files": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
							"commits": map[string]any{
								"type":  "array",
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
							"paths":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
					"relations": map[string]any{
						"type":        "object",
						"description": "Relations to other memories, e.g. {\"supersedes\": [\"mem_20260101_001\"]}",
					},
				},
				"required": []string{"type", "title", "summary", "content"},
			},
		},
		"required": []string{"memory"},
	}, wrap(func(params map[string]any) (any, error) {
		mem, ok := params["memory"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("memory parameter required")
		}
		return svc.Remember(mem)
	}))

	// 3. recall
	reg.Register("recall", "Search memories (Level 2 disclosure). Returns short summaries only. Filter by paths to get file-specific context. Need details? Use open_memory.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search keyword or phrase. Supports both English and Chinese. Use concise terms for best results (e.g. 'PWM', '中断', '逆变器').",
			},
			"filters": map[string]any{
				"type":        "object",
				"description": "Optional filters: status (string), paths (string or array of strings to filter by file path).",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "description": "Filter by memory status."},
					"paths":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Filter by file paths associated with the memory."},
				},
			},
			"limit": map[string]any{"type": "integer"},
		},
	}, wrap(func(params map[string]any) (any, error) {
		query := ""
		if v, ok := params["query"].(string); ok {
			query = v
		}
		filters, _ := params["filters"].(map[string]any)
		limit := 3
		if v, ok := params["limit"].(float64); ok {
			limit = int(v)
		}
		results, err := svc.Recall(query, filters, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"results": results}, nil
	}))

	// 4. open_memory
	reg.Register("open_memory", "Level 3 disclosure: Get full card content by ID. Only call after recall returns a summary you need more detail on.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string"},
		},
	}, wrap(func(params map[string]any) (any, error) {
		id := ""
		if v, ok := params["id"].(string); ok {
			id = v
		}
		return svc.OpenMemory(id)
	}))

	// 5. update_memory
	reg.Register("update_memory", "Update an existing memory card. Use to mark memories as stale, change confidence, add tags, update relations, or set expires_at.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "Memory card ID (e.g. 'mem_20260612_001')."},
			"updates": map[string]any{
				"type":        "object",
				"description": "Fields to update: status, confidence (0.0-1.0), tags (string array), relations (object), expires_at (ISO timestamp).",
			},
		},
		"required": []string{"id", "updates"},
	}, wrap(func(params map[string]any) (any, error) {
		id := ""
		if v, ok := params["id"].(string); ok {
			id = v
		}
		updates, ok := params["updates"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("updates parameter required")
		}
		return svc.UpdateMemory(id, updates)
	}))

	// 6. delete_memory
	reg.Register("delete_memory", "Delete a single memory card permanently. Removes both the YAML file and the SQLite index entry.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "Memory card ID to delete (e.g. 'mem_20260612_001')."},
		},
		"required": []string{"id"},
	}, wrap(func(params map[string]any) (any, error) {
		id := ""
		if v, ok := params["id"].(string); ok {
			id = v
		}
		if id == "" {
			return nil, fmt.Errorf("id parameter required")
		}
		return svc.DeleteMemory(id)
	}))

	// 7. list_recent
	reg.Register("list_recent", "List recently created or updated memories.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{"type": "integer"},
		},
	}, wrap(func(params map[string]any) (any, error) {
		limit := 10
		if v, ok := params["limit"].(float64); ok {
			limit = int(v)
		}
		results, err := svc.ListRecent(limit, 0, nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{"results": results}, nil
	}))

	// 8. synthesize_rules
	reg.Register("synthesize_rules", "Regenerate agent-rules.yaml from active convention and decision cards. Returns the full rules document. NOTE: init_project already returns the latest rules — use this only when you have written new memories and need fresh rules.", map[string]any{
		"type": "object", "properties": map[string]any{},
	}, wrap(func(params map[string]any) (any, error) {
		doc, err := svc.SynthesizeRules()
		if err != nil {
			return nil, err
		}
		rules := make([]map[string]any, len(doc.Rules))
		for i, r := range doc.Rules {
			rules[i] = map[string]any{
				"id":            r.ID,
				"source_memory": r.SourceMemory,
				"title":         r.Title,
				"category":      r.Category,
				"body":          r.Body,
				"created_at":    r.CreatedAt,
			}
		}
		return map[string]any{
			"version":        doc.Version,
			"synthesized_at": doc.SynthesizedAt,
			"rule_count":     len(doc.Rules),
			"rules":          rules,
		}, nil
	}))

	// 9. disclosure
	reg.Register("disclosure", "渐进披露项目记忆。first模式返回高优先级(>=3)全貌；subsequent模式返回核心(>=5)+近期变更。减少上下文占用。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"type":        "string",
				"description": "Disclosure mode: 'first' or 'subsequent'",
				"enum":        []string{"first", "subsequent"},
			},
			"since": map[string]any{
				"type":        "string",
				"description": "ISO timestamp (required for subsequent mode)",
			},
		},
		"required": []string{"mode"},
	}, wrap(func(params map[string]any) (any, error) {
		mode, _ := params["mode"].(string)
		since, _ := params["since"].(string)
		var results []map[string]any
		switch mode {
		case "first":
			r, err := svc.ListRecent(20, 0, map[string]any{"status": "active", "priority": 3})
			if err != nil {
				return nil, err
			}
			results = r
		case "subsequent":
			highPri, e1 := svc.ListRecent(15, 0, map[string]any{"status": "active", "priority": 5})
			recent, e2 := svc.ListRecent(15, 0, map[string]any{"status": "active"})
			if e1 != nil {
				return nil, e1
			}
			if e2 != nil {
				return nil, e2
			}
			seen := map[string]bool{}
			for _, r := range highPri {
				seen[r["id"].(string)] = true
				results = append(results, r)
			}
			for _, r := range recent {
				if !seen[r["id"].(string)] {
					if since == "" || (r["updated_at"] != nil && IsAfterTime(fmt.Sprint(r["updated_at"]), since)) {
						results = append(results, r)
					}
				}
			}
			if len(results) > 15 {
				results = results[:15]
			}
		default:
			return nil, fmt.Errorf("mode must be 'first' or 'subsequent'")
		}
		return map[string]any{"mode": mode, "results": results}, nil
	}))

	// 10. check_rules_freshness
	reg.Register("check_rules_freshness", "Check if the synthesized rules are stale (i.e., newer convention/decision cards exist that have not been reflected in agent-rules.yaml). Returns stale status and the count of newer cards.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"since": map[string]any{
				"type":        "string",
				"description": "Optional: ISO timestamp to check freshness against. If omitted, uses the rules file synthesized_at timestamp.",
			},
		},
	}, wrap(func(params map[string]any) (any, error) {
		since, _ := params["since"].(string)
		if since == "" {
			// Try to read synthesized_at from rules file
			data, err := os.ReadFile(store.RulesPath(svc.ProjectRoot()))
			if err != nil {
				return map[string]any{"stale": true, "error": "no rules file found"}, nil
			}
			var doc struct {
				SynthesizedAt string `yaml:"synthesized_at"`
			}
			if yaml.Unmarshal(data, &doc) != nil || doc.SynthesizedAt == "" {
				return map[string]any{"stale": true, "error": "could not parse synthesized_at"}, nil
			}
			since = doc.SynthesizedAt
		}
		// List recent memories and check for convention/decision cards newer than since
		recent, err := svc.ListRecent(50, 0, map[string]any{"status": "active"})
		if err != nil {
			return nil, err
		}
		var newCards []map[string]any
		for _, c := range recent {
			tp, _ := c["type"].(string)
			if tp != "convention" && tp != "decision" {
				continue
			}
			upd, _ := c["updated_at"].(string)
			if IsAfterTime(upd, since) {
				newCards = append(newCards, map[string]any{
					"id":    c["id"],
					"type":  tp,
					"title": c["title"],
				})
			}
		}
		stale := len(newCards) > 0
		result := map[string]any{
			"stale":         stale,
			"rules_age":     since,
			"checked_at":    time.Now().Format(time.RFC3339),
			"newer_cards":   newCards,
			"newer_count":   len(newCards),
		}
		if stale {
			result["message"] = "Rules are stale; call synthesize_rules to regenerate"
		} else {
			result["message"] = "Rules are up to date"
		}
		return result, nil
	}))
	// 11. context_for_files
	reg.Register("context_for_files", "获取与指定文件关联的活跃记忆。传入当前编辑的文件路径，系统自动返回相关 conventions/decisions/已知问题。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"paths": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "文件路径列表",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "返回结果数量上限",
			},
		},
		"required": []string{"paths"},
	}, wrap(func(params map[string]any) (any, error) {
		paths := toStringSlice(params["paths"])
		if len(paths) == 0 { return map[string]any{"results": []map[string]any{}, "matched_files": 0}, nil }
		limit := 20
		if v, ok := params["limit"].(float64); ok { limit = int(v) }
		results, err := svc.ContextForFiles(paths, limit)
		if err != nil { return nil, err }
		return map[string]any{"results": results, "matched_files": len(paths)}, nil
	}))
}
