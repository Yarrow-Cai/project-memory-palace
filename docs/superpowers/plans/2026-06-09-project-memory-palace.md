# Project Memory Palace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local Python project memory tool with YAML memory cards, a rebuildable SQLite FTS5 index, a CLI, and an MCP server for progressive AI recall.

**Architecture:** YAML files under `.project-memory/cards/` are the source of truth. A shared core service validates, stores, indexes, searches, updates, audits, and rebuilds memories. The CLI and MCP server are thin adapters over that shared service.

**Tech Stack:** Python 3.11+, stdlib `argparse`, `dataclasses`, `sqlite3`, `pathlib`, `json`, `datetime`; PyYAML for YAML; pytest for tests; official MCP Python SDK `FastMCP` for the MCP server.

---

## File Structure

Create this package layout:

```text
pyproject.toml
README.md
src/
  pmem/
    __init__.py
    audit.py
    cli.py
    constants.py
    defaults.py
    index.py
    mcp_server.py
    models.py
    paths.py
    service.py
    yaml_io.py
tests/
  conftest.py
  test_audit.py
  test_cli.py
  test_index.py
  test_models.py
  test_service.py
  test_yaml_io.py
```

Responsibilities:

- `constants.py`: allowed memory types, statuses, source kinds, relation kinds, and required card fields.
- `models.py`: validation errors, memory card dataclass, conversion between Python data and YAML-safe dicts.
- `paths.py`: project root and `.project-memory` path helpers.
- `defaults.py`: default `config.yaml` and `rules/agent-rules.yaml` content.
- `yaml_io.py`: YAML read/write, ID generation, filename generation, card discovery.
- `index.py`: SQLite schema, upsert, rebuild, search, recent list.
- `service.py`: high-level operations used by CLI and MCP.
- `audit.py`: memory quality checks.
- `cli.py`: `pmem` command line interface.
- `mcp_server.py`: five MCP tools: `remember`, `recall`, `open_memory`, `update_memory`, `list_recent`.

---

### Task 1: Project Scaffold

**Files:**
- Create: `pyproject.toml`
- Create: `README.md`
- Create: `src/pmem/__init__.py`
- Create: `tests/conftest.py`

- [ ] **Step 1: Write the package metadata**

Create `pyproject.toml` with:

```toml
[build-system]
requires = ["setuptools>=69", "wheel"]
build-backend = "setuptools.build_meta"

[project]
name = "project-memory-palace"
version = "0.1.0"
description = "Local YAML-backed project memory with SQLite search, CLI, and MCP tools."
readme = "README.md"
requires-python = ">=3.11"
dependencies = [
  "PyYAML>=6.0.2",
  "mcp>=1.12.0"
]

[project.optional-dependencies]
dev = [
  "pytest>=8.0"
]

[project.scripts]
pmem = "pmem.cli:main"
pmem-mcp = "pmem.mcp_server:main"

[tool.pytest.ini_options]
testpaths = ["tests"]
pythonpath = ["src"]
```

- [ ] **Step 2: Write the README skeleton**

Create `README.md` with:

```markdown
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
```

- [ ] **Step 3: Create package marker**

Create `src/pmem/__init__.py` with:

```python
"""Project Memory Palace package."""

__all__ = ["__version__"]

__version__ = "0.1.0"
```

- [ ] **Step 4: Create pytest fixture helper**

Create `tests/conftest.py` with:

```python
from pathlib import Path

import pytest


@pytest.fixture
def project_root(tmp_path: Path) -> Path:
    return tmp_path / "demo-project"
```

- [ ] **Step 5: Install development dependencies**

Run:

```bash
python -m pip install -e ".[dev]"
```

Expected: package installs and exposes `pmem` and `pmem-mcp`.

- [ ] **Step 6: Run the empty test suite**

Run:

```bash
pytest -q
```

Expected: `no tests ran` or zero failing tests.

- [ ] **Step 7: Commit**

```bash
git add pyproject.toml README.md src/pmem/__init__.py tests/conftest.py
git commit -m "chore: scaffold project memory package"
```

---

### Task 2: Card Constants and Validation

**Files:**
- Create: `src/pmem/constants.py`
- Create: `src/pmem/models.py`
- Create: `tests/test_models.py`

- [ ] **Step 1: Write failing tests for valid and invalid cards**

Create `tests/test_models.py` with:

```python
import pytest

from pmem.models import MemoryCard, ValidationError, validate_card_data


def valid_card_data() -> dict:
    return {
        "schema_version": 1,
        "id": "mem_20260609_001",
        "type": "decision",
        "status": "active",
        "confidence": 0.86,
        "title": "Use YAML cards",
        "summary": "YAML cards are the source of truth.",
        "content": "SQLite is a rebuildable index.",
        "source": {
            "kind": "conversation",
            "description": "User confirmed this design.",
            "files": [],
            "commits": [],
        },
        "scope": {
            "project": "project-memory-palace",
            "modules": ["storage"],
            "paths": [],
        },
        "tags": ["yaml", "storage"],
        "relations": {
            "supersedes": [],
            "superseded_by": [],
            "related_to": [],
            "explains": [],
            "caused_by": [],
        },
        "created_at": "2026-06-09T17:00:00+08:00",
        "updated_at": "2026-06-09T17:00:00+08:00",
    }


def test_validate_card_data_accepts_valid_card():
    validate_card_data(valid_card_data())


def test_validate_card_data_rejects_missing_required_field():
    data = valid_card_data()
    del data["summary"]

    with pytest.raises(ValidationError) as error:
        validate_card_data(data)

    assert "summary" in str(error.value)


def test_validate_card_data_rejects_unknown_type():
    data = valid_card_data()
    data["type"] = "note"

    with pytest.raises(ValidationError) as error:
        validate_card_data(data)

    assert "type" in str(error.value)


def test_validate_card_data_rejects_out_of_range_confidence():
    data = valid_card_data()
    data["confidence"] = 1.2

    with pytest.raises(ValidationError) as error:
        validate_card_data(data)

    assert "confidence" in str(error.value)


def test_memory_card_round_trips_to_dict():
    card = MemoryCard.from_dict(valid_card_data())

    assert card.id == "mem_20260609_001"
    assert card.to_dict()["relations"]["supersedes"] == []
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
pytest tests/test_models.py -q
```

Expected: FAIL with `ModuleNotFoundError: No module named 'pmem.models'`.

- [ ] **Step 3: Define constants**

Create `src/pmem/constants.py` with:

```python
MEMORY_TYPES = {
    "project_goal",
    "design",
    "decision",
    "change_reason",
    "bugfix",
    "module",
    "convention",
    "open_question",
}

MEMORY_STATUSES = {
    "active",
    "stale",
    "superseded",
    "rejected",
}

SOURCE_KINDS = {
    "conversation",
    "file",
    "commit",
    "manual",
    "test",
    "analysis",
}

RELATION_KINDS = {
    "supersedes",
    "superseded_by",
    "related_to",
    "explains",
    "caused_by",
}

REQUIRED_FIELDS = {
    "schema_version",
    "id",
    "type",
    "status",
    "confidence",
    "title",
    "summary",
    "content",
    "source",
    "scope",
    "tags",
    "relations",
    "created_at",
    "updated_at",
}
```

- [ ] **Step 4: Implement card validation and dataclass**

Create `src/pmem/models.py` with:

```python
from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from pmem.constants import (
    MEMORY_STATUSES,
    MEMORY_TYPES,
    RELATION_KINDS,
    REQUIRED_FIELDS,
    SOURCE_KINDS,
)


class ValidationError(ValueError):
    """Raised when a memory card does not match the required schema."""


def _require_mapping(value: Any, field_name: str) -> dict:
    if not isinstance(value, dict):
        raise ValidationError(f"{field_name} must be a mapping")
    return value


def _require_list(value: Any, field_name: str) -> list:
    if not isinstance(value, list):
        raise ValidationError(f"{field_name} must be a list")
    return value


def validate_card_data(data: dict[str, Any]) -> None:
    missing = sorted(REQUIRED_FIELDS.difference(data))
    if missing:
        raise ValidationError(f"missing required fields: {', '.join(missing)}")

    if data["schema_version"] != 1:
        raise ValidationError("schema_version must be 1")

    if data["type"] not in MEMORY_TYPES:
        raise ValidationError(f"type must be one of: {', '.join(sorted(MEMORY_TYPES))}")

    if data["status"] not in MEMORY_STATUSES:
        raise ValidationError(f"status must be one of: {', '.join(sorted(MEMORY_STATUSES))}")

    confidence = data["confidence"]
    if not isinstance(confidence, int | float) or not 0.0 <= float(confidence) <= 1.0:
        raise ValidationError("confidence must be a number between 0.0 and 1.0")

    for field_name in ("id", "title", "summary", "content", "created_at", "updated_at"):
        if not isinstance(data[field_name], str) or not data[field_name].strip():
            raise ValidationError(f"{field_name} must be a non-empty string")

    source = _require_mapping(data["source"], "source")
    if source.get("kind") not in SOURCE_KINDS:
        raise ValidationError(f"source.kind must be one of: {', '.join(sorted(SOURCE_KINDS))}")
    if not isinstance(source.get("description"), str) or not source["description"].strip():
        raise ValidationError("source.description must be a non-empty string")
    _require_list(source.get("files", []), "source.files")
    _require_list(source.get("commits", []), "source.commits")

    scope = _require_mapping(data["scope"], "scope")
    if not isinstance(scope.get("project"), str):
        raise ValidationError("scope.project must be a string")
    _require_list(scope.get("modules", []), "scope.modules")
    _require_list(scope.get("paths", []), "scope.paths")

    _require_list(data["tags"], "tags")

    relations = _require_mapping(data["relations"], "relations")
    for relation in RELATION_KINDS:
        _require_list(relations.get(relation, []), f"relations.{relation}")


@dataclass(frozen=True)
class MemoryCard:
    data: dict[str, Any]

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "MemoryCard":
        validate_card_data(data)
        return cls(data=dict(data))

    @property
    def id(self) -> str:
        return self.data["id"]

    @property
    def type(self) -> str:
        return self.data["type"]

    @property
    def status(self) -> str:
        return self.data["status"]

    @property
    def updated_at(self) -> str:
        return self.data["updated_at"]

    def to_dict(self) -> dict[str, Any]:
        return dict(self.data)
```

- [ ] **Step 5: Run tests**

Run:

```bash
pytest tests/test_models.py -q
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/pmem/constants.py src/pmem/models.py tests/test_models.py
git commit -m "feat: add memory card validation"
```

---

### Task 3: YAML IO and Project Initialization

**Files:**
- Create: `src/pmem/paths.py`
- Create: `src/pmem/defaults.py`
- Create: `src/pmem/yaml_io.py`
- Create: `tests/test_yaml_io.py`

- [ ] **Step 1: Write failing tests for initialization and YAML round trip**

Create `tests/test_yaml_io.py` with:

```python
from pathlib import Path

from pmem.yaml_io import (
    discover_cards,
    ensure_project_memory,
    next_card_identity,
    read_card,
    write_card,
)


def card_data(card_id: str = "mem_20260609_001") -> dict:
    return {
        "schema_version": 1,
        "id": card_id,
        "type": "decision",
        "status": "active",
        "confidence": 0.9,
        "title": "YAML storage",
        "summary": "YAML is the source of truth.",
        "content": "The index can be rebuilt from YAML.",
        "source": {
            "kind": "conversation",
            "description": "User confirmed YAML storage.",
            "files": [],
            "commits": [],
        },
        "scope": {
            "project": "demo",
            "modules": ["storage"],
            "paths": [],
        },
        "tags": ["yaml"],
        "relations": {
            "supersedes": [],
            "superseded_by": [],
            "related_to": [],
            "explains": [],
            "caused_by": [],
        },
        "created_at": "2026-06-09T17:00:00+08:00",
        "updated_at": "2026-06-09T17:00:00+08:00",
    }


def test_ensure_project_memory_creates_expected_layout(project_root: Path):
    ensure_project_memory(project_root)

    assert (project_root / ".project-memory" / "cards").is_dir()
    assert (project_root / ".project-memory" / "rules" / "agent-rules.yaml").is_file()
    assert (project_root / ".project-memory" / "config.yaml").is_file()


def test_write_and_read_card_round_trip(project_root: Path):
    ensure_project_memory(project_root)

    path = write_card(project_root, card_data())
    loaded = read_card(path)

    assert path.name == "2026-06-09_001_decision.yaml"
    assert loaded.id == "mem_20260609_001"


def test_discover_cards_returns_sorted_cards(project_root: Path):
    ensure_project_memory(project_root)
    write_card(project_root, card_data("mem_20260609_002"))
    write_card(project_root, card_data("mem_20260609_001"))

    cards = discover_cards(project_root)

    assert [card.id for card in cards] == ["mem_20260609_001", "mem_20260609_002"]


def test_next_card_identity_uses_existing_sequence(project_root: Path):
    ensure_project_memory(project_root)
    write_card(project_root, card_data("mem_20260609_001"))

    card_id, sequence = next_card_identity(project_root, "2026-06-09")

    assert card_id == "mem_20260609_002"
    assert sequence == 2
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
pytest tests/test_yaml_io.py -q
```

Expected: FAIL with `ModuleNotFoundError: No module named 'pmem.yaml_io'`.

- [ ] **Step 3: Implement path helpers**

Create `src/pmem/paths.py` with:

```python
from pathlib import Path


MEMORY_DIR = ".project-memory"


def memory_dir(project_root: Path) -> Path:
    return project_root / MEMORY_DIR


def cards_dir(project_root: Path) -> Path:
    return memory_dir(project_root) / "cards"


def rules_dir(project_root: Path) -> Path:
    return memory_dir(project_root) / "rules"


def config_path(project_root: Path) -> Path:
    return memory_dir(project_root) / "config.yaml"


def rules_path(project_root: Path) -> Path:
    return rules_dir(project_root) / "agent-rules.yaml"


def index_path(project_root: Path) -> Path:
    return memory_dir(project_root) / "index.sqlite3"
```

- [ ] **Step 4: Implement default config and rules**

Create `src/pmem/defaults.py` with:

```python
DEFAULT_CONFIG = """\
schema_version: 1
project_name: ""
default_recall_limit: 5
search:
  use_embeddings: false
  include_rejected_by_default: false
"""

DEFAULT_AGENT_RULES = """\
schema_version: 1
must_remember_after:
  - solved_problem
  - implemented_feature
  - fixed_bug
  - produced_design
  - clarified_project_goal
  - discovered_module_workflow
  - changed_previous_decision
  - established_convention
do_not_remember:
  - temporary_command_output
  - low_value_debug_noise
  - unconfirmed_guess
  - duplicate_without_new_information
  - secrets_tokens_accounts_private_data
memory_value_checks:
  - helps_future_project_understanding
  - reduces_repeated_analysis
  - has_clear_source
  - not_duplicate_or_links_to_existing_memory
  - supersedes_old_memory_when_needed
"""
```

- [ ] **Step 5: Implement YAML IO**

Create `src/pmem/yaml_io.py` with:

```python
from __future__ import annotations

import re
from pathlib import Path
from typing import Any

import yaml

from pmem.defaults import DEFAULT_AGENT_RULES, DEFAULT_CONFIG
from pmem.models import MemoryCard
from pmem.paths import cards_dir, config_path, memory_dir, rules_dir, rules_path


ID_RE = re.compile(r"^mem_(\d{8})_(\d{3})$")


def ensure_project_memory(project_root: Path) -> None:
    cards_dir(project_root).mkdir(parents=True, exist_ok=True)
    rules_dir(project_root).mkdir(parents=True, exist_ok=True)
    if not config_path(project_root).exists():
        config_path(project_root).write_text(DEFAULT_CONFIG, encoding="utf-8")
    if not rules_path(project_root).exists():
        rules_path(project_root).write_text(DEFAULT_AGENT_RULES, encoding="utf-8")


def card_filename(card: dict[str, Any]) -> str:
    match = ID_RE.match(card["id"])
    if not match:
        raise ValueError(f"invalid memory id: {card['id']}")
    date_token = match.group(1)
    sequence = match.group(2)
    date_part = f"{date_token[0:4]}-{date_token[4:6]}-{date_token[6:8]}"
    return f"{date_part}_{sequence}_{card['type']}.yaml"


def write_card(project_root: Path, data: dict[str, Any]) -> Path:
    ensure_project_memory(project_root)
    card = MemoryCard.from_dict(data)
    path = cards_dir(project_root) / card_filename(card.to_dict())
    path.write_text(
        yaml.safe_dump(card.to_dict(), sort_keys=False, allow_unicode=True),
        encoding="utf-8",
    )
    return path


def read_card(path: Path) -> MemoryCard:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise ValueError(f"card file must contain a mapping: {path}")
    return MemoryCard.from_dict(data)


def discover_cards(project_root: Path) -> list[MemoryCard]:
    ensure_project_memory(project_root)
    cards = [read_card(path) for path in cards_dir(project_root).glob("*.yaml")]
    return sorted(cards, key=lambda card: card.id)


def next_card_identity(project_root: Path, date_part: str) -> tuple[str, int]:
    ensure_project_memory(project_root)
    date_token = date_part.replace("-", "")
    max_sequence = 0
    for card in discover_cards(project_root):
        match = ID_RE.match(card.id)
        if match and match.group(1) == date_token:
            max_sequence = max(max_sequence, int(match.group(2)))
    next_sequence = max_sequence + 1
    return f"mem_{date_token}_{next_sequence:03d}", next_sequence


def assert_memory_layout(project_root: Path) -> None:
    required = [memory_dir(project_root), cards_dir(project_root), config_path(project_root), rules_path(project_root)]
    missing = [str(path) for path in required if not path.exists()]
    if missing:
        raise FileNotFoundError(f"project memory is not initialized: {', '.join(missing)}")
```

- [ ] **Step 6: Run tests**

Run:

```bash
pytest tests/test_yaml_io.py -q
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/pmem/paths.py src/pmem/defaults.py src/pmem/yaml_io.py tests/test_yaml_io.py
git commit -m "feat: add yaml memory storage"
```

---

### Task 4: SQLite Index

**Files:**
- Create: `src/pmem/index.py`
- Create: `tests/test_index.py`

- [ ] **Step 1: Write failing tests for index rebuild, search, and recent**

Create `tests/test_index.py` with:

```python
from pathlib import Path

from pmem.index import MemoryIndex
from pmem.yaml_io import ensure_project_memory, write_card


def card_data(card_id: str, title: str, tag: str, path: str) -> dict:
    return {
        "schema_version": 1,
        "id": card_id,
        "type": "decision",
        "status": "active",
        "confidence": 0.9,
        "title": title,
        "summary": f"{title} summary",
        "content": f"{title} full content",
        "source": {
            "kind": "conversation",
            "description": "User confirmed this.",
            "files": [],
            "commits": [],
        },
        "scope": {
            "project": "demo",
            "modules": ["storage"],
            "paths": [path],
        },
        "tags": [tag],
        "relations": {
            "supersedes": [],
            "superseded_by": [],
            "related_to": [],
            "explains": [],
            "caused_by": [],
        },
        "created_at": "2026-06-09T17:00:00+08:00",
        "updated_at": f"2026-06-09T17:0{card_id[-1]}:00+08:00",
    }


def test_rebuild_indexes_cards(project_root: Path):
    ensure_project_memory(project_root)
    write_card(project_root, card_data("mem_20260609_001", "YAML storage", "yaml", "src/storage.py"))

    index = MemoryIndex(project_root)
    index.rebuild()

    results = index.search("YAML", {}, 5)
    assert results[0]["id"] == "mem_20260609_001"
    assert "content" not in results[0]


def test_search_filters_by_path(project_root: Path):
    ensure_project_memory(project_root)
    write_card(project_root, card_data("mem_20260609_001", "YAML storage", "yaml", "src/storage.py"))
    write_card(project_root, card_data("mem_20260609_002", "MCP server", "mcp", "src/mcp_server.py"))

    index = MemoryIndex(project_root)
    index.rebuild()

    results = index.search("server", {"paths": ["src/mcp_server.py"]}, 5)
    assert [row["id"] for row in results] == ["mem_20260609_002"]


def test_recent_returns_newest_first(project_root: Path):
    ensure_project_memory(project_root)
    write_card(project_root, card_data("mem_20260609_001", "First", "one", "a.py"))
    write_card(project_root, card_data("mem_20260609_002", "Second", "two", "b.py"))

    index = MemoryIndex(project_root)
    index.rebuild()

    recent = index.recent(limit=2)
    assert [row["id"] for row in recent] == ["mem_20260609_002", "mem_20260609_001"]
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
pytest tests/test_index.py -q
```

Expected: FAIL with `ModuleNotFoundError: No module named 'pmem.index'`.

- [ ] **Step 3: Implement SQLite index**

Create `src/pmem/index.py` with:

```python
from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any

from pmem.models import MemoryCard
from pmem.paths import index_path
from pmem.yaml_io import discover_cards, ensure_project_memory


class MemoryIndex:
    def __init__(self, project_root: Path):
        self.project_root = project_root
        ensure_project_memory(project_root)
        self.path = index_path(project_root)

    def connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(self.path)
        connection.row_factory = sqlite3.Row
        return connection

    def initialize(self) -> None:
        with self.connect() as connection:
            connection.executescript(
                """
                CREATE TABLE IF NOT EXISTS memories (
                    id TEXT PRIMARY KEY,
                    type TEXT NOT NULL,
                    status TEXT NOT NULL,
                    title TEXT NOT NULL,
                    summary TEXT NOT NULL,
                    source_kind TEXT NOT NULL,
                    confidence REAL NOT NULL,
                    tags_json TEXT NOT NULL,
                    modules_json TEXT NOT NULL,
                    paths_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );

                CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
                    id UNINDEXED,
                    title,
                    summary,
                    content,
                    tags,
                    modules,
                    paths,
                    tokenize='unicode61'
                );

                CREATE TABLE IF NOT EXISTS relations (
                    source_id TEXT NOT NULL,
                    relation TEXT NOT NULL,
                    target_id TEXT NOT NULL,
                    PRIMARY KEY (source_id, relation, target_id)
                );
                """
            )

    def clear(self) -> None:
        self.initialize()
        with self.connect() as connection:
            connection.execute("DELETE FROM relations")
            connection.execute("DELETE FROM memory_fts")
            connection.execute("DELETE FROM memories")

    def rebuild(self) -> None:
        self.clear()
        for card in discover_cards(self.project_root):
            self.upsert(card)

    def upsert(self, card: MemoryCard) -> None:
        self.initialize()
        data = card.to_dict()
        tags = data["tags"]
        modules = data["scope"].get("modules", [])
        paths = data["scope"].get("paths", [])
        source_kind = data["source"]["kind"]
        relations = data["relations"]
        with self.connect() as connection:
            connection.execute(
                """
                INSERT INTO memories (
                    id, type, status, title, summary, source_kind, confidence,
                    tags_json, modules_json, paths_json, updated_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT(id) DO UPDATE SET
                    type=excluded.type,
                    status=excluded.status,
                    title=excluded.title,
                    summary=excluded.summary,
                    source_kind=excluded.source_kind,
                    confidence=excluded.confidence,
                    tags_json=excluded.tags_json,
                    modules_json=excluded.modules_json,
                    paths_json=excluded.paths_json,
                    updated_at=excluded.updated_at
                """,
                (
                    data["id"],
                    data["type"],
                    data["status"],
                    data["title"],
                    data["summary"],
                    source_kind,
                    data["confidence"],
                    json.dumps(tags),
                    json.dumps(modules),
                    json.dumps(paths),
                    data["updated_at"],
                ),
            )
            connection.execute("DELETE FROM memory_fts WHERE id = ?", (data["id"],))
            connection.execute(
                """
                INSERT INTO memory_fts (id, title, summary, content, tags, modules, paths)
                VALUES (?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    data["id"],
                    data["title"],
                    data["summary"],
                    data["content"],
                    " ".join(tags),
                    " ".join(modules),
                    " ".join(paths),
                ),
            )
            connection.execute("DELETE FROM relations WHERE source_id = ?", (data["id"],))
            for relation, targets in relations.items():
                for target in targets:
                    connection.execute(
                        "INSERT OR IGNORE INTO relations (source_id, relation, target_id) VALUES (?, ?, ?)",
                        (data["id"], relation, target),
                    )

    def search(self, query: str, filters: dict[str, Any], limit: int) -> list[dict[str, Any]]:
        self.initialize()
        status_filter = filters.get("status") or ["active"]
        path_filter = filters.get("paths") or []
        params: list[Any] = [query]
        where = ["m.status IN ({})".format(",".join("?" for _ in status_filter))]
        params.extend(status_filter)
        if path_filter:
            path_checks = []
            for path in path_filter:
                path_checks.append("m.paths_json LIKE ?")
                params.append(f"%{path}%")
            where.append("(" + " OR ".join(path_checks) + ")")
        params.append(limit)
        sql = f"""
            SELECT
                m.id, m.type, m.status, m.title, m.summary, m.source_kind,
                m.confidence, m.updated_at, bm25(memory_fts) AS rank
            FROM memory_fts
            JOIN memories m ON m.id = memory_fts.id
            WHERE memory_fts MATCH ?
              AND {' AND '.join(where)}
            ORDER BY rank ASC, m.updated_at DESC
            LIMIT ?
        """
        with self.connect() as connection:
            rows = connection.execute(sql, params).fetchall()
        return [self._summary_row(row, ["fts"]) for row in rows]

    def recent(self, limit: int) -> list[dict[str, Any]]:
        self.initialize()
        with self.connect() as connection:
            rows = connection.execute(
                """
                SELECT id, type, status, title, summary, source_kind, confidence, updated_at
                FROM memories
                ORDER BY updated_at DESC
                LIMIT ?
                """,
                (limit,),
            ).fetchall()
        return [self._summary_row(row, ["recent"]) for row in rows]

    def _summary_row(self, row: sqlite3.Row, matched_by: list[str]) -> dict[str, Any]:
        return {
            "id": row["id"],
            "type": row["type"],
            "status": row["status"],
            "title": row["title"],
            "summary": row["summary"],
            "confidence": row["confidence"],
            "source_hint": row["source_kind"],
            "matched_by": matched_by,
            "updated_at": row["updated_at"],
        }
```

- [ ] **Step 4: Run index tests**

Run:

```bash
pytest tests/test_index.py -q
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/pmem/index.py tests/test_index.py
git commit -m "feat: add sqlite memory index"
```

---

### Task 5: Core Memory Service

**Files:**
- Create: `src/pmem/service.py`
- Create: `tests/test_service.py`

- [ ] **Step 1: Write failing service tests**

Create `tests/test_service.py` with:

```python
from pathlib import Path

from pmem.service import MemoryService


def remember_input() -> dict:
    return {
        "type": "decision",
        "title": "YAML storage",
        "summary": "YAML is the source of truth.",
        "content": "SQLite is only an index.",
        "confidence": 0.9,
        "source": {
            "kind": "conversation",
            "description": "User confirmed YAML storage.",
            "files": [],
            "commits": [],
        },
        "scope": {
            "project": "demo",
            "modules": ["storage"],
            "paths": ["src/storage.py"],
        },
        "tags": ["yaml", "storage"],
        "relations": {
            "supersedes": [],
            "related_to": [],
            "explains": [],
            "caused_by": [],
        },
    }


def test_init_creates_memory_layout(project_root: Path):
    service = MemoryService(project_root)

    service.init_project()

    assert (project_root / ".project-memory" / "cards").is_dir()
    assert (project_root / ".project-memory" / "index.sqlite3").is_file()


def test_remember_writes_card_and_notification(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()

    result = service.remember(remember_input())

    assert result["id"].startswith("mem_")
    assert "Project memory written" in result["notification"]
    assert service.open_memory(result["id"])["title"] == "YAML storage"


def test_remember_caps_missing_source_confidence(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload.pop("source")
    payload["confidence"] = 0.95

    result = service.remember(payload)
    card = service.open_memory(result["id"])

    assert card["confidence"] == 0.5
    assert card["source"]["kind"] == "analysis"


def test_recall_returns_summary_without_content(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    service.remember(remember_input())

    results = service.recall("YAML", {}, 5)

    assert results[0]["title"] == "YAML storage"
    assert "content" not in results[0]


def test_update_memory_changes_status(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(remember_input())

    updated = service.update_memory(created["id"], {"status": "stale", "reason": "Needs review."})

    assert updated["status"] == "stale"
    assert service.open_memory(created["id"])["status"] == "stale"
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
pytest tests/test_service.py -q
```

Expected: FAIL with `ModuleNotFoundError: No module named 'pmem.service'`.

- [ ] **Step 3: Implement service**

Create `src/pmem/service.py` with:

```python
from __future__ import annotations

from copy import deepcopy
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from pmem.constants import RELATION_KINDS
from pmem.index import MemoryIndex
from pmem.models import MemoryCard
from pmem.yaml_io import discover_cards, ensure_project_memory, next_card_identity, write_card


class MemoryNotFoundError(KeyError):
    """Raised when a memory id is not found."""


class MemoryService:
    def __init__(self, project_root: Path):
        self.project_root = project_root
        self.index = MemoryIndex(project_root)

    def init_project(self) -> None:
        ensure_project_memory(self.project_root)
        self.index.initialize()

    def remember(self, payload: dict[str, Any]) -> dict[str, Any]:
        self.init_project()
        now = self._now()
        card_id, _sequence = next_card_identity(self.project_root, now[:10])
        data = self._build_card(card_id, payload, now)
        path = write_card(self.project_root, data)
        card = MemoryCard.from_dict(data)
        self.index.upsert(card)
        return {
            "id": card.id,
            "path": str(path),
            "notification": self._notification(card),
        }

    def recall(self, query: str, filters: dict[str, Any] | None, limit: int = 5) -> list[dict[str, Any]]:
        self.init_project()
        return self.index.search(query, filters or {}, limit)

    def open_memory(self, memory_id: str) -> dict[str, Any]:
        for card in discover_cards(self.project_root):
            if card.id == memory_id:
                return card.to_dict()
        raise MemoryNotFoundError(f"memory not found: {memory_id}")

    def list_recent(self, limit: int = 10) -> list[dict[str, Any]]:
        self.init_project()
        return self.index.recent(limit)

    def update_memory(self, memory_id: str, updates: dict[str, Any]) -> dict[str, Any]:
        existing = self.open_memory(memory_id)
        data = deepcopy(existing)
        if "status" in updates:
            data["status"] = updates["status"]
        if "confidence" in updates:
            data["confidence"] = updates["confidence"]
        if "tags" in updates:
            data["tags"] = updates["tags"]
        if "relations" in updates:
            relations = data.setdefault("relations", self._empty_relations())
            for relation, targets in updates["relations"].items():
                current = list(relations.get(relation, []))
                for target in targets:
                    if target not in current:
                        current.append(target)
                relations[relation] = current
        data["updated_at"] = self._now()
        write_card(self.project_root, data)
        card = MemoryCard.from_dict(data)
        self.index.upsert(card)
        return card.to_dict()

    def rebuild_index(self) -> None:
        self.init_project()
        self.index.rebuild()

    def _build_card(self, card_id: str, payload: dict[str, Any], now: str) -> dict[str, Any]:
        source = payload.get("source")
        confidence = float(payload.get("confidence", 0.5))
        if not source:
            source = {
                "kind": "analysis",
                "description": "Source was not supplied by caller.",
                "files": [],
                "commits": [],
            }
            confidence = min(confidence, 0.5)
        source.setdefault("files", [])
        source.setdefault("commits", [])
        scope = payload.get("scope") or {}
        scope.setdefault("project", "")
        scope.setdefault("modules", [])
        scope.setdefault("paths", [])
        relations = self._empty_relations()
        for relation, targets in (payload.get("relations") or {}).items():
            if relation in relations:
                relations[relation] = list(targets)
        return {
            "schema_version": 1,
            "id": card_id,
            "type": payload["type"],
            "status": payload.get("status", "active"),
            "confidence": confidence,
            "title": payload["title"],
            "summary": payload["summary"],
            "content": payload["content"],
            "source": source,
            "scope": scope,
            "tags": list(payload.get("tags", [])),
            "relations": relations,
            "created_at": now,
            "updated_at": now,
        }

    def _empty_relations(self) -> dict[str, list[str]]:
        return {relation: [] for relation in RELATION_KINDS}

    def _now(self) -> str:
        return datetime.now(timezone.utc).astimezone().isoformat(timespec="seconds")

    def _notification(self, card: MemoryCard) -> str:
        data = card.to_dict()
        return (
            "Project memory written:\n"
            f"- ID: {data['id']}\n"
            f"- Type: {data['type']}\n"
            f"- Summary: {data['summary']}\n"
            f"- Source: {data['source']['kind']} - {data['source']['description']}\n"
            "- Future use: use recall to retrieve this summary, then open_memory for details."
        )
```

- [ ] **Step 4: Run service tests**

Run:

```bash
pytest tests/test_service.py -q
```

Expected: PASS.

- [ ] **Step 5: Run existing tests**

Run:

```bash
pytest -q
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/pmem/service.py tests/test_service.py
git commit -m "feat: add core memory service"
```

---

### Task 6: Audit

**Files:**
- Create: `src/pmem/audit.py`
- Create: `tests/test_audit.py`

- [ ] **Step 1: Write failing audit tests**

Create `tests/test_audit.py` with:

```python
from pathlib import Path

from pmem.audit import audit_project
from pmem.service import MemoryService


def payload() -> dict:
    return {
        "type": "decision",
        "title": "Untaged low confidence",
        "summary": "This card needs audit.",
        "content": "The card has no tags and low confidence.",
        "confidence": 0.4,
        "source": {
            "kind": "analysis",
            "description": "AI inference.",
            "files": [],
            "commits": [],
        },
        "scope": {
            "project": "demo",
            "modules": [],
            "paths": [],
        },
        "tags": [],
        "relations": {},
    }


def test_audit_reports_low_quality_card(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(payload())

    report = audit_project(project_root)

    assert report[0]["id"] == created["id"]
    assert "low_confidence" in report[0]["issues"]
    assert "missing_tags" in report[0]["issues"]
    assert "missing_scope" in report[0]["issues"]
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
pytest tests/test_audit.py -q
```

Expected: FAIL with `ModuleNotFoundError: No module named 'pmem.audit'`.

- [ ] **Step 3: Implement audit**

Create `src/pmem/audit.py` with:

```python
from __future__ import annotations

from pathlib import Path
from typing import Any

from pmem.yaml_io import discover_cards


def audit_project(project_root: Path) -> list[dict[str, Any]]:
    report: list[dict[str, Any]] = []
    seen_titles: dict[str, str] = {}
    for card in discover_cards(project_root):
        data = card.to_dict()
        issues: list[str] = []
        if data["confidence"] <= 0.5:
            issues.append("low_confidence")
        if not data["tags"]:
            issues.append("missing_tags")
        if not data["scope"].get("modules") and not data["scope"].get("paths"):
            issues.append("missing_scope")
        if data["source"]["kind"] == "analysis" and data["confidence"] > 0.7:
            issues.append("high_confidence_inference")
        title_key = data["title"].strip().lower()
        if title_key in seen_titles:
            issues.append(f"possible_duplicate:{seen_titles[title_key]}")
        else:
            seen_titles[title_key] = data["id"]
        if data["status"] == "stale":
            issues.append("stale")
        if issues:
            report.append(
                {
                    "id": data["id"],
                    "title": data["title"],
                    "status": data["status"],
                    "issues": issues,
                }
            )
    return report
```

- [ ] **Step 4: Run audit tests**

Run:

```bash
pytest tests/test_audit.py -q
```

Expected: PASS.

- [ ] **Step 5: Run full tests**

Run:

```bash
pytest -q
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/pmem/audit.py tests/test_audit.py
git commit -m "feat: add memory audit checks"
```

---

### Task 7: CLI

**Files:**
- Create: `src/pmem/cli.py`
- Create: `tests/test_cli.py`

- [ ] **Step 1: Write failing CLI tests**

Create `tests/test_cli.py` with:

```python
from pathlib import Path

from pmem.cli import run


def test_cli_init(project_root: Path):
    code = run(["--project-root", str(project_root), "init"])

    assert code == 0
    assert (project_root / ".project-memory" / "cards").is_dir()


def test_cli_remember_and_search(project_root: Path, capsys):
    run(["--project-root", str(project_root), "init"])
    card_file = project_root / "card.yaml"
    card_file.write_text(
        """
type: decision
title: YAML storage
summary: YAML is the source of truth.
content: SQLite is only an index.
confidence: 0.9
source:
  kind: conversation
  description: User confirmed YAML storage.
  files: []
  commits: []
scope:
  project: demo
  modules: [storage]
  paths: [src/storage.py]
tags: [yaml, storage]
relations:
  supersedes: []
  related_to: []
  explains: []
  caused_by: []
""".strip(),
        encoding="utf-8",
    )

    assert run(["--project-root", str(project_root), "remember", "--file", str(card_file)]) == 0
    assert run(["--project-root", str(project_root), "search", "YAML"]) == 0
    output = capsys.readouterr().out

    assert "YAML storage" in output
    assert "SQLite is only an index" not in output
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
pytest tests/test_cli.py -q
```

Expected: FAIL with `ModuleNotFoundError: No module named 'pmem.cli'`.

- [ ] **Step 3: Implement CLI**

Create `src/pmem/cli.py` with:

```python
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import yaml

from pmem.audit import audit_project
from pmem.service import MemoryNotFoundError, MemoryService


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="pmem")
    parser.add_argument("--project-root", default=".", help="Project root containing .project-memory")
    subcommands = parser.add_subparsers(dest="command", required=True)

    subcommands.add_parser("init")

    remember = subcommands.add_parser("remember")
    remember.add_argument("--file", required=True)

    search = subcommands.add_parser("search")
    search.add_argument("query")
    search.add_argument("--limit", type=int, default=5)

    open_cmd = subcommands.add_parser("open")
    open_cmd.add_argument("id")

    recent = subcommands.add_parser("recent")
    recent.add_argument("--limit", type=int, default=10)

    update = subcommands.add_parser("update")
    update.add_argument("id")
    update.add_argument("--status")
    update.add_argument("--confidence", type=float)

    subcommands.add_parser("rebuild-index")
    subcommands.add_parser("audit")
    return parser


def run(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    service = MemoryService(Path(args.project_root))
    try:
        if args.command == "init":
            service.init_project()
            print("Initialized project memory.")
        elif args.command == "remember":
            payload = yaml.safe_load(Path(args.file).read_text(encoding="utf-8"))
            result = service.remember(payload)
            print(result["notification"])
        elif args.command == "search":
            results = service.recall(args.query, {}, args.limit)
            print(yaml.safe_dump({"results": results}, sort_keys=False, allow_unicode=True))
        elif args.command == "open":
            print(yaml.safe_dump(service.open_memory(args.id), sort_keys=False, allow_unicode=True))
        elif args.command == "recent":
            print(yaml.safe_dump({"results": service.list_recent(args.limit)}, sort_keys=False, allow_unicode=True))
        elif args.command == "update":
            updates = {}
            if args.status:
                updates["status"] = args.status
            if args.confidence is not None:
                updates["confidence"] = args.confidence
            print(yaml.safe_dump(service.update_memory(args.id, updates), sort_keys=False, allow_unicode=True))
        elif args.command == "rebuild-index":
            service.rebuild_index()
            print("Rebuilt memory index.")
        elif args.command == "audit":
            print(json.dumps({"issues": audit_project(Path(args.project_root))}, indent=2))
        return 0
    except (FileNotFoundError, MemoryNotFoundError, ValueError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


def main() -> None:
    raise SystemExit(run())
```

- [ ] **Step 4: Run CLI tests**

Run:

```bash
pytest tests/test_cli.py -q
```

Expected: PASS.

- [ ] **Step 5: Run full tests**

Run:

```bash
pytest -q
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/pmem/cli.py tests/test_cli.py
git commit -m "feat: add pmem cli"
```

---

### Task 8: MCP Server

**Files:**
- Create: `src/pmem/mcp_server.py`

- [ ] **Step 1: Verify MCP SDK import path**

Run:

```bash
python -c "from mcp.server.fastmcp import FastMCP; print(FastMCP)"
```

Expected: prints a `FastMCP` class.

- [ ] **Step 2: Implement MCP tools**

Create `src/pmem/mcp_server.py` with:

```python
from __future__ import annotations

from pathlib import Path
from typing import Any

from mcp.server.fastmcp import FastMCP

from pmem.service import MemoryService


mcp = FastMCP("project-memory-palace")


def _service(project_root: str) -> MemoryService:
    return MemoryService(Path(project_root))


@mcp.tool()
def remember(project_root: str, memory: dict[str, Any]) -> dict[str, Any]:
    """Write a project memory card and return the user notification payload."""
    return _service(project_root).remember(memory)


@mcp.tool()
def recall(project_root: str, query: str, filters: dict[str, Any] | None = None, limit: int = 5) -> dict[str, Any]:
    """Return relevant memory summaries. Full content is not returned."""
    return {"results": _service(project_root).recall(query, filters or {}, limit)}


@mcp.tool()
def open_memory(project_root: str, id: str) -> dict[str, Any]:
    """Open one full memory card by ID."""
    return _service(project_root).open_memory(id)


@mcp.tool()
def update_memory(project_root: str, id: str, updates: dict[str, Any]) -> dict[str, Any]:
    """Update memory status, relations, tags, or confidence."""
    return _service(project_root).update_memory(id, updates)


@mcp.tool()
def list_recent(project_root: str, limit: int = 10) -> dict[str, Any]:
    """List recently created or updated memory summaries."""
    return {"results": _service(project_root).list_recent(limit)}


def main() -> None:
    mcp.run()
```

- [ ] **Step 3: Add a smoke import command**

Run:

```bash
python -m pmem.mcp_server --help
```

Expected: command starts far enough to show SDK help or exits cleanly without import errors. If the SDK does not support `--help`, run `python -c "import pmem.mcp_server; print(pmem.mcp_server.mcp.name)"` and expect `project-memory-palace`.

- [ ] **Step 4: Run full tests**

Run:

```bash
pytest -q
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/pmem/mcp_server.py
git commit -m "feat: add mcp memory server"
```

---

### Task 9: End-to-End Smoke Test and Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add usage documentation**

Append this to `README.md`:

```markdown
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
```

- [ ] **Step 2: Create a temporary smoke card outside git**

Create `example-card.yaml` manually during the smoke test with:

```yaml
type: decision
title: YAML memory source
summary: YAML cards are the durable source of truth.
content: SQLite can be rebuilt from YAML cards.
confidence: 0.9
source:
  kind: manual
  description: Smoke test card.
  files: []
  commits: []
scope:
  project: smoke-test
  modules:
    - storage
  paths:
    - src/pmem/yaml_io.py
tags:
  - yaml
  - smoke-test
relations:
  supersedes: []
  related_to: []
  explains: []
  caused_by: []
```

- [ ] **Step 3: Run CLI smoke test**

Run:

```bash
pmem init
pmem remember --file example-card.yaml
pmem search "YAML"
pmem recent --limit 5
pmem rebuild-index
pmem audit
```

Expected:

- `pmem init` prints `Initialized project memory.`
- `pmem remember` prints `Project memory written:`
- `pmem search "YAML"` returns a result with title `YAML memory source`
- `pmem recent --limit 5` includes the same memory ID
- `pmem rebuild-index` prints `Rebuilt memory index.`
- `pmem audit` returns JSON

- [ ] **Step 4: Remove smoke artifacts**

Run:

```bash
Remove-Item -LiteralPath example-card.yaml
Remove-Item -LiteralPath .project-memory -Recurse
```

Expected: smoke files are removed from the repository root.

- [ ] **Step 5: Run full test suite**

Run:

```bash
pytest -q
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "docs: add usage and smoke test notes"
```

---

## Self-Review Checklist

Spec coverage:

- Local personal project memory: covered by `pmem init`, per-project `.project-memory`, and no server-side storage.
- YAML source of truth: covered by `yaml_io.py` and tests.
- SQLite rebuildable index: covered by `index.py`, `rebuild`, and tests.
- No embeddings: covered by FTS5 search only.
- MCP tools: covered by `mcp_server.py`.
- CLI: covered by `cli.py`.
- Automatic AI notification: covered by `MemoryService.remember` notification payload.
- Progressive recall: covered by `MemoryIndex.search`, which returns summaries without `content`.
- Status and relation updates: covered by `MemoryService.update_memory`.
- Audit: covered by `audit.py`.

Completion-marker scan:

- This plan contains no unresolved markers or deferred implementation instructions.
- Each code step names exact files and provides concrete code.

Type consistency:

- Memory IDs use `mem_YYYYMMDD_NNN`.
- Stored cards include `superseded_by` even when `remember` input omits it.
- CLI and MCP both use `MemoryService`.
- `recall` and `search` return summaries without full `content`.
