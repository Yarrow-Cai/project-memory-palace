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
    """
    Write one durable project memory card.

    Use when you have just solved a problem, implemented a feature, fixed a bug,
    produced a design, clarified a project goal, discovered a module workflow, or
    changed an earlier decision.

    You must use this after any result that would help future project understanding.
    Do not use it for temporary command output, low-value debug noise, or secrets.

    Returns the created memory metadata and a user-facing notification payload.
    """
    return _service(project_root).remember(memory)


@mcp.tool()
def recall(
    project_root: str,
    query: str,
    filters: dict[str, Any] | None = None,
    limit: int = 5,
) -> dict[str, Any]:
    """
    Retrieve a small set of relevant memory summaries.

    Use before loading project context or when you need to recover prior decisions
    without flooding the conversation with full memory contents.

    You must use this first when resuming work on a project, checking prior design
    choices, or looking for related bugfixes or conventions.

    This tool returns summaries only. It does not return full card content.
    """
    assert_memory_layout(Path(project_root))
    return {"results": _service(project_root).recall(query, filters or {}, limit)}


@mcp.tool()
def open_memory(project_root: str, id: str) -> dict[str, Any]:
    """
    Open one full memory card by ID.

    Use only after recall finds a likely match and you need the full YAML details.
    Do not use this as the first lookup when a summary is enough.

    This returns the full card for one memory ID.
    """
    assert_memory_layout(Path(project_root))
    return _service(project_root).open_memory(id)


@mcp.tool()
def update_memory(
    project_root: str,
    id: str,
    updates: dict[str, Any],
) -> dict[str, Any]:
    """
    Update an existing memory card.

    Use when a memory becomes stale, superseded, refined, or needs corrected
    status, confidence, tags, or relations.

    You must use this when a prior memory is no longer accurate. Do not use it for
    empty updates or no-op churn.
    """
    assert_memory_layout(Path(project_root))
    return _service(project_root).update_memory(id, updates)


@mcp.tool()
def list_recent(project_root: str, limit: int = 10) -> dict[str, Any]:
    """
    List recently created or updated memory summaries.

    Use when you need a quick view of what was just written or changed in the
    project memory base.

    You must use this after automatic writes if you need to report the latest
    memory activity to the user.

    This returns summaries only.
    """
    assert_memory_layout(Path(project_root))
    return {"results": _service(project_root).list_recent(limit)}


def main() -> None:
    mcp.run()
