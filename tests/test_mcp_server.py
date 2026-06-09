from pathlib import Path

import pytest

import pmem.mcp_server as mcp_server


def memory_payload() -> dict:
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


def assert_memory_not_created(project_root: Path):
    assert not (project_root / ".project-memory").exists()


def test_mcp_server_imports_with_registered_tools():
    assert mcp_server.mcp.name == "project-memory-palace"
    assert set(mcp_server.mcp._tool_manager._tools) == {
        "remember",
        "recall",
        "open_memory",
        "update_memory",
        "list_recent",
    }


def test_mcp_tools_happy_path(project_root: Path):
    created = mcp_server.remember(str(project_root), memory_payload())

    recalled = mcp_server.recall(str(project_root), "YAML")
    opened = mcp_server.open_memory(str(project_root), created["id"])
    updated = mcp_server.update_memory(
        str(project_root),
        created["id"],
        {"status": "stale"},
    )
    recent = mcp_server.list_recent(str(project_root), limit=1)

    assert created["id"].startswith("mem_")
    assert recalled["results"][0]["title"] == "YAML storage"
    assert "content" not in recalled["results"][0]
    assert opened["content"] == "SQLite is only an index."
    assert updated["status"] == "stale"
    assert recent["results"][0]["id"] == created["id"]


@pytest.mark.parametrize(
    ("tool", "args"),
    [
        (mcp_server.recall, ("YAML",)),
        (mcp_server.open_memory, ("mem_20990101_999",)),
        (mcp_server.update_memory, ("mem_20990101_999", {"status": "stale"})),
        (mcp_server.list_recent, ()),
    ],
)
def test_mcp_read_and_update_tools_do_not_initialize_project(
    project_root: Path,
    tool,
    args: tuple,
):
    with pytest.raises(FileNotFoundError):
        tool(str(project_root), *args)

    assert_memory_not_created(project_root)


@pytest.mark.parametrize("limit", [0, -1])
def test_mcp_recall_rejects_non_positive_limit(project_root: Path, limit: int):
    mcp_server.remember(str(project_root), memory_payload())

    with pytest.raises(ValueError, match="limit"):
        mcp_server.recall(str(project_root), "YAML", limit=limit)


@pytest.mark.parametrize("limit", [0, -1])
def test_mcp_list_recent_rejects_non_positive_limit(
    project_root: Path,
    limit: int,
):
    mcp_server.remember(str(project_root), memory_payload())

    with pytest.raises(ValueError, match="limit"):
        mcp_server.list_recent(str(project_root), limit=limit)
