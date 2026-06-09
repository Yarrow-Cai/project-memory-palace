# Project Memory Palace Design

## Goal

Build a local personal project memory tool that lets AI clients use an MCP server to remember and recall project knowledge without loading the entire project history into context.

The first version focuses on one project at a time. It stores durable project knowledge such as project goals, design details, design intent, change reasons, bug roots, module workflows, and conventions. The AI writes memories automatically after useful work and tells the user what was written.

## Scope

The first version is:

- Local-first.
- Single-user.
- Python-based.
- MCP-compatible.
- CLI-manageable.
- YAML-backed.
- SQLite-indexed.
- Searchable without embeddings.

The first version does not include:

- Embedding or vector search.
- Web UI.
- Team sync.
- User accounts or permissions.
- Full conversation listening.
- Large-scale automatic source ingestion.
- A default MCP tool for deleting memories.

## Chosen Approach

Use a file-first architecture with a rebuildable SQLite index.

Each project has a `.project-memory/` directory. YAML files are the source of truth. SQLite is only a local navigation and search index. If SQLite is damaged or deleted, it can be rebuilt from YAML cards.

This keeps the memory base transparent, easy to review, easy to back up, and friendly for AI tools to read and update.

## Directory Layout

```text
.project-memory/
  cards/
    2026-06-09_001_decision.yaml
    2026-06-09_002_bugfix.yaml
  index.sqlite3
  config.yaml
  rules/
    agent-rules.yaml
```

`cards/` stores durable YAML memory cards.

`index.sqlite3` stores FTS5 search data, tag indexes, file path indexes, module indexes, relation indexes, and recent activity data.

`config.yaml` stores project-level settings.

`rules/agent-rules.yaml` stores AI client rules for when and how to write memories.

## Architecture

The tool has three main layers:

- Core memory service: validates cards, writes YAML, reads YAML, maintains indexes, searches, updates status and relations.
- MCP server: exposes memory tools to AI clients.
- CLI: gives the user direct control for initializing, searching, opening, auditing, updating, and rebuilding the memory base.

MCP and CLI must call the same core memory service. They must not implement separate business logic.

## Data Flow

1. The AI solves a problem, implements a feature, fixes a bug, creates a design, clarifies a project goal, or discovers a useful module workflow.
2. The AI calls `remember`.
3. The core service validates the input and writes a YAML card under `.project-memory/cards/`.
4. The core service updates SQLite indexes.
5. The MCP server returns a notification payload.
6. The AI tells the user what was written, why it was written, and how it may help later.
7. In later work, the AI calls `recall` with a query and optional file/module context.
8. `recall` returns only a small number of summaries and memory IDs.
9. The AI calls `open_memory` only when it needs the full card.

The default retrieval path must be progressive. It should return relevant summaries first, not full memory contents.

## Memory Card Schema

Each memory is one YAML card.

Example:

```yaml
schema_version: 1
id: mem_20260609_001
type: decision
status: active
confidence: 0.86

title: "Use YAML cards as the source of truth"
summary: "Project memory is stored as structured YAML cards. SQLite is only a rebuildable search index."

content: |
  The user explicitly chose YAML instead of Markdown for real memory storage.
  YAML is easier for AI clients to write, validate, patch, and consume because fields have clear boundaries.

source:
  kind: conversation
  description: "User confirmed the storage format during product design."
  files: []
  commits: []

scope:
  project: "project-memory-palace"
  modules:
    - storage
    - mcp
  paths: []

tags:
  - storage
  - yaml
  - memory-card

relations:
  supersedes: []
  superseded_by: []
  related_to: []
  explains: []
  caused_by: []

created_at: "2026-06-09T17:00:00+08:00"
updated_at: "2026-06-09T17:00:00+08:00"
```

## Required Fields

Every card must include:

- `schema_version`
- `id`
- `type`
- `status`
- `confidence`
- `title`
- `summary`
- `content`
- `source`
- `scope`
- `tags`
- `relations`
- `created_at`
- `updated_at`

`summary` is used for default recall and should be short, usually one to three sentences.

`content` contains the complete reasoning, background, design purpose, or change reason. It is not returned by default recall.

`source` is required. If the source is incomplete or uncertain, `confidence` must not exceed `0.5`.

`scope.paths` links memory to project files. `scope.modules` links memory to logical modules.

## Memory Types

Supported first-version types:

- `project_goal`: project goals, product purpose, and constraints.
- `design`: architecture, module responsibilities, and workflow design.
- `decision`: explicit technical or product decisions.
- `change_reason`: why a change was made.
- `bugfix`: bug symptom, root cause, and fix.
- `module`: module behavior, upstream and downstream relationships, key flows.
- `convention`: naming, coding, structure, or workflow conventions.
- `open_question`: unresolved questions or risks.

## Status Model

Supported statuses:

- `active`: currently valid.
- `stale`: possibly outdated but not confirmed.
- `superseded`: replaced by newer memory.
- `rejected`: rejected by the user or known to be wrong.

Old memories should not be deleted when designs change. New memories should link to old memories through `supersedes`, and old memories should link back through `superseded_by`.

`rejected` memories are kept for audit but excluded from default recall.

## Source Requirements

Each memory must identify where it came from.

Supported source kinds:

- `conversation`
- `file`
- `commit`
- `manual`
- `test`
- `analysis`

Source data should include a short description and, when relevant, file paths, commit hashes, or test names.

Secrets, tokens, account credentials, and private personal data must not be stored by default.

## Confidence Rules

Use confidence to communicate how reliable a memory is:

- `0.85` to `1.0`: explicit user-confirmed fact or decision.
- `0.75` to `0.95`: directly observed from source files, commits, or tests.
- `0.45` to `0.7`: AI inference from available context.
- `0.0` to `0.5`: incomplete source, uncertain interpretation, or unconfirmed assumption.

If a memory has no reliable source, the system must cap confidence at `0.5`.

## MCP Tools

The first version exposes five MCP tools:

- `remember`
- `recall`
- `open_memory`
- `update_memory`
- `list_recent`

### remember

Writes a new memory card.

Input fields:

```yaml
type: decision
title: "..."
summary: "..."
content: "..."
confidence: 0.8
source:
  kind: conversation
  description: "..."
  files: []
  commits: []
scope:
  project: "..."
  modules: []
  paths: []
tags: []
relations:
  supersedes: []
  related_to: []
  explains: []
  caused_by: []
```

Behavior:

- Validate schema fields.
- Generate `id`, `created_at`, and `updated_at`.
- Write a YAML card.
- Update SQLite indexes.
- Return the created ID and a notification payload.

The AI must tell the user what was written after a successful `remember`.

### recall

Retrieves relevant memory summaries.

Input:

```yaml
query: "why was this module designed this way"
filters:
  type: []
  tags: []
  paths: []
  status:
    - active
limit: 5
```

Default output:

```yaml
results:
  - id: mem_20260609_001
    type: decision
    status: active
    title: "..."
    summary: "..."
    confidence: 0.86
    source_hint: "conversation"
    matched_by:
      - title
      - tags
      - path
    updated_at: "2026-06-09T17:00:00+08:00"
```

`recall` must not return full `content` by default.

### open_memory

Opens one full memory card by ID.

Input:

```yaml
id: mem_20260609_001
```

Output is the full YAML card.

### update_memory

Updates status, relations, tags, or confidence.

Input example:

```yaml
id: mem_20260609_001
status: superseded
relations:
  superseded_by:
    - mem_20260609_008
reason: "A newer design replaced this decision."
```

This tool must not silently delete memories.

### list_recent

Lists recently created or updated memories.

Input:

```yaml
limit: 10
```

Output should include ID, type, status, title, summary, confidence, and updated time.

## CLI

The CLI command is `pmem`.

First-version commands:

```bash
pmem init
pmem remember --file card.yaml
pmem search "keywords or question"
pmem open mem_20260609_001
pmem recent --limit 10
pmem update mem_20260609_001 --status stale
pmem rebuild-index
pmem audit
```

Command responsibilities:

- `init`: create `.project-memory/`, default config, default rules, and SQLite schema.
- `remember --file`: import a YAML card from a file.
- `search`: run the same retrieval logic as MCP `recall`, with more human-readable output.
- `open`: print one full YAML card.
- `recent`: list recent memory changes.
- `update`: manually change status, tags, relations, or confidence.
- `rebuild-index`: rebuild SQLite from YAML cards.
- `audit`: report low-quality or suspicious memories.

## Search Strategy

The first version does not use embeddings.

Search uses symbolic hybrid retrieval:

1. SQLite FTS5 indexes `title`, `summary`, `content`, `tags`, `modules`, and `paths`.
2. Title, tags, paths, and modules are weighted higher than raw content.
3. `active` cards are prioritized.
4. `stale` cards are down-ranked.
5. `superseded` cards appear only when useful for historical explanation or relation expansion.
6. `rejected` cards are excluded by default.
7. If the caller provides current file paths or module names, matching memory cards are boosted.
8. After direct matches, the retriever may add a small number of relation cards through `explains`, `caused_by`, or `supersedes`.
9. Default result limit is five summaries.

The retrieval system should prefer a smaller set of high-signal summaries over a large dump of loosely related memories.

## AI Auto-Memory Rules

The AI should call `remember` after:

- Solving a problem.
- Implementing a feature.
- Fixing a bug.
- Producing a design.
- Clarifying a project goal.
- Discovering a module workflow.
- Changing a previous decision.
- Establishing a convention.

The AI should not remember:

- Temporary command output.
- Debugging noise with no long-term value.
- Unconfirmed guesses.
- Duplicate summaries with no new information.
- Secrets, tokens, accounts, or private personal data.

Before writing memory, the AI should check:

- Will this help future understanding of project purpose, design reason, change reason, or module behavior?
- Will it reduce repeated analysis later?
- Does it have a clear source?
- Does it duplicate existing memory?
- Should it supersede an older memory?

## User Notification

After a successful `remember`, the AI must notify the user in this shape:

```text
Project memory written:
- ID: mem_...
- Type: decision / bugfix / ...
- Summary: ...
- Source: ...
- Future use: ...
```

If a new memory supersedes an old one, the notification must mention the old memory ID and the relation change.

## Error Handling

YAML schema invalid:

- Reject the write.
- Return field-specific validation errors.

Missing source:

- Allow only low-confidence memory.
- Cap confidence at `0.5`.
- Add an audit warning.

SQLite index damaged:

- Keep YAML cards untouched.
- Return an error that recommends `pmem rebuild-index`.

Unknown memory ID:

- Return a readable not-found error.

Possible duplicate:

- Warn the caller.
- Allow the write in the first version.

Secret-like content:

- Reject by default.
- Require explicit user intent before storing sensitive data.

## Testing Requirements

The first implementation should test:

- YAML card schema validation.
- `remember` writes YAML and updates SQLite.
- `recall` returns summaries without `content`.
- `open_memory` returns a full card by ID.
- `update_memory` updates status and relations.
- `list_recent` returns recent changes.
- `rebuild-index` restores SQLite from YAML cards.
- `audit` reports low confidence, missing source detail, missing tags, missing scope, likely duplicates, and stale memories.
- MCP and CLI both use the same core memory service.

## Success Criteria

The first version is successful when:

- A project can run `pmem init` to create a local memory base.
- An AI client can connect to the MCP server.
- The AI can write a YAML memory after useful work.
- The user is notified after automatic memory writing.
- The AI can recall a small set of relevant summaries.
- The AI can open full memory only when needed.
- The user can inspect, search, update, audit, and rebuild the memory base from the CLI.
- No embedding environment is required.
