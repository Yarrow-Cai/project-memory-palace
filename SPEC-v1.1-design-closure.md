# SPEC: pmem v1.1 — Design Closure

> Covers: P0 SetWorkspace bug + config.yaml redesign + tool reduction + correctness lifecycle + multi-agent awareness

---

## Part 1: P0 Fix — SetWorkspace Single-Project Fallback

### Bug
`init_workspace(workspace="D:\CYC\PROJUSE\YTC_code")` 返回 `"no projects found"`。
`SetWorkspace` 只扫描子目录中的 `.project-memory/`，不检查 path 本身。

### Fix
`WorkspaceService.SetWorkspace()` 加 fallback：

```go
func (ws *WorkspaceService) SetWorkspace(path string, ...) error {
    // 现有: 扫描子目录
    entries, _ := os.ReadDir(path)
    for _, e := range entries {
        if e.IsDir() && hasDotProjectMemory(filepath.Join(path, e.Name())) {
            // 添加到 projects map
        }
    }
    // NEW fallback: 检查 path 本身是否有 .project-memory/
    if len(newProjects) == 0 {
        projMemoryDir := filepath.Join(path, ".project-memory")
        if info, err := os.Stat(projMemoryDir); err == nil && info.IsDir() {
            name := filepath.Base(path)
            newProjects[name] = New(path)
            firstProj = name
        }
    }
    if len(newProjects) == 0 {
        return fmt.Errorf("no projects found in %s", path)
    }
    // ...
}
```

**改动**: `internal/service/workspace.go` 的 `SetWorkspace()` — 单文件，+8 行。

---

## Part 2: config.yaml Redesign — Single Source of Truth

### 问题
当前有两套配置系统互不知道对方：
- `%APPDATA%/config.json` — workspace + default_project（我刚加的）
- `.project-memory/config.yaml` — 每个工程一份，但目前是空的

### 设计
**合并到 config.yaml。** 删除 `config.json`，config.yaml 成为唯一配置入口。

```yaml
# .project-memory/config.yaml (v2)
schema_version: 2
project: "INVERTER_PFC"
description: "1KW 双向逆变器 PFC + LLC 固件"
created_at: "2025-06-01T00:00:00Z"
workspace_path: "D:/CYC/PROJUSE"    # NEW: 所属 workspace
default: true                         # NEW: 是否此 workspace 的默认工程
modules:                              # NEW (optional)
  - driver/pfc
  - driver/llc
  - comm/can
```

### 好处
1. **单工程检测**：`SetWorkspace` 检查 `path/.project-memory/config.yaml` 是否存在 → 直接识别为单工程
2. **workspace 归属**：每个 config.yaml 知道自己的 workspace，pmem 可以反向推导
3. **`init_project` 返回内容**：大部分元信息（project name, description, modules）来自 config.yaml，不需要运行时计算
4. **工具间信息一致性**：不再有 config.json 和 config.yaml 说不同话的风险

### 迁移
- 现有 config.yaml → 自动升级 schema_version 1→2，保留已有字段
- `config.json` → 启动时如果存在，读取并合并到第一个工程的 config.yaml，然后删除 config.json
- 新增的 `workspace_path`/`default`/`modules` 字段为空时不强制要求

### 改动
| 文件 | 变更 |
|------|------|
| `internal/store/config.go` | **REWRITE**: 删 config.json 逻辑，改读写 config.yaml |
| `internal/store/layout.go` | `defaultConfigYAML` → schema_version 2 |
| `internal/service/workspace.go` | 删 `%APPDATA%` 依赖，改用 config.yaml |
| `internal/service/mcp_tools.go` | `init_workspace` + `init_project` 读 config.yaml |
| `cmd/pmem/main.go` | 启动时从 config.yaml 读 workspace path |

---

## Part 3: Tool Reduction — 16 → 14

### 移除 `synthesize_rules` (MCP #13)
**理由**: 规则生成不该是 agent 手动触发的操作。它应该是自动的。
**替代**: 每次 `remember()` 或 `update_memory()` 成功后，自动触发增量规则更新。Agent 不需要知道"规则要重新生成"这件事。

### 移除 `check_rules_freshness` (MCP #14)
**理由**: 规则是否过期应该作为 `init_project` 返回值的一部分。
**替代**: `init_project` 返回时附带 `rules_fresh: true/false` + `rules_stale_since: "timestamp"`。Agent 不需要单独调一个工具来检查。

### 最终工具清单：14

```
入口 (3): init_workspace · init_project · context_for_files
搜索 (3): recall · open_memory · get_relations
写入 (3): remember · update_memory · delete_memory
运维 (2): audit_project · vacuum
知识 (3): list_recent · list_templates · extract_patterns
```

---

## Part 4: Card Correctness Lifecycle

### 问题
当前状态机只有 5 个状态，全是"有用/没用"维度。缺失"可验证性"维度。

### 新增状态
```
active ──→ needs_review ──→ verified ──→ active  (人工或 Agent 验证环)
active ──→ outdated_by:<commit>                    (被特定 commit 推翻)
stale  ──→ outdated_by:<commit>                    (过时 + 知道原因)
```

### 新字段: `MemoryCard`
```go
type MemoryCard struct {
    // ... 现有字段 ...
    VerifiedBy   string `yaml:"verified_by,omitempty"`   // 验证者 agent ID
    VerifiedAt   string `yaml:"verified_at,omitempty"`   // ISO timestamp
    OutdatedBy   string `yaml:"outdated_by,omitempty"`   // 导致过时的 commit SHA
}
```

### `update_memory` 新能力
```json
// Agent 验证某张卡片：
update_memory(id="mem_...", updates={status: "verified", verified_by: "claude-code"})

// Agent 发现卡片被某 commit 推翻：
update_memory(id="mem_...", updates={status: "outdated", outdated_by: "a1b2c3d"})
```

### WebUI 展示
卡片详情面板新增"验证状态"行：🟢 Verified by claude-code / 🟡 Needs Review / 🔴 Outdated by a1b2c3d

---

## Part 5: Multi-Agent Change Awareness

### 问题
Agent A 写了一张卡片，Agent B 永远不会自动知道。

### 方案（不引入 webhook/SSE 推送，保持简单）
在以下工具的返回值中加入 `recent_changes` 字段：

#### `init_project` 返回变更摘要
```json
{
  "project": "INVERTER_PFC",
  "rules": [...],
  "recent": [...],
  "recent_changes": {                    // NEW
    "since": "2026-07-09T20:00:00Z",
    "new_cards": 3,
    "updated_cards": 1,
    "agents_active": ["claude-code", "codex-cli"],
    "highlights": [
      {"id": "mem_...", "title": "PFC频率修正", "by": "codex-cli", "when": "..."}
    ]
  }
}
```

#### `context_for_files` 返回交叉 Agent 提示
```json
{
  "results": [...],
  "cross_agent_hint": "claude-code 在 20 分钟前修改了 driver/pfc.c 的关联卡片"  // NEW
}
```

### 无需新工具，无需推送基础设施
Agent 在自然的工作流中（调 `init_project`、`context_for_files`）就能被动感知到其他 Agent 的活动。

---

## Part 6: 不改的（明确边界）

| 项目 | 决定 | 原因 |
|------|------|------|
| 类型 schema（Gap A） | 不做 | 需要重构 MemoryCard 结构，影响面太大 |
| 语义搜索（Gap B） | 不做 | 需要 embedding 模型依赖，超出 pmem 的轻量定位 |
| Webhook/SSE 推送 | 不做 | 保持零依赖，用返回值嵌入变更摘要替代 |

---

## 执行计划

### Phase 1: P0 Fix
- `workspace.go`: SetWorkspace fallback — 单文件，+8 行
- 构建 → 测试 → 提交

### Phase 2: config.yaml 重构
- 删 `%APPDATA%/config.json`
- config.yaml v2 schema
- 所有读写统一走 config.yaml

### Phase 3: 工具收敛 + 自动规则
- 删 `synthesize_rules` + `check_rules_freshness` MCP 注册
- `remember`/`update_memory` 自动触发增量规则更新
- `init_project` 返回 `rules_fresh` 字段

### Phase 4: 卡片生命周期
- `constants.go`: 加 `needs_review`/`verified` 状态 + `OutdatedBy`/`VerifiedBy` 字段
- `update_memory`: 支持新状态
- WebUI: 验证状态展示

### Phase 5: 多 Agent 变更感知
- `service.go`: `InitProject` 返回 `recent_changes`
- `index.go`: 新增 `RecentChanges(since)` SQL 查询
- mcp_tools: `init_project` + `context_for_files` 返回值扩展

---

## 验证

```bash
go build -o bin/pmem.exe ./cmd/pmem && go vet ./... && go test ./... -count=1
```

- Phase 1-3 各独立构建验证
- Phase 4-5 需要新增测试覆盖新状态转换

---

## 最终效果

| 指标 | v1.0 现状 | v1.1 目标 |
|------|----------|----------|
| MCP 工具 | 16 | **14** |
| SetWorkspace | ❌ 单工程炸 | ✅ fallback |
| 配置系统 | 两套分裂 | config.yaml 统一 |
| 规则生成 | 手动触发 | 自动增量 |
| 卡片状态 | 5 种（全"有用/无用"） | 7 种（+可验证性维度） |
| 多 Agent 感知 | ❌ 轮询 | ✅ init_project 被动感知 |
| 代码改动量 | — | ~300 行（5 个文件组） |
