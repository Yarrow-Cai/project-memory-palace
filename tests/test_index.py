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
