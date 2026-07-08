# pmem 多工程工作区模式 — 实现计划

> **For Hermes:** Use codex exec to implement, then manual verification.
>
> **Goal:** 让 pmem MCP server 支持一个 workspace 目录下同时服务多个工程，每个查询工具可指定 `project` 参数。
>
> **Problem:** 当前 `serve-mcp <dir>` 只能绑定单个工程。Agent 在不同工程间切换时，MCP 返回的数据永远是绑定工程的数据，无法区分。
>
> **Architecture:** 新增 `WorkspaceService` 层，管理 `projectName → *MemoryService` 映射。所有 MCP 工具加 `project` 可选参数。

---

## 改动范围

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `internal/service/workspace.go` | **新建** | WorkspaceService：扫描、路由、listProjects |
| `internal/service/mcp_tools.go` | 修改 | 所有 11 个工具加 `project` 参数 + 新增 `list_projects` |
| `cmd/pmem/main.go` | 修改 | cmdServeMCP 改用 WorkspaceService |
| `internal/store/paths.go` | 修改 | 新增 workspace 级别路径函数 |

不动的文件：`index.go`, `card.go`, `constants.go`, `helpers.go`, `audit.go`, `decay.go`（都是 per-project 逻辑，无需修改）。

---

## Task 1: 新建 WorkspaceService

**文件:** `internal/service/workspace.go`（新文件）

```go
package service

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// WorkspaceService manages multiple pmem projects under one workspace directory.
// It discovers .project-memory/ subdirectories and routes queries to the correct project.
type WorkspaceService struct {
    workspaceDir string
    projects     map[string]*MemoryService // projectName -> service
    defaultProj  string                    // first project found, used when no project specified
}

// NewWorkspace scans the workspace directory for project subdirectories
// that contain .project-memory/, creates a MemoryService for each, and
// returns a WorkspaceService that can route queries.
func NewWorkspace(workspaceDir string) (*WorkspaceService, error) {
    ws := &WorkspaceService{
        workspaceDir: workspaceDir,
        projects:     make(map[string]*MemoryService),
    }

    entries, err := os.ReadDir(workspaceDir)
    if err != nil {
        return nil, fmt.Errorf("read workspace dir: %w", err)
    }

    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }
        projectRoot := filepath.Join(workspaceDir, entry.Name())
        memDir := filepath.Join(projectRoot, ".project-memory")
        if info, err := os.Stat(memDir); err != nil || !info.IsDir() {
            continue
        }
        svc := New(projectRoot)
        if err := svc.InitProject(); err != nil {
            continue // skip broken projects
        }
        ws.projects[entry.Name()] = svc
        if ws.defaultProj == "" {
            ws.defaultProj = entry.Name()
        }
    }

    return ws, nil
}

// resolve returns the MemoryService for the given project name.
// If name is empty, returns the default project.
func (ws *WorkspaceService) resolve(project string) (*MemoryService, string, error) {
    if project == "" {
        if ws.defaultProj == "" {
            return nil, "", fmt.Errorf("no projects found in workspace")
        }
        project = ws.defaultProj
    }
    svc, ok := ws.projects[project]
    if !ok {
        // Try case-insensitive match
        for name, s := range ws.projects {
            if strings.EqualFold(name, project) {
                return s, name, nil
            }
        }
        return nil, "", fmt.Errorf("project %q not found. Available: %s", project, ws.ProjectNames())
    }
    return svc, project, nil
}

// ProjectNames returns all discovered project names.
func (ws *WorkspaceService) ProjectNames() []string {
    names := make([]string, 0, len(ws.projects))
    for name := range ws.projects {
        names = append(names, name)
    }
    return names
}

// ListProjects returns project metadata for the list_projects tool.
func (ws *WorkspaceService) ListProjects() ([]map[string]any, error) {
    var result []map[string]any
    for name, svc := range ws.projects {
        count, _ := svc.Count(nil)
        result = append(result, map[string]any{
            "name":          name,
            "project_root":  svc.ProjectRoot(),
            "card_count":    count,
            "is_default":    name == ws.defaultProj,
        })
    }
    return result, nil
}

// AutoDetect tries to determine which project a set of file paths belongs to.
// Returns the project name that contains the most matching paths.
func (ws *WorkspaceService) AutoDetect(paths []string) string {
    bestName := ""
    bestCount := 0
    for name, svc := range ws.projects {
        root := svc.ProjectRoot()
        count := 0
        for _, p := range paths {
            // Normalize both paths for comparison
            absPath := p
            if !filepath.IsAbs(p) {
                absPath = filepath.Join(root, p)
            }
            rel, err := filepath.Rel(root, absPath)
            if err == nil && !strings.HasPrefix(rel, "..") {
                count++
            }
        }
        if count > bestCount {
            bestCount = count
            bestName = name
        }
    }
    return bestName
}

// Close closes all underlying project services.
func (ws *WorkspaceService) Close() error {
    for _, svc := range ws.projects {
        _ = svc.Close()
    }
    return nil
}

// extractProject extracts the "project" parameter from MCP tool params.
// Returns the project name, or empty string if not specified.
func extractProject(params map[string]any) string {
    if p, ok := params["project"].(string); ok {
        return p
    }
    return ""
}
```

---

## Task 2: MCP 工具加 `project` 参数

**文件:** `internal/service/mcp_tools.go`

### 2.1 改造 RegisterAllTools 签名

当前签名：
```go
func RegisterAllTools(reg *mcp.ToolRegistry, svc *MemoryService, projectRoot string, wrapHandler func(mcp.ToolHandler) mcp.ToolHandler)
```

改为接受 `WorkspaceService`：
```go
func RegisterAllTools(reg *mcp.ToolRegistry, ws *WorkspaceService, wrapHandler func(mcp.ToolHandler) mcp.ToolHandler)
```

### 2.2 11 个已有工具加 `project` 参数

在每个 tool definition 的 `properties` 最前面加：
```go
"project": map[string]any{
    "type":        "string",
    "description": "目标工程名称。不填则使用默认工程。可用 list_projects 查看所有工程。",
},
```

每个 handler 开头加：
```go
svc, projName, err := ws.resolve(extractProject(params))
if err != nil { return nil, err }
```

### 2.3 新增 `list_projects` 工具（第 12 个）

```go
reg.Register("list_projects", "列出当前工作区所有可用工程及其卡片数量。用于了解有哪些工程、选择 project 参数。", map[string]any{
    "type": "object", "properties": map[string]any{},
}, wrap(func(params map[string]any) (any, error) {
    projects, err := ws.ListProjects()
    if err != nil { return nil, err }
    return map[string]any{
        "workspace": ws.workspaceDir,
        "projects":  projects,
        "total":     len(projects),
    }, nil
}))
```

### 2.4 `context_for_files` 自动检测工程

特殊处理：当用户未指定 `project` 时，根据 `paths` 参数自动推断：
```go
project := extractProject(params)
if project == "" {
    rawPaths, _ := params["paths"].([]any)
    paths := make([]string, len(rawPaths))
    for i, p := range rawPaths { paths[i] = fmt.Sprint(p) }
    project = ws.AutoDetect(paths)
}
svc, _, err := ws.resolve(project)
```

---

## Task 3: cmdServeMCP 改用 WorkspaceService

**文件:** `cmd/pmem/main.go`

```go
func cmdServeMCP(args []string) int {
    if len(args) > 0 { projectRoot = args[0] }

    // Try workspace mode first: scan for .project-memory/ subdirs
    ws, err := service.NewWorkspace(projectRoot)
    if err != nil || len(ws.ProjectNames()) == 0 {
        // Fallback: single-project mode (backward compatible)
        svc, svcErr := newService()
        if svcErr != nil { ... }
        reg := mcp.NewToolRegistry()
        // Wrap single svc in a workspace with one project
        ws = &service.WorkspaceService{...}  // or simpler: add a NewSingleProject helper
    }

    reg := mcp.NewToolRegistry()
    service.RegisterAllTools(reg, ws, nil)
    srv := &mcp.StdioServer{Registry: reg, Reader: os.Stdin, Writer: os.Stdout}
    defer ws.Close()
    ...
}
```

实际上更干净的方案：`WorkspaceService` 增加一个单工程兼容构造函数：

```go
// internal/service/workspace.go
func NewSingleProject(projectRoot string) (*WorkspaceService, error) {
    svc := New(projectRoot)
    if err := svc.InitProject(); err != nil {
        return nil, err
    }
    name := filepath.Base(projectRoot)
    return &WorkspaceService{
        workspaceDir: projectRoot,
        projects:     map[string]*MemoryService{name: svc},
        defaultProj:  name,
    }, nil
}
```

然后 `cmdServeMCP` 就是一行的改动：
```go
func cmdServeMCP(args []string) int {
    if len(args) > 0 { projectRoot = args[0] }
    ws, err := service.NewWorkspace(projectRoot)
    if err != nil || len(ws.ProjectNames()) == 0 {
        ws, err = service.NewSingleProject(projectRoot)
    }
    if err != nil { ... }
    reg := mcp.NewToolRegistry()
    service.RegisterAllTools(reg, ws, nil)
    ...
}
```

---

## Task 4: Web API 改造（可选，最小改动）

**文件:** `cmd/pmem/main.go` — cmdServeWeb

同样改为 WorkspaceService 模式，`/api/project` 端点返回所有工程列表而不是单工程。

---

## Task 5: 构建 + 测试

```bash
go build -o bin/pmem.exe ./cmd/pmem
go test ./internal/... -v
```

验证：
- [ ] 在包含多个 `.project-memory/` 的 workspace 下启动 `serve-mcp`
- [ ] `list_projects` 返回所有工程
- [ ] `disclosure(project="YTC_code")` 只返回 YTC 的卡片
- [ ] `disclosure(project="INVERTER_PFC")` 只返回逆变器的卡片
- [ ] 不传 `project` 时使用默认工程（向后兼容）
- [ ] 单工程目录下 `serve-mcp` 正常降级
