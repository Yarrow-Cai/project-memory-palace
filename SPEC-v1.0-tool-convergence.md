# SPEC: MCP Tool Convergence — 23 → 16

> **Principle**: 6 个真实需求，不需要 23 个工具。合并语义重叠、移除自动冗余、内嵌低频检测。

---

## 目标

| 指标 | 现状 | 目标 |
|------|------|------|
| MCP 工具数 | 23 | **16** |
| 功能覆盖 | 100% | 100%（零损失） |
| 工具定义 token 开销 | ~4K/turn | ~2.5K/turn |
| 语义重叠 | 4 组 | 0 组 |

---

## 合并清单（7 个操作）

### M1: `list_projects` → 移除
**理由**: `init_workspace` 已返回完整工程列表 + 卡片数 + 最近活动摘要。

**操作**:
- 从 `mcp_tools.go` 删除 `list_projects` 注册
- `/api/projects` Web API **保留**（WebUI 侧边栏需要）
- `init_workspace` 返回的 `next` hints 不再提 `list_projects`

---

### M2: `recall_all` → 合并入 `recall`
**理由**: `recall` 不传 `project` 就是跨工程，传了就限定工程。两个工具的搜索逻辑完全一致，只是路由不同。

**操作**:
```diff
- recall          query + project → 单工程搜索
- recall_all      query + filters → 跨工程搜索
+ recall          query + project? + filters → project 为空则跨工程
```

**实现**:
- `recall` handler 中：`project` 参数为空 → 调 `ws.RecallAll(query, filters, limit)`
- `project` 参数非空 → 现有单工程逻辑
- description 更新："指定 project 则限定工程搜索，不传则跨所有工程搜索"
- 删除 `recall_all` 注册

---

### M3: `remember_batch` → 合并入 `remember`
**理由**: 唯一的区别是 `memory` 参数是 object 还是 array。

**操作**:
```diff
- remember       memory: object
- remember_batch memories: array
+ remember       memory: object | array  (自动检测)
```

**实现**:
- handler 中检测 `params["memory"]` 类型：
  - `map[string]any` → 单张创建（现有逻辑）
  - `[]any` → 遍历数组，逐张创建，返回 `{results: [{id, error?}, ...]}`
- schema 更新：`memory` param 改为 `oneOf: [{type: object}, {type: array}]`
- 删除 `remember_batch` 注册
- **保留** service 层的 `RememberBatch()` 方法（批量创建），只是入口统一

---

### M4: `recall_batch` → 合并入 `open_memory`
**理由**: `open_memory` 取单张详情，`recall_batch` 批量取详情。只是 ID 数量不同。

**操作**:
```diff
- open_memory     id: string
- recall_batch    ids: string[]
+ open_memory     id: string | string[]  (自动检测)
```

**实现**:
- handler 中检测参数类型：
  - `string` → 单张详情（现有逻辑）
  - `[]any` → 遍历 ID 列表，批量返回 `{results: [{id, card? error?}, ...]}`
- 删除 `recall_batch` 注册

---

### M5: `check_duplicates` → 内嵌入 `remember`
**理由**: 去重是写入流程的一部分，不应该要求 agent 先调一个工具再调另一个。

**操作**:
- `remember` 的返回结果中新增 `warnings` 数组
- 写入完成后自动运行 `check_duplicates`，若有相似卡片：
  ```json
  {
    "id": "mem_...",
    "warnings": [
      {
        "type": "possible_duplicate",
        "similar_id": "mem_...",
        "similar_title": "...",
        "score": 0.85
      }
    ]
  }
  ```
- `check_duplicates` MCP 工具 **保留**（agent 可能需要在写入前检查）
- 但 `remember` 内置检查确保**不调用也会被警告**

---

### M6: `refresh_workspace` → 移除
**理由**: v1.0 已实现 30s 自动轮询。手动刷新是 redundant API surface。

**操作**:
- 从 `mcp_tools.go` 删除注册
- `/api/workspace/refresh` Web API + tray handler **保留**（WebUI 手动刷新按钮）
- service 层 `RefreshWorkspace()` 方法保留（被自动轮询调用）

---

### M7: `disclosure` → 合并入 `init_project`
**理由**: `disclosure` 返回高优先级上下文，`init_project` 返回 rules + recent + hints。它们都服务于「agent 进入工程」这个动作，只是粒度不同。用 mode 参数区分。

**操作**:
```diff
- init_project     project → rules + recent + hints
- disclosure       mode, project → 上下文卡片
+ init_project     project, disclosure_mode? → rules + recent + hints + disclosure
```

**参数**:
- `disclosure_mode` 可选：`"first"` / `"subsequent"` / 空（不返回 disclosure）
- `disclosure_since` 可选：subsequent 模式的时间起点

**返回值合并**:
```json
{
  "project": "...",
  "rules": [...],
  "recent": [...],
  "hints": [...],
  "disclosure": { "cards": [...], "mode": "first" }  // 仅 disclosure_mode 非空时
}
```

- 删除 `disclosure` 注册

---

## 最终工具箱（16 个）

| # | 工具 | 用途 |
|---|------|------|
| 1 | `init_workspace` | 会话入口：工程列表 + 最近活动（合并了 list_projects 功能） |
| 2 | `init_project` | 工程初始化：rules + recent + hints + disclosure（合并了 disclosure） |
| 3 | `context_for_files` | 文件路径 → 自动关联记忆 |
| 4 | `recall` | 搜索（合并了 recall_all：不传 project 即跨工程） |
| 5 | `open_memory` | 详情获取（合并了 recall_batch：支持 ID 数组） |
| 6 | `remember` | 创建卡片（合并了 remember_batch + 内置去重警告） |
| 7 | `update_memory` | 更新卡片（含自动版本历史） |
| 8 | `delete_memory` | 删除卡片 |
| 9 | `list_recent` | 最近记忆 |
| 10 | `get_relations` | 关系图遍历 |
| 11 | `audit_project` | 工程审计 |
| 12 | `vacuum` | 压缩数据库 |
| 13 | `synthesize_rules` | 重新生成规则 |
| 14 | `check_rules_freshness` | 规则过期检查 |
| 15 | `list_templates` | 模板列表 |
| 16 | `extract_patterns` | 跨工程模式提取 |

---

## 不变的部分

| 文件 | 影响 |
|------|------|
| `internal/index/index.go` | 无变更 |
| `internal/store/` | 无变更 |
| `internal/memory/` | 无变更 |
| `internal/mcp/` | 无变更 |
| `internal/tray/tray.go` | 无变更（Web API 端点不变） |
| `web/index.html` | 无变更 |

## 变更文件

| 文件 | 变更内容 |
|------|---------|
| `internal/service/mcp_tools.go` | 重写工具注册：删 7 个，改 6 个签名 |
| `internal/service/service.go` | `Remember()` 返回值加 `warnings`；`OpenMemory()` 支持数组 |
| `internal/service/workspace.go` | `InitProject` 加 disclosure 逻辑 |
| `cmd/pmem/main.go` | Web API 端点无变更（保留所有 /api/* 端点） |

## 向后兼容

| 旧调用 | 新调用 | 兼容性 |
|--------|--------|--------|
| `list_projects()` | `init_workspace()` 返回的 `projects` 字段 | ⚠️ 返回结构不同（更丰富），需 agent 适配 |
| `recall_all(query)` | `recall(query)` 不传 project | ✅ 返回结构相同 |
| `remember_batch(memories)` | `remember(memory: [...])` | ✅ 返回结构相同（包装在 `results` 中） |
| `recall_batch(ids)` | `open_memory(id: [...])` | ✅ 返回结构相同 |
| `disclosure(mode, project)` | `init_project(project, disclosure_mode)` | ⚠️ 返回结构嵌入在 `disclosure` 字段中 |
| `refresh_workspace()` | 无需调用（自动） | ✅ 功能自动执行 |

---

## 验证

```bash
cd project-memory-palace
go build -o bin/pmem.exe ./cmd/pmem && go vet ./... && go test ./... -count=1
```

- 现有测试全部通过（M3/M4 需要改测试适配新签名）
- 手动验证：启动 pmem，用 MCP inspector 调 16 个工具，覆盖合并后的所有路径

## 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| Agent 调 `list_projects` 失败 | 低 | `init_workspace` 已在 agent 引导流程中被优先推荐 |
| `disclosure` 合并后 agent 不习惯新结构 | 中 | `disclosure` 字段独立，不影响 `rules`/`recent` |
| 批量 `remember` 数组检测边界情况 | 低 | 用类型断言 `switch v := params["memory"].(type)` |

---

## Commit message

```
refactor(v1.0): MCP tool convergence — 23 → 16

Merge semantic overlaps:
- list_projects → init_workspace (redundant)
- recall_all → recall (project param)
- remember_batch → remember (auto-detect array)
- recall_batch → open_memory (auto-detect array)
- check_duplicates warnings → remember built-in
- refresh_workspace → removed (30s auto-polling)
- disclosure → init_project (disclosure_mode param)

Zero functionality loss. Tool definition tokens ↓37%.
```
