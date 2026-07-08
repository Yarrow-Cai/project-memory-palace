# pmem 第一性原理缺陷修复计划

> **For Hermes:** Use codex exec to implement, then manual verification.
>
> **Goal:** 修复 pmem 的 8 个第一性原理缺陷：P0 文件自动关联、使用反馈、Agent 冲突检测；P1 遗留 bug；P2 类型区分、优先级衰减。
>
> **Architecture:** 渐进式增量修改——每阶段仅改最小必要代码，保持 YAML/SQLite 双存储架构不变。
>
> **Tech Stack:** Go 1.21+, SQLite (modernc), YAML v3, FTS5
>
> **Git repo:** ~/Desktop/pmem/project-memory-palace/project-memory-palace/
>
> **pmem binary (build target):** `bin/pmem.exe` in repo root
>
> **Test command:** `go test ./internal/... -v`

---

## Phase 1: 修复 P1 遗留缺陷 (5 min)

### Task 1.1: 移除 Remember() 中的自动 SynthesizeRules()

**What:** `service.go:53` 的 `_, _ = s.SynthesizeRules()` 造成每次写入做全扫描。删除此行。

**File:** `internal/service/service.go`
**Change:** 删除第 53 行
```go
// DELETE this line:
_, _ = s.SynthesizeRules()
```

### Task 1.2: 修复 check_rules_freshness 中的字符串时间比较

**What:** `mcp_tools.go:372` 使用 `upd > since` 做字符串比较，应改用 `IsAfterTime()`。

**File:** `internal/service/mcp_tools.go`
**Change:** 第 371-372 行
```go
// OLD:
upd, _ := c["updated_at"].(string)
if upd > since {

// NEW:
upd, _ := c["updated_at"].(string)
if IsAfterTime(upd, since) {
```

---

## Phase 2: 文件-记忆自动关联 (P0, ~10 min)

### 核心设计

新增 `context_for_files` MCP 工具：agent 传入当前编辑的文件路径列表，系统返回所有关联的 memory cards（按 `scope.paths` 匹配）。

**API 签名:**
```
context_for_files(paths: string[], limit?: int) → { results: MemoryCardSummary[] }
```

**匹配逻辑:** 任一 `scope.paths` 元素是传入 `paths` 中任一元素的后缀匹配（`strings.HasSuffix`）或前缀匹配。

### Task 2.1: 在 index 层添加按路径查询

**File:** `internal/index/index.go`

添加方法 `SearchByPaths`:
```go
// SearchByPaths returns active memory cards whose scope.paths intersect
// with the given file paths. Uses suffix matching for path containment.
func (idx *MemoryIndex) SearchByPaths(paths []string, limit int) ([]map[string]any, error) {
    if len(paths) == 0 { return nil, nil }
    if limit <= 0 { limit = 20 }
    if err := idx.Initialize(); err != nil { return nil, err }
    db, err := idx.connect()
    if err != nil { return nil, err }

    // Build OR clause: path LIKE '%suffix1' OR path LIKE '%suffix2' ...
    var clauses []string
    var args []any
    for _, p := range paths {
        clauses = append(clauses, "mp.path LIKE ?")
        args = append(args, "%"+p)
        // Also match prefix: the stored path contains the given path as prefix
        clauses = append(clauses, "? LIKE '%' || mp.path")
        args = append(args, p)
    }
    args = append(args, limit)

    q := fmt.Sprintf(
        `SELECT DISTINCT m.id,m.type,m.status,m.priority,m.title,m.summary,m.source_kind,m.confidence,m.updated_at
         FROM memories m
         JOIN memory_paths mp ON mp.memory_id = m.id
         WHERE m.status = 'active'
           AND (m.expires_at = '' OR m.expires_at > datetime('now'))
           AND (%s)
         ORDER BY m.priority DESC, m.updated_at DESC
         LIMIT ?`, strings.Join(clauses, " OR "))

    rows, err := db.Query(q, args...)
    if err != nil { return nil, fmt.Errorf("search by paths: %w", err) }
    defer rows.Close()
    var results []map[string]any
    for rows.Next() {
        var id, tp, st, title, summary, sk, upd string
        var priority int
        var conf float64
        rows.Scan(&id, &tp, &st, &priority, &title, &summary, &sk, &conf, &upd)
        results = append(results, map[string]any{
            "id": id, "type": tp, "status": st, "priority": priority,
            "title": title, "summary": summary,
            "confidence": conf, "source_hint": sk,
            "matched_by": []string{"file_context"},
            "updated_at": upd,
        })
    }
    return results, rows.Err()
}
```

### Task 2.2: 在 service 层添加 ContextForFiles

**File:** `internal/service/service.go`

```go
func (s *MemoryService) ContextForFiles(paths []string, limit int) ([]map[string]any, error) {
    if err := s.InitProject(); err != nil { return nil, err }
    return s.idx.SearchByPaths(paths, limit)
}
```

### Task 2.3: 注册 MCP 工具 context_for_files

**File:** `internal/service/mcp_tools.go`

在 `RegisterAllTools` 末尾（`}` 之前）添加第 11 个工具:

```go
// 11. context_for_files
reg.Register("context_for_files", "获取与指定文件关联的活跃记忆。当你要编辑/阅读某些文件时调用此工具，系统自动返回相关的 conventions、decisions、已知问题。不需要事先知道关键词。", map[string]any{
    "type": "object",
    "properties": map[string]any{
        "paths": map[string]any{
            "type":        "array",
            "items":       map[string]any{"type": "string"},
            "description": "当前正在编辑/阅读的文件路径列表（绝对或相对）",
        },
        "limit": map[string]any{"type": "integer", "description": "最大返回数量，默认 20"},
    },
    "required": []string{"paths"},
}, wrap(func(params map[string]any) (any, error) {
    rawPaths, ok := params["paths"].([]any)
    if !ok { return nil, fmt.Errorf("paths parameter required") }
    paths := make([]string, len(rawPaths))
    for i, p := range rawPaths { paths[i] = fmt.Sprint(p) }
    limit := 20
    if v, ok := params["limit"].(float64); ok { limit = int(v) }
    results, err := svc.ContextForFiles(paths, limit)
    if err != nil { return nil, err }
    return map[string]any{"results": results, "matched_files": len(paths)}, nil
}))
```

### Task 2.4: 添加 Web API 端点

**File:** `cmd/pmem/main.go`

在 `cmdServeWeb` 中添加路由（放在 `/api/search` 之后）:

```go
http.HandleFunc("/api/context", func(w http.ResponseWriter, r *http.Request) {
    pathStr := r.URL.Query().Get("paths")
    if pathStr == "" { writeWebJSONList(w, nil, nil); return }
    paths := strings.Split(pathStr, ",")
    limit := service.ParseIntParam(r.URL.Query().Get("limit"), 20)
    results, err := svc.ContextForFiles(paths, limit)
    writeWebJSONList(w, results, err)
})
```

需要 `import "strings"` 如果还没导入。

---

## Phase 3: 记忆使用追踪 (P0, ~10 min)

### Task 3.1: 扩展 SQLite schema 添加 access_count 和 last_accessed_at

**File:** `internal/index/index.go`

在 `schemaDDL` 后添加 migration:

```go
// Migration: add access tracking columns (added in schema v4)
db.Exec("ALTER TABLE memories ADD COLUMN access_count INTEGER NOT NULL DEFAULT 0")
db.Exec("ALTER TABLE memories ADD COLUMN last_accessed_at TEXT NOT NULL DEFAULT ''")
```

放在 `Initialize()` 方法中，紧跟 expires_at migration 之后。

### Task 3.2: 添加 RecordAccess 方法

**File:** `internal/index/index.go`

```go
// RecordAccess increments the access count and updates last_accessed_at
// for the given memory IDs. Called when memories appear in recall/disclosure results.
func (idx *MemoryIndex) RecordAccess(ids []string) error {
    if len(ids) == 0 { return nil }
    db, err := idx.connect()
    if err != nil { return err }
    now := time.Now().Format(time.RFC3339)
    q := "UPDATE memories SET access_count = access_count + 1, last_accessed_at = ? WHERE id IN (?" + strings.Repeat(",?", len(ids)-1) + ")"
    args := make([]any, 0, len(ids)+1)
    args = append(args, now)
    for _, id := range ids {
        args = append(args, id)
    }
    _, err = db.Exec(q, args...)
    return err
}
```

需要 `import "time"` 如果还没导入。

### Task 3.3: 在 recall/disclosure/open_memory 时记录访问

**File:** `internal/service/service.go`

在 `Recall()` 返回前:
```go
func (s *MemoryService) Recall(query string, filters map[string]any, limit int) ([]map[string]any, error) {
    if err := s.InitProject(); err != nil { return nil, err }
    results, err := s.idx.Search(query, filters, limit)
    if err != nil { return nil, err }
    // Record access
    if len(results) > 0 {
        ids := make([]string, len(results))
        for i, r := range results { ids[i], _ = r["id"].(string) }
        _ = s.idx.RecordAccess(ids)
    }
    return results, nil
}
```

在 `OpenMemory()` 返回前:
```go
func (s *MemoryService) OpenMemory(memoryID string) (map[string]any, error) {
    // ... existing code ...
    // Record access
    _ = s.idx.RecordAccess([]string{card.ID})  // after card read succeeds
    return cardToMap(cardObj), nil
}
```

在 `ListRecent()` 中（供 disclosure 使用）:
```go
func (s *MemoryService) ListRecent(limit, offset int, filters map[string]any) ([]map[string]any, error) {
    if err := s.InitProject(); err != nil { return nil, err }
    results, err := s.idx.Recent(limit, offset, filters)
    if err != nil { return nil, err }
    if len(results) > 0 {
        ids := make([]string, len(results))
        for i, r := range results { ids[i], _ = r["id"].(string) }
        _ = s.idx.RecordAccess(ids)
    }
    return results, nil
}
```

### Task 3.4: 添加热门记忆查询方法

**File:** `internal/index/index.go`

```go
// HotMemories returns the most frequently accessed active memories.
func (idx *MemoryIndex) HotMemories(limit int) ([]map[string]any, error) {
    if limit <= 0 { limit = 10 }
    if err := idx.Initialize(); err != nil { return nil, err }
    db, err := idx.connect()
    if err != nil { return nil, err }
    rows, err := db.Query(
        `SELECT id,type,status,priority,title,summary,source_kind,confidence,access_count,last_accessed_at,updated_at
         FROM memories WHERE status='active' AND access_count > 0 AND (expires_at='' OR expires_at>datetime('now'))
         ORDER BY access_count DESC LIMIT ?`, limit)
    if err != nil { return nil, err }
    defer rows.Close()
    var results []map[string]any
    for rows.Next() {
        var id, tp, st, title, summary, sk, accessed, upd string
        var priority, count int
        var conf float64
        rows.Scan(&id, &tp, &st, &priority, &title, &summary, &sk, &conf, &count, &accessed, &upd)
        results = append(results, map[string]any{
            "id": id, "type": tp, "status": st, "priority": priority,
            "title": title, "summary": summary,
            "confidence": conf, "source_hint": sk,
            "access_count": count, "last_accessed_at": accessed,
            "matched_by": []string{"hot"}, "updated_at": upd,
        })
    }
    return results, rows.Err()
}
```

### Task 3.5: Web API 端点 + 在 recents 中返回 access_count

**File:** `internal/index/index.go` — 修改 `Recent()` 的 SELECT 和 Scan，加入 `access_count, last_accessed_at`

**File:** `cmd/pmem/main.go` — 添加 `/api/hot` 端点

---

## Phase 4: Agent 来源标记 + 冲突检测 (P0, ~10 min)

### Task 4.1: MemoryCard 添加 source_agent 字段

**File:** `internal/memory/card.go`

```go
type MemoryCard struct {
    // ... existing fields ...
    SourceAgent  string  `yaml:"source_agent" json:"source_agent"` // 创建此记忆的 AI agent 标识
}
```

默认值在 `NewCard()` 中设为空字符串。

### Task 4.2: SQLite schema 添加 source_agent 列

**File:** `internal/index/index.go`

Migration:
```go
db.Exec("ALTER TABLE memories ADD COLUMN source_agent TEXT NOT NULL DEFAULT ''")
```

更新 `doUpsert` 中的 INSERT 和 SELECT 语句。

### Task 4.3: MCP tool remember 接受 source_agent

**File:** `internal/service/mcp_tools.go`

在 `remember` tool 的 `memory` properties 中添加:
```go
"source_agent": map[string]any{
    "type":        "string",
    "description": "创建此记忆的 AI agent 标识 (如 claude-code, codex-cli, hermes)",
},
```

### Task 4.4: 增强 audit 发现潜在矛盾

**File:** `internal/audit/audit.go`

在现有检查后添加 `near_duplicate` 检测:

```go
// near_duplicate: same type + overlapping paths/tags + different agent
```

以及检测同一个 `scope.paths` 下有多个同类型但不同 source_agent 的记忆（潜在矛盾信号）。

---

## Phase 5: 优先级自动衰减 (P2, ~8 min)

### Task 5.1: 添加衰减计算方法

**新文件:** `internal/index/decay.go`

```go
package index

import "time"

// DecayFactor returns a multiplier (0.0-1.0) based on how long since
// the memory was last accessed. Fresh memories = 1.0, old = 0.3.
func DecayFactor(lastAccessedAt string) float64 {
    if lastAccessedAt == "" { return 1.0 } // never accessed yet
    t, err := time.Parse(time.RFC3339, lastAccessedAt)
    if err != nil { return 1.0 }
    days := time.Since(t).Hours() / 24
    switch {
    case days < 7:   return 1.0
    case days < 30:  return 0.85
    case days < 90:  return 0.6
    case days < 180: return 0.4
    default:         return 0.25
    }
}

// EffectivePriority computes effective priority = manual_priority * decay_factor
func EffectivePriority(manualPriority int, lastAccessedAt string) float64 {
    return float64(manualPriority) * DecayFactor(lastAccessedAt)
}
```

### Task 5.2: 在 disclosure/recall 中使用有效优先级

修改 `Recent()` 和 `Search()` 的 ORDER BY，当存在 `access_count` 和 `last_accessed_at` 时，按有效优先级排序。

---

## Phase 6: 类型区分 — fact vs interpretation (P2, ~8 min)

### Task 6.1: MemoryCard 添加 knowledge_kind 字段

**File:** `internal/memory/card.go`

```go
type MemoryCard struct {
    // ... existing fields ...
    KnowledgeKind string `yaml:"knowledge_kind" json:"knowledge_kind"` // "fact" | "interpretation" | "rule" | ""
}
```

空字符串 = 旧 schema 兼容，视为 "unknown"。

### Task 6.2: constants.go 添加合法值

**File:** `internal/memory/constants.go`

```go
var KnowledgeKinds = map[string]bool{
    "fact": true, "interpretation": true, "rule": true,
}
```

### Task 6.3: SQLite migration + MCP tool 更新

Schema migration、upsert 更新、remember tool 的 properties 添加 `knowledge_kind` 字段。

---

## Phase 7: 构建 + 全量验证

```bash
cd ~/Desktop/pmem/project-memory-palace/project-memory-palace
go build -o bin/pmem.exe ./cmd/pmem
go test ./internal/... -v
```

验证清单:
- [ ] `pmem.exe serve-mcp .` 启动后 MCP 工具列表包含 `context_for_files`
- [ ] `curl http://127.0.0.1:8147/api/context?paths=main.go` 返回文件关联记忆
- [ ] `curl http://127.0.0.1:8147/api/hot` 返回热门记忆
- [ ] 多次调用 `recall` 后 `access_count` 增长
- [ ] 旧 YAML 文件（无 source_agent/knowledge_kind）仍可正常读取（向后兼容）
- [ ] WebUI 正常加载
