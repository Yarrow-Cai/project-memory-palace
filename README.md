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

## Local Smoke Test

```bash
pmem init
pmem remember --file example-card.yaml
pmem search "YAML"
pmem recent --limit 5
pmem rebuild-index
pmem audit
```

`pmem search` returns summaries and IDs. Use `pmem open <id>` to inspect full YAML.

## MCP Usage

Run:

```bash
pmem-mcp
```

Expose the command to an MCP-compatible AI client. The AI should call:

- `remember` after useful work.
- `recall` before loading project context.
- `open_memory` only when details are needed.
- `update_memory` when a memory becomes stale or superseded.
- `list_recent` to report recent automatic writes.
