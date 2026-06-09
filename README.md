# Project Memory Palace

Local project memory for AI agents.

The first version stores durable project knowledge as YAML cards under `.project-memory/cards/`.
SQLite is a rebuildable search index. AI clients use MCP tools for progressive recall, and users manage the memory base with the `pmem` CLI.

## Development

```bash
python -m pip install -e ".[dev]"
pytest
```

## Commands

```bash
pmem init
pmem search "query"
pmem recent --limit 10
pmem-mcp
```
