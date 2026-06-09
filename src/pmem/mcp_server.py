from __future__ import annotations

from pathlib import Path
from typing import Any

from mcp.server.fastmcp import FastMCP

from pmem.service import MemoryService
from pmem.yaml_io import assert_memory_layout


mcp = FastMCP("project-memory-palace")


def _service(project_root: str) -> MemoryService:
    return MemoryService(Path(project_root))


@mcp.tool()
def remember(project_root: str, memory: dict[str, Any]) -> dict[str, Any]:
    """Write a project memory card and return the user notification payload."""
    return _service(project_root).remember(memory)


@mcp.tool()
def recall(
    project_root: str,
    query: str,
    filters: dict[str, Any] | None = None,
    limit: int = 5,
) -> dict[str, Any]:
    """Return relevant memory summaries. Full content is not returned."""
    assert_memory_layout(Path(project_root))
    return {"results": _service(project_root).recall(query, filters or {}, limit)}


@mcp.tool()
def open_memory(project_root: str, id: str) -> dict[str, Any]:
    """Open one full memory card by ID."""
    assert_memory_layout(Path(project_root))
    return _service(project_root).open_memory(id)


@mcp.tool()
def update_memory(
    project_root: str,
    id: str,
    updates: dict[str, Any],
) -> dict[str, Any]:
    """Update memory status, relations, tags, or confidence."""
    assert_memory_layout(Path(project_root))
    return _service(project_root).update_memory(id, updates)


@mcp.tool()
def list_recent(project_root: str, limit: int = 10) -> dict[str, Any]:
    """List recently created or updated memory summaries."""
    assert_memory_layout(Path(project_root))
    return {"results": _service(project_root).list_recent(limit)}


def main() -> None:
    mcp.run()
