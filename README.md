# 🏰 Project Memory Palace

> 本地项目记忆库 — 为 AI 编程代理而生。无需编译，一键安装。

`pmem` 在项目根目录创建 `.project-memory/` 持久化记忆卡片（YAML），通过 SQLite 索引提供快速搜索，并通过 MCP 协议向 Claude Desktop / Claude Code / Codex CLI 提供渐进披露的项目上下文。

---

## 一分钟安装

```bash
curl -fsSL https://github.com/Yarrow-Cai/project-memory-palace/releases/latest/download/pmem.exe -o %USERPROFILE%\.pmem\bin\pmem.exe
setx PATH "%PATH%;%USERPROFILE%\.pmem\bin"
```

或者运行一键安装脚本：

```bash
git clone https://github.com/Yarrow-Cai/project-memory-palace.git
cd project-memory-palace
install.bat
```

`install.bat` 自动完成：
1. 从 GitHub Release 下载最新 `pmem.exe` → `%USERPROFILE%\.pmem\bin\`
2. 添加到用户 PATH
3. 配置 **Claude Desktop** / **Claude Code** / **Codex CLI** 的 MCP 集成

---

## MCP 集成

### Claude Desktop
自动写入 `%APPDATA%\Claude\claude_desktop_config.json`，也可手动添加：

```json
{
  "mcpServers": {
    "project-memory-palace": {
      "command": "C:\\Users\\<用户名>\\.pmem\\bin\\pmem.exe",
      "args": ["serve-mcp", "."]
    }
  }
}
```

### Claude Code
```bash
claude mcp add project-memory-palace -- "%USERPROFILE%\.pmem\bin\pmem.exe" serve-mcp .
```

### Codex CLI
```bash
codex mcp add project-memory-palace -- "%USERPROFILE%\.pmem\bin\pmem.exe" serve-mcp .
```

---

## CLI 命令

| 命令 | 说明 |
|------|------|
| `pmem init` | 初始化项目（创建 `.project-memory/`） |
| `pmem remember --file card.yaml` | 写入记忆卡片 |
| `pmem search "关键词"` | 搜索记忆 |
| `pmem open <id>` | 查看完整卡片 |
| `pmem recent --limit 20` | 最近记忆列表 |
| `pmem update --status stale <id>` | 更新状态 |
| `pmem delete <id>` | 永久删除单条记忆 |
| `pmem purge` | 批量清理所有已过期记忆 |
| `pmem rebuild-index` | 重建索引 |
| `pmem audit` | 审计记忆健康度 |
| `pmem serve-web` | Web UI（http://127.0.0.1:8147） |
| `pmem serve-mcp` | MCP stdio 服务器 |

---

## MCP 工具

| 工具 | 说明 |
|------|------|
| `init_project` | 初始化 + 返回活跃规则和近期活动 |
| `remember` | 写入记忆（type, title, summary, content...） |
| `recall` | 关键词搜索（返回摘要） |
| `open_memory` | 按 ID 获取完整内容 |
| `update_memory` | 更新状态/置信度/过期时间/标签/关联 |
| `delete_memory` | 永久删除单条记忆（同时清理 YAML + 索引） |
| `list_recent` | 最近记忆 |
| `synthesize_rules` | 生成 agent-rules.yaml |
| `disclosure` | 渐进披露（first/subsequent 模式） |

---

## 记忆生命周期

每条记忆卡片有完整的状态流转：

```
active → stale / superseded / rejected → expired → 🗑️ 删除
```

| 状态 | 含义 | 操作 |
|------|------|------|
| `active` | 活跃，正常参与搜索和披露 | `remember` 默认状态 |
| `stale` | 信息过时但仍可参考 | `update_memory({status:"stale"})` |
| `superseded` | 被新卡片取代，保留供回溯 | 设置 `relations.supersedes` |
| `rejected` | 经过讨论被否定 | `update_memory({status:"rejected"})` |
| `expired` | 已过期，不再参与搜索 | 设置 `expires_at` 自动到期，或手动设 |

- 设置 `expires_at`（ISO 时间戳），到期后自动视为过期
- `pmem purge` / Web UI 批量清理所有 `expired` 卡片
- `delete_memory` / `pmem delete` 永久删除（含 YAML 文件和索引）
- `recall` / `disclosure` 默认排除 `expired` 状态

---

## Web UI

```bash
pmem serve-web
# 打开 http://127.0.0.1:8147
```

记忆列表 · 搜索 · 详情 · 项目管理 · 规则查看 · 统计 · 深色/浅色主题 · 可拖拽分隔线

---

## 记忆卡片格式

```yaml
type: decision              # 20 种类型：convention, decision, architecture, schematic, pinout,
                            # power, clock, driver, register, peripheral, dma, protocol,
                            # timing, interrupt, state_machine, change_reason, knowledge,
                            # insight, pattern, trick, bug
title: 选择 FreeRTOS 作为调度核心
summary: 因为任务间同步需求，决定使用 FreeRTOS 替代裸机循环
content: >
  经过对比，FreeRTOS 的任务隔离和信号量机制更适合逆变器控制...
confidence: 0.85
status: active
priority: 4                 # 1-5，决定是否参与渐进披露
expires_at: ""              # ISO 时间戳，空 = 永不过期
tags: [rtos, freertos, architecture]
source:
  kind: analysis
  description: 基于项目需求分析
scope:
  modules: [inverter-ctrl, comm-handler]
  paths: [src/inverter/task.c]
```

---

## 从源码编译

```bash
git clone https://github.com/Yarrow-Cai/project-memory-palace.git
cd project-memory-palace/project-memory-palace
go build -o bin/pmem.exe ./cmd/pmem
```

---

## 项目结构

```
project-memory-palace/
├── cmd/pmem/          # CLI 入口
├── internal/
│   ├── memory/        # 卡片模型 + 校验
│   ├── store/         # 卡片读写
│   ├── index/         # SQLite 索引
│   ├── service/       # 业务逻辑
│   ├── mcp/           # MCP 协议 + SSE
│   ├── rule/          # agent-rules 合成
│   ├── audit/         # 健康度审计
│   └── tray/          # 托盘应用
├── web/               # Web UI
├── install.bat        # 一键安装脚本
└── README.md
```

---

## License

MIT
