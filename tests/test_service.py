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
