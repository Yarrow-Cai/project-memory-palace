package service

import (
	"fmt"
	"os"
	"time"

	"github.com/atop/project-memory-palace/internal/audit"
	"github.com/atop/project-memory-palace/internal/mcp"
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

				projectProp := map[string]any{
		"type":        "string",
		"description": "目标工程名称。留空则使用默认工程。",
	}

	// 0. init_workspace
	reg.Register("init_workspace", "初始化工作区。扫描所有可用工程，返回工程列表和简要状态。应在每个会话开始时首先调用。可选 workspace 参数可切换到新的工作区目录（持久化到配置文件）。", map[string]any{
		"type": "object", "properties": map[string]any{
			"workspace": map[string]any{
				"type":        "string",
				"description": "可选: 切换到此工作区目录路径。留空则使用当前工作区。",
			},
		},
	}, wrap(func(params map[string]any) (any, error) {
		// Optional workspace switch
		if path, ok := params["workspace"].(string); ok && path != "" {
			if err := ws.SetWorkspace(path, ""); err != nil { return nil, err }
		}
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
				"3. recall query=<keyword> - 跨所有工程搜索（不传 project 参数）",
			},
		}, nil
	}))

	// 1. init_project
	reg.Register("init_project", "初始化指定工程。返回该工程的 rules、recent activities、可选 disclosure 和 next-step guide。应先调用 init_workspace 了解可用工程，再用本工具初始化目标工程。", map[string]any{"type": "object", "properties": map[string]any{
		"project": projectProp,
		"since": map[string]any{
			"type":        "string",
			"description": "ISO timestamp — returns changes since this time (e.g. 2026-07-09T00:00:00Z)",
		},
		"disclosure_mode": map[string]any{
			"type":        "string",
			"description": "可选: 渐进披露模式 — 'first'（高优先级全貌）或 'subsequent'（核心+近期变更）",
			"enum":        []string{"first", "subsequent"},
		},
		"disclosure_since": map[string]any{
			"type":        "string",
			"description": "ISO timestamp — disclosure 变更时间起点（subsequent 模式用）",
		},
	},
	}, wrap(func(params map[string]any) (any, error) {
		svc, projName, err := ws.resolve(extractProject(params))
		if err != nil { return nil, err }
		// Auto-set as default project + persist
		store.SetDefaultProject(ws.workspaceDir, projName)
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
				if syncedAt, ok := doc["synthesized_at"].(string); ok {
					result["rules_fresh"] = true
					result["rules_synthesized_at"] = syncedAt
				}
			}
		}
		if _, ok := result["rules_fresh"]; !ok {
			result["rules_fresh"] = false
		}
	recent, _ := svc.ListRecent(5, 0, nil)
		result["recent"] = recent
		// Multi-agent change awareness: summarize recent agent activity
		if allRecent, err := svc.ListRecent(20, 0, nil); err == nil && len(allRecent) > 0 {
			oneHourAgo := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
			var newCards, updatedCards int
			agents := make(map[string]bool)
			var highlights []map[string]any
			for _, c := range allRecent {
				created, _ := c["created_at"].(string)
				updated, _ := c["updated_at"].(string)
				agent, _ := c["source_agent"].(string)
				if created >= oneHourAgo {
					newCards++
				}
				if updated >= oneHourAgo && updated != created {
					updatedCards++
				}
				if agent != "" {
					agents[agent] = true
				}
				if len(highlights) < 5 {
					highlights = append(highlights, map[string]any{
						"id": c["id"], "title": c["title"],
						"by": agent, "when": updated,
					})
				}
			}
			agentList := make([]string, 0, len(agents))
			for a := range agents { agentList = append(agentList, a) }
			result["recent_changes"] = map[string]any{
				"since": oneHourAgo,
				"new_cards": newCards, "updated_cards": updatedCards,
				"agents_active": agentList, "highlights": highlights,
			}
		}
		// If `since` is provided, return cards changed since that timestamp
		if since, ok := params["since"].(string); ok && since != "" {
			changedCards, _ := svc.ListChangesSince(since, 20)
			result["changes"] = changedCards
			result["changes_since"] = since
			result["change_count"] = len(changedCards)
		}
		// Disclosure merge: if disclosure_mode is set, call Disclosure() and include in response
		if mode, ok := params["disclosure_mode"].(string); ok && mode != "" {
			since, _ := params["disclosure_since"].(string)
			cards, err := svc.Disclosure(mode, since)
			if err == nil {
				result["disclosure"] = map[string]any{
					"cards": cards,
					"mode":  mode,
				}
			}
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
	reg.Register("remember", "写入一张或多张持久化记忆卡片。必填: type, title, summary, content。传入单个 object 创建一张，或传入 array 批量创建。提供 source 可获得 >0.5 置信度。写入后自动检测重复并返回 warnings。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": projectProp,
			"memory": map[string]any{
				"type":        "object",
				"description": "单张记忆卡片（object）或批量数组（array of objects）。数组模式返回 results 列表。",
			},
		},
		"required": []string{"memory"},
	}, wrap(func(params map[string]any) (any, error) {
		raw, ok := params["memory"]
		if !ok {
			return nil, fmt.Errorf("memory parameter required")
		}
		switch mem := raw.(type) {
		case map[string]any:
			// Single card
			svc, _, err := ws.resolve(extractProject(params))
			if err != nil { return nil, err }
			result, err := svc.Remember(mem)
			if err != nil { return nil, err }
			// Built-in duplicate check
			if title, _ := mem["title"].(string); title != "" {
				cardID, _ := result["id"].(string)
				warnings := checkDuplicates(svc, title, 0.3, cardID)
				if len(warnings) > 0 {
					result["warnings"] = warnings
				}
			}
			return result, nil
		case []any:
			// Batch
			if len(mem) == 0 {
				return nil, fmt.Errorf("memory array must not be empty")
			}
			svc, _, err := ws.resolve(extractProject(params))
			if err != nil { return nil, err }
			var results []map[string]any
			for i, m := range mem {
				payload, ok := m.(map[string]any)
				if !ok {
					results = append(results, map[string]any{"index": i, "error": "invalid memory object, expected object"})
					continue
				}
				result, err := svc.Remember(payload)
				if err != nil {
					results = append(results, map[string]any{"index": i, "error": err.Error()})
					continue
				}
				// Built-in duplicate check
				if title, _ := payload["title"].(string); title != "" {
					cardID, _ := result["id"].(string)
					warnings := checkDuplicates(svc, title, 0.3, cardID)
					if len(warnings) > 0 {
						result["warnings"] = warnings
					}
				}
				results = append(results, result)
			}
			return map[string]any{"results": results, "total": len(mem), "success": len(results)}, nil
		default:
			return nil, fmt.Errorf("memory must be an object or array of objects")
		}
	}))

	// 3. recall
	reg.Register("recall", "搜索记忆（Level 2 披露）。返回简短摘要。不传 project 则跨所有工程搜索。传入 paths 过滤可获取文件特定上下文。需要详情？用 open_memory。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": projectProp,
			"query": map[string]any{
				"type":        "string",
				"description": "搜索关键词或短语。支持中英文。用简洁术语获得最佳结果。",
			},
			"filters": map[string]any{
				"type":        "object",
				"description": "可选过滤: status (string), paths (string 或 string array, 按文件路径过滤)。",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "description": "Filter by memory status."},
					"paths":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Filter by file paths associated with the memory."},
				},
			},
			"limit": map[string]any{"type": "integer"},
		},
		"required": []string{"query"},
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

		project := extractProject(params)
		var results []map[string]any
		if project == "" {
			// Cross-project search
			r, err := ws.RecallAll(query, filters, limit)
			if err != nil { return nil, err }
			results = r
		} else {
			svc, _, err := ws.resolve(project)
			if err != nil { return nil, err }
			r, err := svc.Recall(query, filters, limit)
			if err != nil { return nil, err }
			results = r
		}
		return map[string]any{"results": results}, nil
	}))

	// 4. open_memory
	reg.Register("open_memory", "Level 3 披露: 按 ID 获取完整卡片内容。传入单条 ID 字符串或 ID 数组批量获取。仅在 recall 返回摘要后需要详情时调用。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": projectProp,
			"id": map[string]any{
				"type":        "string",
				"description": "卡片 ID（如 'mem_20260101_001'）或 ID 数组",
			},
		},
	}, wrap(func(params map[string]any) (any, error) {
		svc, _, err := ws.resolve(extractProject(params))
		if err != nil { return nil, err }

		switch id := params["id"].(type) {
		case string:
			// Single card
			if id == "" {
				return nil, fmt.Errorf("id parameter required")
			}
			return svc.OpenMemory(id)
		case []any:
			// Batch
			ids := toStringSlice(params["id"])
			if len(ids) == 0 {
				return nil, fmt.Errorf("id array must not be empty")
			}
			var results []map[string]any
			for _, memID := range ids {
				card, err := svc.OpenMemory(memID)
				if err != nil {
					results = append(results, map[string]any{"id": memID, "error": err.Error()})
				} else {
					results = append(results, card)
				}
			}
			// Record access for batch
			var accessedIDs []string
			for _, r := range results {
				if rid, ok := r["id"].(string); ok && rid != "" {
					accessedIDs = append(accessedIDs, rid)
				}
			}
			if len(accessedIDs) > 0 {
				_ = svc.idx.RecordAccess(accessedIDs)
			}
			return map[string]any{"results": results}, nil
		default:
			return nil, fmt.Errorf("id must be a string or array of strings")
		}
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
		if v, ok := params["id"].(string); ok { id = v }
		updates, ok := params["updates"].(map[string]any)
		if !ok { return nil, fmt.Errorf("updates parameter required") }
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
		if v, ok := params["id"].(string); ok { id = v }
		if id == "" { return nil, fmt.Errorf("id parameter required") }
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
		if v, ok := params["limit"].(float64); ok { limit = int(v) }
		results, err := svc.ListRecent(limit, 0, nil)
		if err != nil { return nil, err }
		return map[string]any{"results": results}, nil
	}))

	// 8. context_for_files
	reg.Register("context_for_files", "获取与指定文件关联的活跃记忆。传入当前编辑的文件路径，系统自动返回相关 conventions/decisions/已知问题。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": projectProp,
			"paths": map[string]any{
				"type": "array", "items": map[string]any{"type": "string"},
				"description": "文件路径列表",
			},
			"limit": map[string]any{"type": "integer", "description": "返回结果数量上限"},
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
		// Cross-agent awareness: show recent activity from other agents
		if ch, err := svc.CrossAgentHint(paths); err == nil && ch != "" {
			result["cross_agent_hint"] = ch
		}
		return result, nil
	}))

	// 11. audit_project
	reg.Register("audit_project", "审计指定工程的所有记忆卡片，检测低置信度、缺失标签/范围、高置信度推理、疑似重复、过期卡片、多Agent冲突等问题。", map[string]any{
		"type": "object",
		"properties": map[string]any{"project": projectProp},
	}, wrap(func(params map[string]any) (any, error) {
		svc, _, err := ws.resolve(extractProject(params))
		if err != nil { return nil, err }
		report, err := audit.AuditProject(svc.ProjectRoot())
		if err != nil { return nil, err }
		return map[string]any{
			"issues": report, "issue_count": len(report), "project": svc.ProjectRoot(),
		}, nil
	}))

	// 12. vacuum
	reg.Register("vacuum", "压缩数据库，回收磁盘空间。建议定期调用以保持数据库性能。", map[string]any{
		"type": "object",
		"properties": map[string]any{"project": projectProp},
	}, wrap(func(params map[string]any) (any, error) {
		svc, _, err := ws.resolve(extractProject(params))
		if err != nil { return nil, err }
		if err := svc.Vacuum(); err != nil { return nil, err }
		return map[string]any{"status": "ok", "message": "database vacuumed successfully"}, nil
	}))

	// 13. get_relations
	reg.Register("get_relations", "获取卡片的关系图。返回出向关系、入向引用、卡片标题，支持多层级图遍历（最多3层）。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "卡片 ID（如 'mem_20260101_001'）"},
			"direction": map[string]any{
				"type": "string", "description": "关系方向: 'outgoing'（出向，默认）, 'incoming'（入向）, 或 'both'（双向）",
				"enum": []string{"outgoing", "incoming", "both"},
			},
			"depth": map[string]any{
				"type": "integer", "description": "图遍历深度（默认 1，最大 3）",
				"minimum": float64(1), "maximum": float64(3),
			},
			"project": projectProp,
		},
		"required": []string{"id"},
	}, wrap(func(params map[string]any) (any, error) {
		svc, _, err := ws.resolve(extractProject(params))
		if err != nil { return nil, err }
		id, _ := params["id"].(string)
		if id == "" { return nil, fmt.Errorf("id parameter required") }
		direction := "outgoing"
		if v, ok := params["direction"].(string); ok && v != "" { direction = v }
		depth := 1
		if v, ok := params["depth"].(float64); ok { depth = int(v) }
		return svc.GetRelations(id, direction, depth)
	}))

	// 14. list_templates
	reg.Register("list_templates", "列出当前工程可用的卡片模板（.project-memory/templates/*.yaml）。", map[string]any{
		"type": "object",
		"properties": map[string]any{"project": projectProp},
	}, wrap(func(params map[string]any) (any, error) {
		svc, _, err := ws.resolve(extractProject(params))
		if err != nil { return nil, err }
		names, err := svc.ListTemplates()
		if err != nil { return nil, err }
		return map[string]any{"templates": names}, nil
	}))

	// 15. extract_patterns
	reg.Register("extract_patterns", "跨工程提取可复用的知识模式。扫描所有工程中 type=pattern 的卡片，按标签相似度聚合，返回在多个工程中出现的可迁移模式。", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"min_projects": map[string]any{
				"type": "integer", "description": "模式至少需要出现在几个工程中（默认 2）",
			},
		},
	}, wrap(func(params map[string]any) (any, error) {
		minProjects := 2
		if v, ok := params["min_projects"].(float64); ok { minProjects = int(v) }
		return ws.ExtractPatterns(minProjects)
	}))

}
