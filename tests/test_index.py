from pathlib import Path

import pytest

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
    write_card(
        project_root,
        card_data("mem_20260609_001", "YAML storage", "yaml", "src/storage.py"),
    )

    index = MemoryIndex(project_root)
    index.rebuild()

    results = index.search("YAML", {}, 5)
    assert results[0]["id"] == "mem_20260609_001"
    assert "content" not in results[0]


def test_search_filters_by_path(project_root: Path):
    ensure_project_memory(project_root)
    write_card(
        project_root,
        card_data("mem_20260609_001", "YAML storage", "yaml", "src/storage.py"),
    )
    write_card(
        project_root,
        card_data("mem_20260609_002", "MCP server", "mcp", "src/mcp_server.py"),
    )

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
    assert [row["id"] for row in recent] == [
        "mem_20260609_002",
        "mem_20260609_001",
    ]


def test_search_empty_or_no_token_query_returns_no_results(project_root: Path):
    ensure_project_memory(project_root)
    write_card(
        project_root,
        card_data("mem_20260609_001", "YAML storage", "yaml", "src/storage.py"),
    )

    index = MemoryIndex(project_root)
    index.rebuild()

    assert index.search("", {}, 5) == []
    assert index.search(' -- / "" . ', {}, 5) == []


def test_search_tokenizes_plain_text_with_quotes_hyphens_and_paths(project_root: Path):
    ensure_project_memory(project_root)
    write_card(
        project_root,
        card_data(
            "mem_20260609_001",
            "Storage adapter",
            "storage-helper",
            "src/storage.py",
        ),
    )

    index = MemoryIndex(project_root)
    index.rebuild()

    results = index.search('"src/storage.py" storage-helper', {}, 5)
    assert [row["id"] for row in results] == ["mem_20260609_001"]


def test_search_filters_by_exact_path(project_root: Path):
    ensure_project_memory(project_root)
    write_card(
        project_root,
        card_data("mem_20260609_001", "Backup server", "backup", "src/a.py.bak"),
    )
    write_card(
        project_root,
        card_data("mem_20260609_002", "Exact server", "exact", "src/a.py"),
    )

    index = MemoryIndex(project_root)
    index.rebuild()

    results = index.search("server", {"paths": ["src/a.py"]}, 5)
    assert [row["id"] for row in results] == ["mem_20260609_002"]


def test_rebuild_keeps_existing_index_when_discovery_fails(
    project_root: Path, monkeypatch: pytest.MonkeyPatch
):
    ensure_project_memory(project_root)
    write_card(
        project_root,
        card_data("mem_20260609_001", "Durable storage", "durable", "src/storage.py"),
    )
    index = MemoryIndex(project_root)
    index.rebuild()

    def fail_discovery(_project_root: Path):
        raise RuntimeError("invalid card")

    monkeypatch.setattr("pmem.index.discover_cards", fail_discovery)

    with pytest.raises(RuntimeError, match="invalid card"):
        index.rebuild()

    assert [row["id"] for row in index.recent(1)] == ["mem_20260609_001"]
    assert [row["id"] for row in index.search("Durable", {}, 5)] == [
        "mem_20260609_001"
    ]


def test_search_accepts_string_status_filter(project_root: Path):
    ensure_project_memory(project_root)
    write_card(
        project_root,
        card_data("mem_20260609_001", "Status active", "status", "active.py"),
    )
    stale_card = card_data("mem_20260609_002", "Status stale", "status", "stale.py")
    stale_card["status"] = "stale"
    write_card(project_root, stale_card)

    index = MemoryIndex(project_root)
    index.rebuild()

    results = index.search("Status", {"status": "active"}, 5)
    assert [row["id"] for row in results] == ["mem_20260609_001"]


@pytest.mark.parametrize(
    "filters",
    [
        {"status": []},
        {"status": ["active", 1]},
        {"status": 1},
    ],
)
def test_search_rejects_invalid_status_filters(project_root: Path, filters: dict):
    ensure_project_memory(project_root)
    write_card(
        project_root,
        card_data("mem_20260609_001", "YAML storage", "yaml", "src/storage.py"),
    )

    index = MemoryIndex(project_root)
    index.rebuild()

    with pytest.raises(ValueError, match="status"):
        index.search("YAML", filters, 5)
