package service

import (
	"fmt"
	"os"
	"time"

	"github.com/atop/project-memory-palace/internal/audit"
	"github.com/atop/project-memory-palace/internal/mcp"
	"github.com/atop/project-memory-palace/internal/memory"
	"github.com/atop/project-memory-palace/internal/store"
	"gopkg.in/yaml.v3"
)

// RegisterAllTools registers all MCP tools with the provided registry.
// Each handler is optionally wrapped with wrapHandler (tray layer uses this
// to inject mu.Lock()/defer mu.Unlock() around every handler call).
func RegisterAllTools(reg *mcp.ToolRegistry, ws *WorkspaceService, wrapHandler func(mcp.ToolHandler) mcp.ToolHandler) {
	wrap := func(h mcp.ToolHandler) mcp.ToolHandler {
		if wrapHandler != nil {
			return wrapHandler(h)
		}
		return h
	}

	typeEnum := memory.SortedKeys(memory.MemoryTypes)
	statusEnum := memory.SortedKeys(memory.MemoryStatuses)
	sourceKindEnum := memory.SortedKeys(memory.SourceKinds)
	projectProp := map[string]any{
		"type":        "string",
		"description": "目标工程名称。不填则使用默认工程（第一个发现的工程）。可用 list_projects 查看所有可用工程。",
	}

	// 0. init_workspace
	reg.Register("init_workspace", "初始化工作区。扫描所有可用工程，返回工程列表和简要状态。应在每个会话开始时首先调用。不创建任何文件，纯只读。", map[string]any{
		"type": "object", "properties": map[string]any{},
	}, wrap(func(params map[string]any) (any, error) {
		projects, err := ws.ListProjects()
		if err != nil { return nil, err }
		var digest []map[string]any
		for _, p := range projects {
			name, _ := p["name"].(string)
			if svcName, _, err := ws.resolve(name); err == nil {
				recent, _ := svcName.ListRecent(3, 0, nil)
				cardCount, _ := svcName.Count(nil)
				digest = append(digest, map[string]any{
					"name":       name,
					"card_count": cardCount,
					"recent":     recent,
					"is_default": p["is_default"],
				})
			}
		}
		return map[string]any{
			"workspace":  ws.workspaceDir,
			"projects":   projects,
			"total_projects": len(projects),
			"digest":     digest,
			"next": []string{
				"1. init_project project=<name> - 初始化并获取指定工程的 rules + recent",
				"2. context_for_files paths=[...] - 根据文件自动关联工程记忆",
				"3. recall_all query=<keyword> - 跨所有工程搜索",
			},
		}, nil
	}))

	// 1. init_project
	reg.Register("init_project", "初始化指定工程。返回该工程的 rules、recent activities 和 next-step guide。应先调用 init_workspace 了解可用工程，再用本工具初始化目标工程。", map[string]any{"type": "object", "properties": map[string]any{
		"project": projectProp,
		"since": map[string]any{
			"type":        "string",
			"description": "ISO timestamp — returns changes since this time (e.g. 2026-07-09T00:00:00Z)",
		},
	},
	}, wrap(func(params map[string]any) (any, error) {
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
		if err := svc.InitProject(); err != nil {
			return nil, err
		}
		result := map[string]any{"status": "initialized", "project_root": svc.ProjectRoot()}
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
		// If `since` is provided, return cards changed since that timestamp
		if since, ok := params["since"].(string); ok && since != "" {
			changedCards, _ := svc.ListChangesSince(since, 20)
			result["changes"] = changedCards
			result["changes_since"] = since
			result["change_count"] = len(changedCards)
		}
		result["next"] = []string{
			"1. context_for_files paths=[<current files>] - auto-discover memories linked to files you're working on (no keywords needed!)",
			"2. recall query=<keyword> - search project memory by topic or file path",
			"3. open_memory id=<id> - get full card details when summary is not enough",
			"4. remember memory={...} - persist new knowledge after completing work",
		}
		return result, nil
	}))

	// 2. remember
	reg.Register("remember", "Write one durable project memory card. Required: type, title, summary, content. Include source to achieve confidence > 0.5.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": projectProp,
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
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
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
			"project": projectProp,
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
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
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
			"project": projectProp,
			"id": map[string]any{"type": "string"},
		},
	}, wrap(func(params map[string]any) (any, error) {
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
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
			"project": projectProp,
			"id": map[string]any{"type": "string", "description": "Memory card ID (e.g. 'mem_20260612_001')."},
			"updates": map[string]any{
				"type":        "object",
				"description": "Fields to update: status, confidence (0.0-1.0), tags (string array), relations (object), expires_at (ISO timestamp).",
			},
		},
		"required": []string{"id", "updates"},
	}, wrap(func(params map[string]any) (any, error) {
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
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
			"project": projectProp,
			"id": map[string]any{"type": "string", "description": "Memory card ID to delete (e.g. 'mem_20260612_001')."},
		},
		"required": []string{"id"},
	}, wrap(func(params map[string]any) (any, error) {
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
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
			"project": projectProp,
			"limit": map[string]any{"type": "integer"},
		},
	}, wrap(func(params map[string]any) (any, error) {
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
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
		"type": "object", "properties": map[string]any{
			"project": projectProp,
		},
	}, wrap(func(params map[string]any) (any, error) {
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
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
			"project": projectProp,
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
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
	mode, _ := params["mode"].(string)
	since, _ := params["since"].(string)
	results, err := svc.Disclosure(mode, since)
	if err != nil { return nil, err }
	return map[string]any{"mode": mode, "results": results}, nil
	}))

	// 10. check_rules_freshness
	reg.Register("check_rules_freshness", "Check if the synthesized rules are stale (i.e., newer convention/decision cards exist that have not been reflected in agent-rules.yaml). Returns stale status and the count of newer cards.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": projectProp,
			"since": map[string]any{
				"type":        "string",
				"description": "Optional: ISO timestamp to check freshness against. If omitted, uses the rules file synthesized_at timestamp.",
			},
		},
	}, wrap(func(params map[string]any) (any, error) {
	svc, _, err := ws.resolve(extractProject(params))
	if err != nil { return nil, err }
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
			"project": projectProp,
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
	project := extractProject(params)
	autoDetected := false
	if project == "" {
	    rawPaths, ok := params["paths"].([]any)
	    if ok && len(rawPaths) > 0 {
	        paths := make([]string, len(rawPaths))
	        for ip, p := range rawPaths { paths[ip] = fmt.Sprint(p) }
        detected := ws.AutoDetect(paths)
        project = detected
        autoDetected = true
	    }
	}
	svc, projName, err := ws.resolve(project)
	if err != nil { return nil, err }
		paths := toStringSlice(params["paths"])
		if len(paths) == 0 { return map[string]any{"results": []map[string]any{}, "matched_files": 0}, nil }
		limit := 20
		if v, ok := params["limit"].(float64); ok { limit = int(v) }
		results, err := svc.ContextForFiles(paths, limit)
		if err != nil { return nil, err }
		result := map[string]any{"results": results, "matched_files": len(paths)}
		if autoDetected {
			result["project"] = projName
			result["auto_detected"] = true
		}
		return result, nil
	}))
	// 12. list_projects
	reg.Register("list_projects", "列出当前工作区所有可用工程及其卡片数量。Agent 应在会话开始时调用此工具了解可用工程，然后在其他工具中使用 project 参数指定目标工程。", map[string]any{
		"type": "object", "properties": map[string]any{},
	}, wrap(func(params map[string]any) (any, error) {
		projects, err := ws.ListProjects()
		if err != nil { return nil, err }
		return map[string]any{
			"workspace": ws.workspaceDir,
			"projects":  projects,
			"count":     len(projects),
		}, nil
	}))

	// 13. recall_all
	reg.Register("recall_all", "跨所有工程搜索记忆。当你不知道某个知识点在哪个工程中时使用。结果中每条卡片带 project 字段标明来源工程。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "搜索关键词"},
			"filters": map[string]any{
				"type": "object",
				"description": "可选过滤: status (string), paths (string array)",
				"properties": map[string]any{
					"status": map[string]any{"type": "string"},
					"paths":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			"limit": map[string]any{"type": "integer", "description": "最大返回数量 (默认 10)"},
		},
		"required": []string{"query"},
	}, wrap(func(params map[string]any) (any, error) {
		query, _ := params["query"].(string)
		filters, _ := params["filters"].(map[string]any)
		limit := 10
		if v, ok := params["limit"].(float64); ok { limit = int(v) }
		results, err := ws.RecallAll(query, filters, limit)
		if err != nil { return nil, err }
	return map[string]any{"results": results, "searched_projects": len(ws.projects)}, nil
	}))

	// 14. audit_project
	reg.Register("audit_project", "审计指定工程的所有记忆卡片，检测低置信度、缺失标签/范围、高置信度推理、疑似重复、过期卡片、多Agent冲突等问题。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": projectProp,
		},
	}, wrap(func(params map[string]any) (any, error) {
		svc, _, err := ws.resolve(extractProject(params))
		if err != nil {
			return nil, err
		}
		report, err := audit.AuditProject(svc.ProjectRoot())
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"issues":      report,
			"issue_count": len(report),
			"project":     svc.ProjectRoot(),
		}, nil
	}))

	// 15. vacuum
	reg.Register("vacuum", "压缩数据库，回收磁盘空间。建议定期调用以保持数据库性能。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": projectProp,
		},
	}, wrap(func(params map[string]any) (any, error) {
		svc, _, err := ws.resolve(extractProject(params))
		if err != nil {
			return nil, err
		}
		if err := svc.Vacuum(); err != nil {
			return nil, err
		}
		return map[string]any{
			"status":  "ok",
			"message": "database vacuumed successfully",
		}, nil
	}))

	// 16. refresh_workspace
	// 16. refresh_workspace
	reg.Register("refresh_workspace", "刷新工作区，扫描新添加的工程。添加新工程后无需重启服务。", map[string]any{"type": "object", "properties": map[string]any{},
	}, wrap(func(params map[string]any) (any, error) {
		added := ws.RefreshWorkspace()
		return map[string]any{
			"added":          added,
			"total_projects": len(ws.ProjectNames()),
		}, nil
	}))

	// 17. get_relations
	reg.Register("get_relations", "获取卡片的关系图。返回出向关系、入向引用、卡片标题，支持多层级图遍历（最多3层）。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "卡片 ID（如 'mem_20260101_001'）",
			},
			"direction": map[string]any{
				"type":        "string",
				"description": "关系方向: 'outgoing'（出向，默认）, 'incoming'（入向）, 或 'both'（双向）",
				"enum":        []string{"outgoing", "incoming", "both"},
			},
			"depth": map[string]any{
				"type":        "integer",
				"description": "图遍历深度（默认 1，最大 3）",
				"minimum":     float64(1),
				"maximum":     float64(3),
			},
			"project": projectProp,
		},
		"required": []string{"id"},
	}, wrap(func(params map[string]any) (any, error) {
		svc, _, err := ws.resolve(extractProject(params))
		if err != nil {
			return nil, err
		}
		id, _ := params["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("id parameter required")
		}
		direction := "outgoing"
		if v, ok := params["direction"].(string); ok && v != "" {
			direction = v
		}
		depth := 1
		if v, ok := params["depth"].(float64); ok {
			depth = int(v)
		}
		return svc.GetRelations(id, direction, depth)
	}))
}
