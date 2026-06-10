# Project Memory Palace — Go Rewrite Design

## Goal

Rewrite the existing Python Project Memory Palace in Go, preserving full feature
parity across the CLI, MCP server, and desktop GUI, while upgrading the GUI to a
modern Fyne-based interface that improves on the current CustomTkinter
implementation in look, feel, and responsiveness.

## Scope

This version covers everything the Python v0.1.0 does:

- **CLI** (`pmem`): init, remember, search, open, recent, update, rebuild-index, audit
- **MCP server** (`pmem-mcp`): remember, recall, open_memory, update_memory, list_recent (via stdio JSON-RPC)
- **Desktop GUI** (`pmem-gui`): Fyne-based window with dark/light theme, bilingual (en/zh), project browsing, search, memory detail view, status management, system tray, MCP server start/stop
- **Core**: YAML-backed card storage, SQLite FTS5 search index, schema validation, relation tracking, confidence rules, audit

The following are explicitly out of scope for this version:

- Embedding or vector search
- Web UI
- Team sync or user accounts
- Auto-delete memory tool
- Plugin system

## Directory Layout

```
project-memory-palace/
├── cmd/
│   ├── pmem/              # CLI entrypoint
│   │   └── main.go
│   ├── pmem-mcp/          # MCP server entrypoint
│   │   └── main.go
│   └── pmem-gui/          # Fyne desktop GUI entrypoint
│       └── main.go
├── internal/
│   ├── memory/            # Data model + validation + constants
│   │   ├── card.go        #   MemoryCard struct, from_dict, to_dict
│   │   ├── validate.go    #   Schema validation
│   │   └── constants.go   #   MemoryTypes, Statuses, SourceKinds, RelationKinds
│   ├── store/             # YAML file I/O + directory layout
│   │   ├── paths.go       #   Directory/file path resolution
│   │   ├── layout.go      #   ensure_project_memory, assert_memory_layout
│   │   ├── card_io.go     #   read_card, write_card, discover_cards, card_filename
│   │   └── identity.go    #   ID generation (mem_YYYYMMDD_NNN)
│   ├── index/             # SQLite FTS5 index
│   │   ├── index.go       #   MemoryIndex struct, initialize, search, recent, upsert, rebuild, clear
│   │   └── schema.sql     #   DDL statements (embedded)
│   ├── service/           # Business logic layer
│   │   ├── service.go     #   MemoryService struct + Remember / Recall / Open / ListRecent / Update / Rebuild
│   │   └── errors.go      #   MemoryNotFoundError, etc.
│   ├── audit/             # Quality reports
│   │   └── audit.go       #   AuditProject -> list of issues per card
│   ├── i18n/              # Bilingual translation
│   │   └── i18n.go        #   T(key) function, SetLanguage / GetLanguage, translation tables
│   └── mcp/               # MCP JSON-RPC protocol helpers
│       └── protocol.go    #   Tool registry, request/response types, stdio transport
├── docs/
│   └── superpowers/
│       └── specs/
│           └── 2026-06-10-project-memory-palace-go-design.md
├── go.mod
├── go.sum
└── Makefile
```

### Package boundaries

| Package | Depends on | Purpose |
|---------|-----------|---------|
| `memory` | - | Pure data structures and validators, no I/O |
| `store` | `memory`, `gopkg.in/yaml.v3` | File system interaction for YAML cards |
| `index` | `store`, `modernc.org/sqlite` | SQLite FTS5 search index, rebuildable from cards |
| `service` | `store`, `index`, `memory` | Orchestrates all business operations |
| `audit` | `store`, `memory` | Scans cards for quality issues |
| `i18n` | - | Translation tables and lookup |
| `mcp` | - | JSON-RPC message types and stdio protocol |
| `cmd/*` | `service`, `audit`, `i18n`, `mcp` | Entrypoints, parse args, wire up deps |

## Key Decisions

### SQLite without CGO

Use `modernc.org/sqlite` (pure Go translation of SQLite) instead of
`mattn/go-sqlite3` (CGO). This eliminates the need for GCC / MSYS2 on Windows
and produces a truly self-contained binary.

### MCP protocol: lightweight stdio JSON-RPC

The MCP protocol for tool servers is just JSON-RPC 2.0 over stdin/stdout.
Rather than importing a heavy Python MCP library equivalent, we implement a
minimal (~200 line) `internal/mcp/protocol.go` that handles:

- JSON-RPC 2.0 request/response/error framing
- Tool registration (name, description, input schema)
- Stdio transport with line-delimited JSON
- Graceful shutdown on stdin EOF

This matches what the existing Python `pmem-mcp` does via the `mcp` PyPI
package, but without external dependency overhead.

### Fyne GUI architecture

The GUI uses Fyne v2 and follows its standard app/window pattern:

```
App -> Window -> Split Layout
                  |- Left Panel (Card List with search)
                  |    |- Toolbar (theme toggle, lang toggle, MCP start/stop)
                  |    |- Search entry
                  |    '- List of memory cards (status icon, type icon, title, date)
                  '- Right Panel (Card Detail)
                       |- Title + metadata bar (ID, type, status, confidence)
                       '- Scrollable sections (summary, content, source, tags, scope, relations)
```

Key UI behaviors:

- Dark/light theme via Fyne's built-in `fyne.Settings` theme switching
- Bilingual labels via `i18n.T()` - bound to a theme-aware label refresh
- Card list sorted by `updated_at` descending, with alternating row backgrounds
- Detail panel scrolls independently from the list
- Right-click context menu on cards: Copy ID, Mark as (active/stale/superseded/rejected)
- System tray icon for minimize-to-tray (Fyne desktop extension)
- MCP server lifecycle managed from toolbar with indicator light
- Status bar showing current operation and item count

## Data Model

### MemoryCard struct

```go
type MemoryCard struct {
    SchemaVersion int                    `yaml:"schema_version"`
    ID            string                 `yaml:"id"`
    Type          string                 `yaml:"type"`
    Status        string                 `yaml:"status"`
    Confidence    float64                `yaml:"confidence"`
    Title         string                 `yaml:"title"`
    Summary       string                 `yaml:"summary"`
    Content       string                 `yaml:"content"`
    Source        SourceInfo             `yaml:"source"`
    Scope         ScopeInfo              `yaml:"scope"`
    Tags          []string               `yaml:"tags"`
    Relations     map[string][]string    `yaml:"relations"`
    CreatedAt     string                 `yaml:"created_at"`
    UpdatedAt     string                 `yaml:"updated_at"`
}
```

Identical schema to the Python version. `yaml` tag matches field naming so
existing YAML cards are read-compatible.

### Validation rules (port from Python `models.py`)

All Python validation rules are ported to Go with equivalent checks:

- Required fields presence
- `schema_version == 1`
- `type` in `MEMORY_TYPES`
- `status` in `MEMORY_STATUSES`
- `confidence` is float64 in [0.0, 1.0]
- ID matches `mem_\\d{8}_\\d{3}`
- Source includes valid `kind` and non-empty `description`
- Relations targets are valid IDs, no self-references, no unknown targets

### Constants

```go
var MemoryTypes = map[string]bool{...}
var MemoryStatuses = map[string]bool{...}
var SourceKinds = map[string]bool{...}
var RelationKinds = []string{...}
```

## Core Service

### MemoryService

Same interface as Python `service.py`:

```go
type MemoryService struct { ... }
func New(projectRoot string) *MemoryService
func (s *MemoryService) InitProject() error
func (s *MemoryService) Remember(payload map[string]any) (map[string]any, error)
func (s *MemoryService) Recall(query string, filters map[string]any, limit int) ([]map[string]any, error)
func (s *MemoryService) OpenMemory(id string) (map[string]any, error)
func (s *MemoryService) ListRecent(limit int) ([]map[string]any, error)
func (s *MemoryService) UpdateMemory(id string, updates map[string]any) (map[string]any, error)
func (s *MemoryService) RebuildIndex() error
```

## SQLite Index

### Schema (identical to Python)

```sql
CREATE TABLE IF NOT EXISTS memories ( ... );
CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5( ... );
CREATE TABLE IF NOT EXISTS relations ( ... );
CREATE TABLE IF NOT EXISTS memory_paths ( ... );
```

Search logic: same FTS5 MATCH query with status and path filtering, ordered by
BM25 rank then updated_at DESC.

## CLI Commands

Port all 8 Python CLI commands with identical flag interfaces:

```
pmem init                    -> Initialize project memory
pmem remember --file <path>  -> Import YAML card
pmem search <query> [--limit N] -> Search memory summaries
pmem open <id>               -> Show full card YAML
pmem recent [--limit N]      -> List recent memories
pmem update <id> --status <s> [--confidence <f>] -> Update card
pmem rebuild-index           -> Rebuild SQLite from YAML
pmem audit                   -> Report quality issues
```

All commands accept `--project-root` (default `.`).

Output format: YAML for human-readable (same as Python), JSON for
machine-readable with `--json` flag.

## MCP Server

Tools exposed via stdio JSON-RPC:

| Tool | Input | Output |
|------|-------|--------|
| `remember` | `{project_root, memory}` | `{id, path, notification}` |
| `recall` | `{project_root, query, filters?, limit?}` | `{results: [...]}` |
| `open_memory` | `{project_root, id}` | Full card dict |
| `update_memory` | `{project_root, id, updates}` | Updated card dict |
| `list_recent` | `{project_root, limit?}` | `{results: [...]}` |

Protocol transport: JSON-RPC 2.0 over stdin/stdout, one JSON object per line,
same as the Python MCP library produces.

## GUI Design (Fyne)

### Window layout

```
+-----------------------------------------------------------------+
| Project Memory Palace  -  my-project                             |
+-----------------------------------------------------------------+
| [dark/light] [zh/EN] [green MCP Running] [Stop] [Tray]          |
| Project Root: [________________________] [Browse]                |
| Search: [____________________________] [Search]                  |
+---------------------+-------------------------------------------+
| green design        | Title: Use YAML as source                  |
| green decision      | ID: mem_...  Type: ...                    |
| yellow decision     | -----------------------------------------  |
| red bugfix          | Summary: ...                              |
|                     | -----------------------------------------  |
|                     | Content: ...                              |
|                     | -----------------------------------------  |
|                     | Source, Tags, Scope, ...                   |
+---------------------+-------------------------------------------+
| Ready  |  12 items                    | v0.2.0                   |
+-----------------------------------------------------------------+
```

### Visual improvements over Python version

1. Split pane - left list / right detail, resizable divider (Fyne HSplitContainer)
2. Status indicator - colored circles (green=active, yellow=stale, gray=superseded, red=rejected)
3. Type icon - Fyne canvas icons or unicode symbols matching Python version
4. Inline search with live filtering (debounced)
5. Theme toggle via Fyne App.Settings().SetTheme()
6. Bilingual refresh all labels instantly on language switch
7. System tray minimize via desktop.App.SetSystemTrayIcon()
8. MCP indicator with green/red dot
9. Smooth scrolling via Fyne scroll containers
10. Modern styling with Fyne native Windows look

### Memory card list

- Each row shows: status icon, type icon, title (truncated), updated date
- Alternating row backgrounds
- Right-click context menu: Copy ID, Mark as
- Single click selects and shows detail
- Sort by updated_at descending

### Memory detail panel

- Title at top (bold, wrap)
- Metadata line: ID, Type, Status, Confidence
- Sections: Summary, Content, Source, Tags, Scope, Relations
- Empty sections shown as "(none)" in italic
- Scrollable independently

### Theming

- Dark theme: dark backgrounds, light text, blue accent
- Light theme: light backgrounds, dark text, blue accent
- Matches Python version color scheme for continuity
- Fyne theme interface implementation

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Invalid YAML schema | Reject with field-specific validation errors |
| Missing source | Cap confidence at 0.5, emit audit warning |
| SQLite corrupt | YAML untouched, return error recommending rebuild-index |
| Unknown memory ID | Return MemoryNotFoundError |
| Duplicate title | Allow write, note in audit report |
| File exists on write | Retry up to 3 times with new ID |
| Index write fails after YAML write | Delete the written YAML card, return error |

## Testing Strategy

Using Go built-in testing with table-driven tests:

| Package | Test focus |
|---------|-----------|
| `memory` | Validation: valid/invalid cards, field edge cases, ID regex |
| `store` | YAML round-trip, directory init, ID generation, atomic write |
| `index` | CRUD operations, FTS5 search, status/path filtering, rebuild |
| `service` | Integration: remember -> search -> open -> update -> recent flow, rollback |
| `audit` | Issue detection: low confidence, missing tags, duplicates, staleness |
| `cmd/pmem` | CLI flag parsing, command dispatch |

Each test uses `t.TempDir()` for a fresh project root, ensuring isolation.

## Migration Path

After building the Go version:

1. Go binaries read the same `.project-memory/cards/*.yaml` format - no migration needed
2. SQLite index can be deleted and rebuilt with `pmem rebuild-index`
3. Python and Go versions can coexist; they share the same YAML card format
4. Users switch by replacing `pmem`, `pmem-mcp`, `pmem-gui` commands in PATH
