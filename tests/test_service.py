from copy import deepcopy
from pathlib import Path

import pytest

import pmem.service as service_module
from pmem.service import MemoryService
from pmem.yaml_io import write_card as real_write_card


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


@pytest.mark.parametrize("confidence", [True, False])
def test_remember_rejects_bool_confidence(project_root: Path, confidence: bool):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload["confidence"] = confidence

    with pytest.raises(ValueError, match="confidence"):
        service.remember(payload)


def test_remember_rejects_string_confidence(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload["confidence"] = "0.9"

    with pytest.raises(ValueError, match="confidence"):
        service.remember(payload)


def test_remember_rejects_confidence_above_one(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload["confidence"] = 2.0

    with pytest.raises(ValueError, match="confidence"):
        service.remember(payload)


def test_remember_rejects_confidence_below_zero(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload["confidence"] = -0.1

    with pytest.raises(ValueError, match="confidence"):
        service.remember(payload)


def test_remember_rejects_out_of_range_confidence_without_source(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload.pop("source")
    payload["confidence"] = 2.0

    with pytest.raises(ValueError, match="confidence"):
        service.remember(payload)


def test_remember_rejects_string_tags(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload["tags"] = "yaml"

    with pytest.raises(ValueError, match="tags"):
        service.remember(payload)


def test_remember_rejects_string_relation_targets(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload["relations"]["supersedes"] = "mem_20260609_001"

    with pytest.raises(ValueError, match="relations.supersedes"):
        service.remember(payload)


def test_update_memory_rejects_unknown_relation_key(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(remember_input())

    with pytest.raises(ValueError, match="unknown_relation"):
        service.update_memory(
            created["id"],
            {"relations": {"unknown_relation": ["mem_20260609_999"]}},
        )


def test_update_memory_rejects_string_relation_targets(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(remember_input())

    with pytest.raises(ValueError, match="relations.supersedes"):
        service.update_memory(
            created["id"],
            {"relations": {"supersedes": "mem_20260609_999"}},
        )


def test_update_memory_rejects_non_mapping_relations(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(remember_input())

    with pytest.raises(ValueError, match="relations"):
        service.update_memory(created["id"], {"relations": ["mem_20260609_999"]})


def test_remember_retries_id_collision(
    project_root: Path, monkeypatch: pytest.MonkeyPatch
):
    service = MemoryService(project_root)
    service.init_project()
    calls = {"count": 0}

    def write_card_with_one_collision(
        project_root: Path, data: dict, overwrite: bool = False
    ):
        calls["count"] += 1
        if calls["count"] == 1:
            real_write_card(project_root, data, overwrite=overwrite)
            raise FileExistsError("memory card already exists")
        return real_write_card(project_root, data, overwrite=overwrite)

    monkeypatch.setattr(service_module, "write_card", write_card_with_one_collision)

    result = service.remember(remember_input())

    assert calls["count"] == 2
    assert result["id"].endswith("_002")
    assert service.open_memory(result["id"])["title"] == "YAML storage"


def test_update_memory_rolls_back_yaml_when_index_upsert_fails(
    project_root: Path, monkeypatch: pytest.MonkeyPatch
):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(remember_input())

    def fail_upsert(_card):
        raise RuntimeError("index unavailable")

    monkeypatch.setattr(service.index, "upsert", fail_upsert)

    with pytest.raises(RuntimeError, match="index unavailable"):
        service.update_memory(created["id"], {"status": "stale"})

    assert service.open_memory(created["id"])["status"] == "active"


def test_remember_notification_includes_superseded_relation(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload["relations"]["supersedes"] = ["mem_20260609_001"]

    result = service.remember(payload)

    assert "mem_20260609_001" in result["notification"]
    assert "Supersedes" in result["notification"]


def test_remember_does_not_mutate_payload(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    payload = remember_input()
    payload["relations"]["superseded_by"] = ["mem_20260609_999"]
    original = deepcopy(payload)

    service.remember(payload)

    assert payload == original
