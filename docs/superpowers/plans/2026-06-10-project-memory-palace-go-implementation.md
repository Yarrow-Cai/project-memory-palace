# Project Memory Palace Go Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the Python Project Memory Palace (YAML-backed project memory with SQLite search, CLI, MCP server, and desktop GUI) as a self-contained Go binary with a modern Fyne GUI.

**Architecture:** Standard Go project layout with `cmd/` for entrypoints and `internal/` for library packages. Build bottom-up: data model -> file store -> SQLite index -> service -> CLI -> MCP -> GUI. Each package has TDD tests. No CGO (pure Go SQLite via modernc.org/sqlite).

**Tech Stack:** Go 1.22+, fyne.io/fyne/v2 (GUI), modernc.org/sqlite (pure Go SQLite), gopkg.in/yaml.v3 (YAML), Go stdlib for MCP, CLI, i18n.

---

## File Structure

```
project-memory-palace/
|-- go.mod
|-- Makefile
|-- internal/
|   |-- memory/
|   |   |-- constants.go         # MemoryTypes, MemoryStatuses, SourceKinds, RelationKinds
|   |   |-- card.go              # MemoryCard struct, SourceInfo, ScopeInfo, NewCard, ToSummary
|   |   |-- validate.go          # ValidateCard, ValidatePayload
|   |   |-- memory_test.go       # Tests for validation
|   |-- store/
|   |   |-- paths.go             # MemoryDir, CardsDir, RulesDir, ConfigPath, IndexPath
|   |   |-- layout.go            # EnsureProjectMemory, AssertMemoryLayout, RemoveCard
|   |   |-- identity.go          # NextCardIdentity
|   |   |-- card_io.go           # ReadCard, WriteCard, DiscoverCards, CardFilename
|   |   |-- store_test.go        # Tests for I/O, layout, identity
|   |-- index/
|   |   |-- index.go             # MemoryIndex: Connect, Initialize, Search, Recent, Upsert, Rebuild, Clear
|   |   |-- index_test.go        # Tests for SQLite operations
|   |-- service/
|   |   |-- errors.go            # MemoryNotFoundError
|   |   |-- service.go           # MemoryService: Init, Remember, Recall, Open, ListRecent, Update, Rebuild
|   |   |-- service_test.go      # Integration tests
|   |-- audit/
|   |   |-- audit.go             # AuditProject: scan cards, report issues
|   |   |-- audit_test.go        # Tests for issue detection
|   |-- i18n/
|   |   |-- i18n.go              # T(), SetLanguage(), GetLanguage(), translation tables
|   |   |-- i18n_test.go         # Tests for translation
|   |-- mcp/
|       |-- protocol.go          # JSON-RPC types, Tool registry, StdioServer
|       |-- protocol_test.go     # Tests for JSON-RPC
|-- cmd/
|   |-- pmem/main.go              # CLI: 8 commands
|   |-- pmem-mcp/main.go          # MCP server
|   |-- pmem-gui/main.go          # Fyne desktop GUI
```

Design doc: `docs/superpowers/specs/2026-06-10-project-memory-palace-go-design.md`

---

### Task 1: Project scaffolding and go.mod

**Files:**
- Create: `project-memory-palace/go.mod`
- Create: `project-memory-palace/Makefile`
- Create: `project-memory-palace/internal/memory/constants.go`

- [ ] **Step 1: Create go.mod and directories**

```bash
mkdir -p project-memory-palace/internal/{memory,store,index,service,audit,i18n,mcp}
mkdir -p project-memory-palace/cmd/{pmem,pmem-mcp,pmem-gui}
cd project-memory-palace && go mod init github.com/atop/project-memory-palace
```

- [ ] **Step 2: Add dependencies**

```bash
cd project-memory-palace
go get gopkg.in/yaml.v3@latest
go get modernc.org/sqlite@latest
go get fyne.io/fyne/v2@latest
```

- [ ] **Step 3: Create Makefile**

Write `project-memory-palace/Makefile`:

```makefile
.PHONY: all build build-cli build-mcp build-gui test clean

all: build

build: build-cli build-mcp build-gui

build-cli:
	go build -o bin/pmem.exe ./cmd/pmem

build-mcp:
	go build -o bin/pmem-mcp.exe ./cmd/pmem-mcp

build-gui:
	go build -o bin/pmem-gui.exe ./cmd/pmem-gui

test:
	go test ./internal/... -v -count=1

clean:
	rm -rf bin/
```

- [ ] **Step 4: Create constants.go**

Write `internal/memory/constants.go`:

```go
package memory

var MemoryTypes = map[string]bool{
    "project_goal": true, "design": true, "decision": true,
    "change_reason": true, "bugfix": true, "module": true,
    "convention": true, "open_question": true,
}

var MemoryStatuses = map[string]bool{
    "active": true, "stale": true, "superseded": true, "rejected": true,
}

var SourceKinds = map[string]bool{
    "conversation": true, "file": true, "commit": true,
    "manual": true, "test": true, "analysis": true,
}

var RelationKinds = []string{
    "supersedes", "superseded_by", "related_to", "explains", "caused_by",
}

var RequiredFields = []string{
    "schema_version", "id", "type", "status", "confidence",
    "title", "summary", "content", "source", "scope",
    "tags", "relations", "created_at", "updated_at",
}

const DefaultConfidence = 0.5
const SchemaVersion = 1
const IDPattern = `^mem_\d{8}_\d{3}$`

var RememberRequiredFields = []string{"content", "summary", "title", "type"}

var UpdateAllowedFields = map[string]bool{
    "confidence": true, "reason": true, "relations": true,
    "status": true, "tags": true,
}
```

- [ ] **Step 5: Verify build compiles**

```bash
cd project-memory-palace && go build ./internal/memory && go vet ./internal/memory
```

- [ ] **Step 6: Commit**

```bash
cd project-memory-palace
git add go.mod go.sum Makefile internal/memory/constants.go
git commit -m "feat: project scaffolding with constants"
```

---

### Task 2: MemoryCard data model and validation

**Files:**
- Create: `internal/memory/card.go`
- Create: `internal/memory/validate.go`
- Create: `internal/memory/memory_test.go`

**Steps (TDD):**

- [ ] **Step 1: Write failing test**

Write `internal/memory/memory_test.go` with these tests:
- `TestValidCard` — a fully valid card passes validation
- `TestInvalidSchemaVersion` — schema_version != 1 fails
- `TestInvalidType` — unknown type fails
- `TestInvalidStatus` — unknown status fails
- `TestConfidenceOutOfRange` — confidence > 1.0 fails
- `TestConfidenceNegative` — confidence < 0.0 fails
- `TestInvalidID` — bad ID format fails
- `TestMissingRequiredFields` — empty title/summary/content fails
- `TestInvalidSourceKind` — unknown source.kind fails
- `TestEmptySourceDescription` — empty source.description fails
- `TestInvalidRelationTarget` — bad relation target ID fails
- `TestSelfRelation` — self-referencing relation fails
- `TestNewCardHasRequiredFields` — NewCard produces valid card
- `TestToSummary` — summary does not include content/source/scope

All tests use `validCard()` helper that returns a fully populated valid card.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd project-memory-palace && go test ./internal/memory -v
```
Expected: Compilation errors (types not defined).

- [ ] **Step 3: Write card.go**

```go
package memory

import (
    "fmt"
    "regexp"
    "time"
)

type SourceInfo struct {
    Kind        string   `yaml:"kind" json:"kind"`
    Description string   `yaml:"description" json:"description"`
    Files       []string `yaml:"files" json:"files"`
    Commits     []string `yaml:"commits" json:"commits"`
}

type ScopeInfo struct {
    Project string   `yaml:"project" json:"project"`
    Modules []string `yaml:"modules" json:"modules"`
    Paths   []string `yaml:"paths" json:"paths"`
}

type MemoryCard struct {
    SchemaVersion int                 `yaml:"schema_version" json:"schema_version"`
    ID            string              `yaml:"id" json:"id"`
    Type          string              `yaml:"type" json:"type"`
    Status        string              `yaml:"status" json:"status"`
    Confidence    float64             `yaml:"confidence" json:"confidence"`
    Title         string              `yaml:"title" json:"title"`
    Summary       string              `yaml:"summary" json:"summary"`
    Content       string              `yaml:"content" json:"content"`
    Source        SourceInfo          `yaml:"source" json:"source"`
    Scope         ScopeInfo           `yaml:"scope" json:"scope"`
    Tags          []string            `yaml:"tags" json:"tags"`
    Relations     map[string][]string `yaml:"relations" json:"relations"`
    CreatedAt     string              `yaml:"created_at" json:"created_at"`
    UpdatedAt     string              `yaml:"updated_at" json:"updated_at"`
}

func NewCard(cardType, title, summary, content string, confidence float64) MemoryCard {
    return MemoryCard{
        SchemaVersion: SchemaVersion,
        ID:            fmt.Sprintf("mem_%s_001", time.Now().Format("20060102")),
        Type:          cardType,
        Status:        "active",
        Confidence:    confidence,
        Title:         title,
        Summary:       summary,
        Content:       content,
        Source:        SourceInfo{Kind: "analysis", Description: "Source was not supplied by caller."},
        Tags:          []string{},
        Relations:     map[string][]string{},
        CreatedAt:     time.Now().Format(time.RFC3339),
        UpdatedAt:     time.Now().Format(time.RFC3339),
    }
}

func (c *MemoryCard) ToSummary() map[string]any {
    return map[string]any{
        "id": c.ID, "type": c.Type, "status": c.Status,
        "title": c.Title, "summary": c.Summary,
        "confidence": c.Confidence, "source_hint": c.Source.Kind,
        "matched_by": []string{}, "updated_at": c.UpdatedAt,
    }
}

var idRe = regexp.MustCompile(`^mem_\d{8}_\d{3}$`)
```

- [ ] **Step 4: Write validate.go**

```go
package memory

import (
    "fmt"
    "regexp"
    "strings"
)

var idTargetRe = regexp.MustCompile(`^mem_\d{8}_\d{3}$`)

type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func ValidateCard(card *MemoryCard) error {
    if card.SchemaVersion != SchemaVersion {
        return &ValidationError{"schema_version", fmt.Sprintf("must be %d", SchemaVersion)}
    }
    if !MemoryTypes[card.Type] {
        return &ValidationError{"type", "invalid type"}
    }
    if !MemoryStatuses[card.Status] {
        return &ValidationError{"status", "invalid status"}
    }
    if card.Confidence < 0.0 || card.Confidence > 1.0 {
        return &ValidationError{"confidence", "must be between 0.0 and 1.0"}
    }
    if !idRe.MatchString(card.ID) {
        return &ValidationError{"id", "must match mem_YYYYMMDD_NNN"}
    }
    for _, f := range []struct{n, v string}{{"title",card.Title},{"summary",card.Summary},{"content",card.Content},{"created_at",card.CreatedAt},{"updated_at",card.UpdatedAt}} {
        if strings.TrimSpace(f.v) == "" {
            return &ValidationError{f.n, "must be non-empty"}
        }
    }
    if !SourceKinds[card.Source.Kind] {
        return &ValidationError{"source.kind", "invalid kind"}
    }
    if strings.TrimSpace(card.Source.Description) == "" {
        return &ValidationError{"source.description", "must be non-empty"}
    }
    for rel, targets := range card.Relations {
        found := false
        for _, r := range RelationKinds { if r == rel { found = true; break } }
        if !found { return &ValidationError{"relations", "unknown relation: " + rel} }
        for _, t := range targets {
            if !idTargetRe.MatchString(t) { return &ValidationError{"relations", "invalid target: " + t} }
            if t == card.ID { return &ValidationError{"relations", "self-reference: " + t} }
        }
    }
    return nil
}

func ValidatePayload(payload map[string]any) error {
    for _, f := range RememberRequiredFields {
        if _, ok := payload[f]; !ok { return &ValidationError{f, "is required"} }
    }
    return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
cd project-memory-palace && go test ./internal/memory -v
```
Expected: All 14 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/card.go internal/memory/validate.go internal/memory/memory_test.go
git commit -m "feat: memory card model with validation"
```

---

### Task 3: i18n (bilingual support)

**Files:**
- Create: `internal/i18n/i18n.go`
- Create: `internal/i18n/i18n_test.go`

- [ ] **Step 1: Write failing test**

Write `internal/i18n/i18n_test.go` with tests:
- `TestEnglishTranslation` — "app_title" returns "Project Memory Palace"
- `TestChineseTranslation` — non-English result for "app_title" after SetLanguage("zh")
- `TestFallbackToEnglish` — unknown key returns key itself
- `TestSetUnknownLanguage` — "fr" is ignored, remains "en"
- `TestDefaultLanguage` — GetLanguage() returns "en" initially

- [ ] **Step 2: Run test to verify it fails**

```bash
cd project-memory-palace && go test ./internal/i18n -v
```
Expected: Compilation errors.

- [ ] **Step 3: Write i18n.go**

Package `i18n` with:
- `var translations map[string]map[string]string` — en + zh keys
- `var currentLang = "en"` with `sync.RWMutex`
- `T(key string) string` — lookup with en fallback
- `SetLanguage(lang string)` — only accepts "en" or "zh"
- `GetLanguage() string`

Translate ~50 UI strings: app_title, browse, search, search_placeholder, recent, search_results, mcp_running, mcp_stopped, mcp_start, mcp_stop, status, type, title, updated, select_memory, summary, content, source, tags, scope, relations, confidence, id_label, empty, none, loading, ready, error, items, memories, results_for, copied, updated_to, copy_id, mark_as, show, rebuilding, rebuilt, searching, project_label, detail_summary, detail_content, detail_source, detail_tags, detail_scope, detail_relations, mcp_start_short, mcp_stop_short, mcp_na.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd project-memory-palace && go test ./internal/i18n -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/i18n/i18n.go internal/i18n/i18n_test.go
git commit -m "feat: bilingual i18n support (en/zh)"
```

---

### Task 4: MCP JSON-RPC protocol

**Files:**
- Create: `internal/mcp/protocol.go`
- Create: `internal/mcp/protocol_test.go`

- [ ] **Step 1: Write failing test**

Write `internal/mcp/protocol_test.go` with tests:
- `TestParseRequest` — parses valid JSON-RPC 2.0 request
- `TestParseRequestInvalidJSON` — returns error for bad JSON
- `TestNewResponse` — creates success response with ID and result
- `TestNewErrorResponse` — creates error response with code/message
- `TestToolRegistry` — register + list tools
- `TestToolDispatch` — dispatch to registered handler
- `TestToolDispatchUnknown` — error for unknown tool
- `TestStdioServerLifecycle` — ping -> pong via Stdin/Stdout

- [ ] **Step 2: Run test to verify it fails**

```bash
cd project-memory-palace && go test ./internal/mcp -v
```
Expected: Compilation errors.

- [ ] **Step 3: Write protocol.go**

Package `mcp` with:
- `Request` struct: JSONRPC, ID (json.Number), Method, Params
- `Response` struct: JSONRPC, ID, Result, Error (*ResponseError)
- `ResponseError` struct: Code, Message
- `ParseRequest(raw []byte) (*Request, error)`
- `NewResponse(id json.Number, result any) Response`
- `NewErrorResponse(id json.Number, code int, message string) Response`
- `ToolHandler func(params map[string]any) (any, error)`
- `ToolDef` struct: Name, Description, Schema, Handler
- `ToolRegistry` with Register/List/Dispatch (thread-safe with RWMutex)
- `StdioServer` with Reader, Writer, Registry, HandleOne() method

- [ ] **Step 4: Run test to verify it passes**

```bash
cd project-memory-palace && go test ./internal/mcp -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/protocol.go internal/mcp/protocol_test.go
git commit -m "feat: MCP JSON-RPC protocol with tool registry"
```

---

### Task 5: Store — paths, layout, identity, card I/O

**Files:**
- Create: `internal/store/paths.go`
- Create: `internal/store/layout.go`
- Create: `internal/store/identity.go`
- Create: `internal/store/card_io.go`
- Create: `internal/store/store_test.go`

- [ ] **Step 1: Write failing test**

Write `internal/store/store_test.go` with tests:
- `TestPaths` — MemoryDir, CardsDir, IndexPath have correct suffixes
- `TestEnsureProjectMemory` — creates cards dir, config.yaml, rules dir
- `TestAssertMemoryLayout` — passes after ensure, fails after removing config
- `TestNextCardIdentity` — generates mem_YYYYMMDD_001, increments to _002
- `TestWriteAndReadCard` — round-trip write then read preserves fields
- `TestWriteCardOverwrite` — without overwrite flag fails on duplicate
- `TestDiscoverCards` — finds all 3 written cards
- `TestCardFilename` — produces "2026-06-09_001_decision.yaml"
- `TestCardFilenameInvalidID` — panics for malformed ID

- [ ] **Step 2: Run test to verify it fails**

```bash
cd project-memory-palace && go test ./internal/store -v
```
Expected: Compilation errors.

- [ ] **Step 3: Write paths.go**

Package `store` with:
- `MemoryDir(projectRoot) string` — returns `.project-memory` path
- `CardsDir(projectRoot) string` — returns `.project-memory/cards`
- `RulesDir(projectRoot) string` — returns `.project-memory/rules`
- `ConfigPath(projectRoot) string` — returns `.project-memory/config.yaml`
- `RulesPath(projectRoot) string` — returns `.project-memory/rules/agent-rules.yaml`
- `IndexPath(projectRoot) string` — returns `.project-memory/index.sqlite3`

- [ ] **Step 4: Write layout.go**

With `defaultConfig` and `defaultAgentRules` constants (same as Python version), plus:
- `EnsureProjectMemory(projectRoot string) error`
- `AssertMemoryLayout(projectRoot string) error`
- `RemoveCard(path string) error`
- `EnsureCardsDir(projectRoot string) error`

- [ ] **Step 5: Write identity.go**

With `idRe = regexp.MustCompile(...)` and:
- `NextCardIdentity(projectRoot, dateStr string) (string, int, error)`
  Scans existing cards via DiscoverCards, finds max sequence for date, returns next id.

- [ ] **Step 6: Write card_io.go**

With `filenameIDRe` and `cardIDRe` regexps and:
- `CardFilename(card *memory.MemoryCard) string`
- `WriteCard(projectRoot string, card *memory.MemoryCard, overwrite bool) (string, error)` — atomic write (temp file + rename)
- `ReadCard(path string) (*memory.MemoryCard, error)` — unmarshal + validate
- `DiscoverCards(projectRoot string) ([]*memory.MemoryCard, error)` — glob *.yaml, validate filename, check duplicates, sort by ID

- [ ] **Step 7: Run test to verify it passes**

```bash
cd project-memory-palace && go test ./internal/store -v
```

- [ ] **Step 8: Commit**

```bash
git add internal/store/paths.go internal/store/layout.go internal/store/identity.go internal/store/card_io.go internal/store/store_test.go
git commit -m "feat: store package with YAML card I/O"
```

---

### Task 6: SQLite FTS5 index

**Files:**
- Create: `internal/index/index.go`
- Create: `internal/index/index_test.go`

- [ ] **Step 1: Write failing test**

Write `internal/index/index_test.go` with tests:
- `TestInitialize` — creates tables
- `TestUpsertAndSearch` — write card, search, find by FTS
- `TestSearchByStatus` — default excludes rejected, explicit filter works
- `TestRecent` — returns N most recent cards
- `TestRebuild` — discovers YAML cards and rebuilds index
- `TestUpsertRemovesOldRelations` — re-upsert with empty relations clears old data
- `TestClear` — removes all data
- `TestConnect` — opens and closes SQLite connection

- [ ] **Step 2: Run test to verify it fails**

```bash
cd project-memory-palace && go test ./internal/index -v
```
Expected: Compilation errors.

- [ ] **Step 3: Write index.go**

Package `index` with:
- `schemaDDL` constant (CREATE TABLE statements matching Python schema)
- `MemoryIndex` struct with projectRoot and dbPath
- `NewMemoryIndex(projectRoot string) *MemoryIndex`
- `Connect() (*sql.DB, error)` — opens `modernc.org/sqlite` connection
- `Initialize() error` — executes schema DDL
- `Clear() error` — DELETE from all tables in a transaction
- `Upsert(card *memory.MemoryCard) error` — upsert memories + FTS + paths + relations
- `Search(query string, filters map[string]any, limit int) ([]map[string]any, error)` — FTS5 MATCH with status/path filtering, BM25 ranking
- `Recent(limit int) ([]map[string]any, error)` — SELECT from memories ORDER BY updated_at DESC
- `Rebuild() error` — DiscoverCards + Clear + upsert all in transaction
- Helper functions: `toFTSQuery`, `normalizeStatusFilter`, `normalizePathFilter`

- [ ] **Step 4: Run test to verify it passes**

```bash
cd project-memory-palace && go test ./internal/index -v -count=1
```
Note: First run may download modernc.org/sqlite dependencies.

- [ ] **Step 5: Commit**

```bash
git add internal/index/index.go internal/index/index_test.go
git commit -m "feat: SQLite FTS5 search index"
```

---

### Task 7: Service — business logic layer

**Files:**
- Create: `internal/service/errors.go`
- Create: `internal/service/service.go`
- Create: `internal/service/service_test.go`

- [ ] **Step 1: Write failing test**

Write `internal/service/service_test.go` with tests:
- `TestServiceInitProject` — InitProject creates layout
- `TestRememberAndRecall` — remember then search finds it
- `TestOpenMemory` — open by ID returns full card including content
- `TestOpenMemoryNotFound` — unknown ID returns MemoryNotFoundError
- `TestListRecent` — returns written cards
- `TestUpdateMemory` — change status, verify in index
- `TestRebuildIndex` — rebuild preserves searchable cards
- `TestRememberMissingFields` — rejects payload without required fields
- `TestUpdateNoChanges` — rejects empty updates map
- `TestRememberWithSource` — accepts explicit source field

- [ ] **Step 2: Run test to verify it fails**

```bash
cd project-memory-palace && go test ./internal/service -v
```
Expected: Compilation errors.

- [ ] **Step 3: Write errors.go**

```go
package service

type MemoryNotFoundError struct {
    ID string
}

func (e *MemoryNotFoundError) Error() string {
    return "memory not found: " + e.ID
}
```

- [ ] **Step 4: Write service.go**

Package `service` with:
- `MemoryService` struct with projectRoot + *index.MemoryIndex
- `New(projectRoot string) *MemoryService`
- `InitProject() error` — ensure layout + init index
- `Remember(payload) (map[string]any, error)` — validate payload, generate ID (3 attempts), write YAML, index, return {id, path, notification}
- `Recall(query, filters, limit) ([]map[string]any, error)` — search index
- `OpenMemory(id) (map[string]any, error)` — scan YAML files for matching ID
- `ListRecent(limit) ([]map[string]any, error)` — recent from index
- `UpdateMemory(id, updates) (map[string]any, error)` — validate updates, read existing, apply, write YAML, update index
- `RebuildIndex() error` — delegate to index.Rebuild()
- `ProjectRoot() string` — accessor for CLI

Helper functions: `buildCard`, `cardToMap`, `mapToCard`, `buildNotification`, `toFloat64`, `toStringList`, `anyToStringSlice`

Error rollback: if index.Upsert fails after YAML write, delete the written card file.

- [ ] **Step 5: Run test to verify it passes**

```bash
cd project-memory-palace && go test ./internal/service -v -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/service/errors.go internal/service/service.go internal/service/service_test.go
git commit -m "feat: core business logic service"
```

---

### Task 8: Audit package

**Files:**
- Create: `internal/audit/audit.go`
- Create: `internal/audit/audit_test.go`

- [ ] **Step 1: Write failing test**

Write `internal/audit/audit_test.go` with tests:
- `TestEmptyProjectHasNoIssues` — fresh project returns empty report
- `TestLowConfidenceReported` — card with 0.3 confidence triggers "low_confidence"
- `TestDuplicateTitleReported` — two cards with same title trigger "possible_duplicate"
- `TestStaleStatusReported` — card with "stale" status triggers "stale" issue

- [ ] **Step 2: Run test to verify it fails**

```bash
cd project-memory-palace && go test ./internal/audit -v
```

- [ ] **Step 3: Write audit.go**

Package `audit` with:
- `AuditProject(projectRoot string) ([]map[string]any, error)` — scans all cards via store.DiscoverCards, checks:
  - confidence <= 0.5 → "low_confidence"
  - len(tags) == 0 → "missing_tags"
  - no modules and no paths → "missing_scope"
  - source.kind == "analysis" && confidence > 0.7 → "high_confidence_inference"
  - duplicate title (case-insensitive) → "possible_duplicate:{id}"
  - status == "stale" → "stale"
  - Returns list of {id, title, status, issues} for cards with at least one issue.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd project-memory-palace && go test ./internal/audit -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/audit/audit.go internal/audit/audit_test.go
git commit -m "feat: audit package for memory quality checks"
```

---

### Task 9: CLI (pmem)

**Files:**
- Create: `cmd/pmem/main.go`

- [ ] **Step 1: Write CLI entrypoint**

Write `cmd/pmem/main.go` with:
- `main()` — calls `run()` and `os.Exit`
- `run(args []string) int` — parses `--project-root` flag and command
- Support all 8 commands:
  - `init` → svc.InitProject()
  - `remember --file <path>` → read YAML file, svc.Remember(payload)
  - `search <query> [--limit N]` → svc.Recall(), print YAML output
  - `open <id>` → svc.OpenMemory(), print YAML
  - `recent [--limit N]` → svc.ListRecent(), print YAML
  - `update <id> --status <s> [--confidence <f>]` → svc.UpdateMemory()
  - `rebuild-index` → svc.RebuildIndex()
  - `audit` → audit.AuditProject(), print JSON
- Output: YAML for human-readable (same as Python); validate project root with store.AssertMemoryLayout where appropriate

- [ ] **Step 2: Build**

```bash
cd project-memory-palace && go build -o bin/pmem.exe ./cmd/pmem
```
Expected: Binary created.

- [ ] **Step 3: Commit**

```bash
git add cmd/pmem/main.go
git commit -m "feat: CLI (pmem) with all 8 commands"
```

---

### Task 10: MCP server (pmem-mcp)

**Files:**
- Create: `cmd/pmem-mcp/main.go`

- [ ] **Step 1: Write MCP server entrypoint**

Write `cmd/pmem-mcp/main.go` with:
- `main()` — reads projectRoot from CLI arg (default "."), creates service.New()
- Registers 5 tools on mcp.NewToolRegistry():
  - `remember` — takes {project_root, memory}, returns {id, path, notification}
  - `recall` — takes {project_root, query, filters?, limit?}, returns {results}
  - `open_memory` — takes {project_root, id}, returns full card
  - `update_memory` — takes {project_root, id, updates}, returns updated card
  - `list_recent` — takes {project_root, limit?}, returns {results}
- Handles stdio JSON-RPC loop: parse request, dispatch tool, write response
- Handles MCP `initialize` method with protocolVersion and capabilities
- Helper functions: `getProjectRoot`, `getString`, `getInt`

- [ ] **Step 2: Build**

```bash
cd project-memory-palace && go build -o bin/pmem-mcp.exe ./cmd/pmem-mcp
```

- [ ] **Step 3: Commit**

```bash
git add cmd/pmem-mcp/main.go
git commit -m "feat: MCP server (pmem-mcp) with 5 tools"
```

---

### Task 11: Fyne GUI (pmem-gui)

**Files:**
- Create: `cmd/pmem-gui/main.go`

This is the largest file (~400 lines). It implements the complete Fyne desktop GUI with:
- Dark/light theme toggle (via custom theme structs wrapping fyne built-in themes)
- Bilingual en/zh support (via i18n.T())
- Project root directory entry + browse button
- Search bar with results
- Memory card list (left panel, shows [date] title)
- Memory detail view (right panel, shows summary, content, source, tags, scope, relations with separators)
- System tray minimize (Fyne desktop extension — set master window)
- MCP server start/stop toggle (indicators + button)
- Status bar showing current state and item count
- Auto-refresh every 30 seconds

- [ ] **Step 1: Write GUI entrypoint**

Write `cmd/pmem-gui/main.go`:

Key structure:
```go
package main

import (
    // fyne, service, i18n, store, exec, os, fmt, strings, sync, time
)

var currentLang = "en"

func main() {
    // Create fyne app + window
    // Default project root from os.Getwd()
    // Create widgets: projectEntry, browseBtn, searchEntry, searchBtn
    // Create card list, detail text area
    // Create toolbar with langBtn, themeBtn, mcpIndicator, mcpBtn
    // Layout: HSplit(left panel with list, right panel with detail) + status bar
    
    // refreshList(): calls svc.ListRecent(50), updates list widget
    // doSearch(): calls svc.Recall(), populates list with results
    // renderDetail(): formats full card data into detail text
    // refreshUI(): re-translates all visible labels
    
    // Dark/light theme structs implementing fyne.Theme
    
    w.ShowAndRun()
}
```

- [ ] **Step 2: Build**

```bash
cd project-memory-palace && go build -o bin/pmem-gui.exe ./cmd/pmem-gui
```
If Fyne has platform dependency issues on Windows, install:
```bash
go install fyne.io/fyne/v2/cmd/fyne@latest
```

- [ ] **Step 3: Commit**

```bash
git add cmd/pmem-gui/main.go
git commit -m "feat: Fyne desktop GUI with dark/light theme and bilingual support"
```

---

### Task 12: Build all and smoke test

**Files:** (existing files only)

- [ ] **Step 1: Build everything**

```bash
cd project-memory-palace && make build
```
Expected: Three binaries under `bin/`: `pmem.exe`, `pmem-mcp.exe`, `pmem-gui.exe`.

- [ ] **Step 2: Run all tests**

```bash
cd project-memory-palace && go test ./internal/... -v -count=1
```
Expected: All tests pass across all 6 internal packages.

- [ ] **Step 3: CLI smoke test**

```bash
cd project-memory-palace
bin\pmem.exe --project-root %TEMP%\pmem-test init
# Create a YAML card
echo "type: decision" > %TEMP%\card.yaml
echo "title: Go Smoke Test" >> %TEMP%\card.yaml
echo "summary: Testing Go version" >> %TEMP%\card.yaml
echo "content: Everything works." >> %TEMP%\card.yaml
echo "confidence: 0.95" >> %TEMP%\card.yaml
bin\pmem.exe --project-root %TEMP%\pmem-test remember --file %TEMP%\card.yaml
bin\pmem.exe --project-root %TEMP%\pmem-test search "smoke"
bin\pmem.exe --project-root %TEMP%\pmem-test recent
bin\pmem.exe --project-root %TEMP%\pmem-test audit
```

- [ ] **Step 4: Verify backward compatibility**

Copy an existing Python `.yaml` card from a real project into `%TEMP%\pmem-test\cards\`, then run:
```bash
bin\pmem.exe --project-root %TEMP%\pmem-test rebuild-index
bin\pmem.exe --project-root %TEMP%\pmem-test search "any-content-from-python-card"
```

- [ ] **Step 5: Final commit**

```bash
cd project-memory-palace
git add -A
git commit -m "chore: build, test, and smoke test all commands"
```

---
