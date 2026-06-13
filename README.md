# 🏰 Project Memory Palace

> 本地项目记忆库 — 为 AI 编程代理而生。

`pmem` 在项目根目录创建 `.project-memory/` 持久化记忆卡片（YAML），通过 SQLite 索引提供快速搜索，并通过 MCP 协议向 Claude Desktop / Codex CLI 提供渐进披露的项目上下文。

---

## 快速安装

```bash
# 克隆仓库，运行安装脚本
git clone https://github.com/Yarrow-Cai/project-memory-palace.git
cd project-memory-palace
install.bat
```

安装脚本自动完成：
1. 编译并安装 `pmem.exe` 到 `%USERPROFILE%\.pmem\bin\`
2. 添加到用户 PATH
3. 配置 **Claude Desktop** MCP（写入 `claude_desktop_config.json`）
4. 配置 **Codex CLI** MCP（`codex mcp add`）

---

## 手动安装

### 前置条件
- Go 1.21+
- （可选）Claude Desktop 或 Codex CLI

### 从源码编译

```bash
git clone https://github.com/Yarrow-Cai/project-memory-palace.git
cd project-memory-palace/project-memory-palace
go build -o bin/pmem.exe ./cmd/pmem
```

### 配置 Claude Desktop

编辑 `%APPDATA%\Claude\claude_desktop_config.json`，添加：

```json
{
  "mcpServers": {
    "project-memory-palace": {
      "command": "C:\\Users\\<你的用户名>\\.pmem\\bin\\pmem.exe",
      "args": ["serve-mcp", "."]
    }
  }
}
```

### 配置 Codex CLI

```bash
codex mcp add project-memory-palace -- "%USERPROFILE%\.pmem\bin\pmem.exe" serve-mcp .
```

验证：`codex mcp list`

---

## CLI 命令

```bash
pmem init                          # 初始化当前项目（创建 .project-memory/）
pmem remember --file card.yaml     # 写入记忆卡片
pmem search "关键词"                # 搜索记忆（返回摘要 + ID）
pmem open <id>                     # 查看完整卡片内容
pmem recent --limit 20             # 最近记忆列表
pmem update --status stale <id>    # 标记为过时
pmem rebuild-index                 # 重建搜索索引
pmem audit                         # 审计项目记忆健康度
pmem serve-web                     # 启动 Web UI（http://127.0.0.1:8147）
pmem serve-mcp                     # 启动 MCP stdio 服务器（给 AI 代理用）
```

---

## MCP 工具（AI 代理可用）

| 工具 | 说明 |
|------|------|
| `init_project` | 初始化项目记忆，返回活跃规则 + 近期活动 + 下一步指南 |
| `remember` | 写入一条记忆（type, title, summary, content, confidence, tags...） |
| `recall` | 按关键词搜索记忆（返回摘要） |
| `open_memory` | 按 ID 获取完整记忆内容 |
| `update_memory` | 更新状态 / 置信度 / 标签 / 关联 |
| `list_recent` | 列出最近记忆 |
| `synthesize_rules` | 从 convention / decision 类型记忆生成 agent-rules.yaml |
| `disclosure` | 渐进披露：first 模式返回高优先级全貌，subsequent 返回核心 + 变更 |

---

## Web UI

```bash
pmem serve-web
# 打开 http://127.0.0.1:8147
```

功能：记忆列表 | 搜索 | 详情查看 | 项目切换与管理 | 规则查看 | 统计 | 深色/浅色主题

---

## 项目结构

```
project-memory-palace/
├── cmd/pmem/          # CLI 入口
├── internal/
│   ├── memory/        # 卡片模型 + 校验
│   ├── store/         # 卡片读写 + 身份 ID
│   ├── index/         # SQLite 全文搜索索引
│   ├── service/       # 业务逻辑
│   ├── mcp/           # MCP 协议 + SSE 服务器
│   ├── rule/          # agent-rules 合成
│   ├── audit/         # 记忆健康度审计
│   └── tray/          # Windows 托盘应用
├── web/               # Web UI（单文件 index.html）
├── install.bat        # 一键安装脚本
└── README.md
```

---

## 记忆卡片格式

`remember` 写入 YAML 卡片到 `.project-memory/cards/`：

```yaml
type: decision              # project_goal | design | decision | change_reason
                            # bugfix | module | convention | open_question
title: 选择 FreeRTOS 作为调度核心
summary: 因为任务间同步需求，决定使用 FreeRTOS 替代裸机循环
content: >
  经过对比裸机 while 循环和 FreeRTOS，
  后者的任务隔离和信号量机制更适合逆变器控制逻辑...
confidence: 0.85
status: active
tags: [rtos, freertos, architecture]
source:
  kind: analysis
  description: 基于项目需求分析
scope:
  modules: [inverter-ctrl, comm-handler]
  paths: [src/inverter/task.c, src/comm/handler.c]
relations:
  supersedes: []
```

---

## License

MIT
