# SPEC: Autonomous Workspace & Project Selection

> **Problem**: pmem MCP server is locked to a single workspace path at startup. Agent has zero ability to switch workspaces or select projects through MCP tools. It's a "hardcoded database" rather than a "navigable memory space."

---

## Design

### Core idea: config.json + `set_workspace` MCP tool

```
┌──────────────────────────────────────────┐
│  %APPDATA%/project-memory-palace/        │
│  ├── config.json     ← NEW 持久化配置     │
│  ├── recents.json    ← 已有               │
│  └── ...                                  │
└──────────────────────────────────────────┘
```

`config.json`:
```json
{
  "workspace": "C:\\Users\\Atop\\Desktop\\pmem",
  "default_project": "YTC_code"
}
```

### Startup flow

```
pmem serve-mcp              → 从 config.json 读 workspace → 启动
                               无 config → 回退到 当前目录 "."

pmem serve-mcp <path>       → 直接用 <path>（向后兼容，同时更新 config）
pmem serve-mcp --config      → 显式声明从 config 读取
```

### Runtime: Agent 自主切换

```
Agent 调 set_workspace("/home/user/new-workspace")
  → 写 config.json
  → 重建 WorkspaceService（re-scan 子目录）
  → 返回 workspace info（等效于 init_workspace 输出）
  → MCP server 继续运行，只是 workspace 变了

Agent 调 set_default_project("INVERTER_PFC")
  → 写 config.json
  → 更新 WorkspaceService.defaultProj
  → 后续不传 project 参数的调用默认指向此工程
```

---

## New MCP Tools

### 工具 17: `set_workspace`

| 字段 | 值 |
|------|-----|
| **名称** | `set_workspace` |
| **用途** | 切换 pmem 工作区目录。Agent 可以用它自主导航到不同的项目集合。 |
| **参数** | `path` (string, required) — 工作区根目录路径 |
|  | `default_project` (string, optional) — 设为默认工程 |
| **返回值** | 与 `init_workspace` 相同格式：`{workspace, projects[], default_project, total_cards}` |
| **副作用** | 写 config.json，重建 WorkspaceService |

### 工具 18: `set_default_project`

| 字段 | 值 |
|------|-----|
| **名称** | `set_default_project` |
| **用途** | 设置当前工作区内的默认工程。后续不传 project 参数的工具调用均指向此工程。 |
| **参数** | `project` (string, required) — 工程名称 |
| **返回值** | `{default_project, project_root, card_count}` |
| **副作用** | 写 config.json |

---

## Files to Create/Modify

### NEW: `internal/store/config.go`
```go
type PMemConfig struct {
    Workspace      string `json:"workspace"`
    DefaultProject string `json:"default_project"`
}

func LoadConfig() (*PMemConfig, error)   // reads from APPDATA/project-memory-palace/config.json
func SaveConfig(cfg *PMemConfig) error   // writes atomically
```

- Config path: `%APPDATA%/project-memory-palace/config.json`
- Fallback when file missing: return zero-value config (no error)
- Atomic write: write to temp file → rename

### MODIFY: `cmd/pmem/main.go`

| 改动 | 说明 |
|------|------|
| `cmdServeMCP()` | 无参数时从 config 读 workspace；有参数时写入 config |
| `cmdServeWeb()` | 同上 |
| `run()` tray 入口 | 已有 `tray.Run(".")` → 改成 `tray.Run(config.Workspace)` |
| `/api/project/set` | 切换工程时同步更新 config.json 的 `default_project` |

### MODIFY: `internal/service/mcp_tools.go`

| 改动 | 说明 |
|------|------|
| 新增 `set_workspace` 注册 (#17) | 调 `ws.SetWorkspace(path, defaultProject)` |
| 新增 `set_default_project` 注册 (#18) | 调 `ws.SetDefaultProject(name)` |
| `init_workspace` hints 更新 | 提示 `set_workspace` / `set_default_project` 可用 |

### MODIFY: `internal/service/workspace.go`

| 改动 | 说明 |
|------|------|
| 新增 `ws.SetWorkspace(path, defaultProject)` | 重建 projects map，更新 config |
| 新增 `ws.SetDefaultProject(name)` | 改 defaultProj，验工程存在，更新 config |
| 新增 `ws.SetConfig(cfg)` | 内部方法，统一 config 读写 |
| 可选：`sync.RWMutex` | 保护 projects map 的并发访问 |

### MODIFY: `internal/tray/tray.go`

| 改动 | 说明 |
|------|------|
| `Run()` 启动 | 无参数时从 config 读 workspace |
| `/api/project/set` handler | 更新 config.json |
| recents 处理 | 不冲突——config.json 存 workspace+default_project，recents.json 存历史列表 |

---

## Backward Compatibility

| 场景 | 行为 |
|------|------|
| `pmem serve-mcp .` | 和以前一样，同时写 config |
| `pmem serve-mcp /some/path` | 和以前一样，同时写 config |
| `pmem serve-mcp` (无参数) | **新行为**: 从 config 读 workspace，回退 `.` |
| `pmem serve-web` (无参数) | 同上 |
| Double-click tray icon | 同上（tray 无参启动 → 读 config） |
| install.bat 配置 | `"args": ["serve-mcp", "."]` → 保持不变，第一次启动后 config 被写入 |

---

## WebUI 变化

| 组件 | 变化 |
|------|------|
| 项目切换 `/api/project/set` | 额外写 config.json 的 `default_project` |
| "管理项目" 模态 | 增加 "设为默认" 按钮 |
| 启动默认工程 | WebUI 加载时从 config 读 default_project，自动选中 |

---

## MCP Tool Count

| 变更 | 数量 |
|------|------|
| 当前 | 16 |
| 新增 `set_workspace` | 17 |
| 新增 `set_default_project` | **18** |

---

## 并发安全

### 问题
`set_workspace` 重建 `WorkspaceService.projects` map 时，其他 MCP 工具可能正在读它。当前没有任何锁保护。

### 方案
给 `WorkspaceService` 加 `sync.RWMutex`：
- 所有读操作（resolve, ListProjects, RecallAll, etc.） → `RLock`
- `set_workspace`（重建 projects） → `Lock`

这是一个独立的小改动，但对正确性至关重要——放在本 SPEC 中一起做。

---

## 验证

```bash
# 1. 功能测试
pmem serve-mcp                    # 无参数 → 从 config 启动
# Agent 调 set_workspace("/tmp/test-ws")  # 切换到测试工作区
# Agent 调 init_workspace()               # 验证已切换

# 2. 构建
go build -o bin/pmem.exe ./cmd/pmem && go vet ./... && go test ./... -count=1

# 3. 配置文件检查
cat %APPDATA%/project-memory-palace/config.json  # 确认写入正确
```

---

## 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| 并发读写 WorkspaceService | **中** | 加 RWMutex |
| config.json 写入失败（磁盘满/权限） | 低 | 原子写 + 回退到内存值 |
| 与 recents.json 数据重复 | 低 | 两个文件独立，用途不同：config=当前选择，recents=历史 |
| `set_workspace` 后已有 agent 内存中的旧 project 引用失效 | 低 | agent 应该在 `set_workspace` 后重新调 `init_workspace`（hints 会提示） |

---

## 工具数：16 → 18

```
入口层 (4)    init_workspace · init_project · context_for_files · set_workspace  ← +1
搜索层 (3)    recall · open_memory · get_relations
写入层 (3)    remember · update_memory · delete_memory
运维层 (4)    audit_project · vacuum · synthesize_rules · check_rules_freshness
知识层 (4)    list_recent · list_templates · extract_patterns · set_default_project  ← +1
```
