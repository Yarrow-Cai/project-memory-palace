from pathlib import Path

import pytest

from pmem.yaml_io import (
    assert_memory_layout,
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


def test_write_card_rejects_collision_without_overwrite(project_root: Path):
    ensure_project_memory(project_root)
    path = write_card(project_root, card_data())

    with pytest.raises(FileExistsError) as error:
        write_card(project_root, card_data())

    assert str(path) in str(error.value)


def test_write_card_overwrites_when_explicit(project_root: Path):
    ensure_project_memory(project_root)
    path = write_card(project_root, card_data())
    data = card_data()
    data["title"] = "Updated YAML storage"

    updated_path = write_card(project_root, data, overwrite=True)
    loaded = read_card(updated_path)

    assert updated_path == path
    assert loaded.to_dict()["title"] == "Updated YAML storage"


def test_discover_cards_returns_sorted_cards(project_root: Path):
    ensure_project_memory(project_root)
    write_card(project_root, card_data("mem_20260609_002"))
    write_card(project_root, card_data("mem_20260609_001"))

    cards = discover_cards(project_root)

    assert [card.id for card in cards] == ["mem_20260609_001", "mem_20260609_002"]


def test_discover_cards_requires_initialized_layout(project_root: Path):
    with pytest.raises(FileNotFoundError) as error:
        discover_cards(project_root)

    assert ".project-memory" in str(error.value)
    assert not (project_root / ".project-memory").exists()


def test_next_card_identity_uses_existing_sequence(project_root: Path):
    ensure_project_memory(project_root)
    write_card(project_root, card_data("mem_20260609_001"))

    card_id, sequence = next_card_identity(project_root, "2026-06-09")

    assert card_id == "mem_20260609_002"
    assert sequence == 2


def test_next_card_identity_rejects_malformed_date(project_root: Path):
    ensure_project_memory(project_root)

    with pytest.raises(ValueError) as error:
        next_card_identity(project_root, "2026-13-09")

    assert "2026-13-09" in str(error.value)


def test_next_card_identity_rejects_sequence_overflow(project_root: Path):
    ensure_project_memory(project_root)
    write_card(project_root, card_data("mem_20260609_999"))

    with pytest.raises(ValueError) as error:
        next_card_identity(project_root, "2026-06-09")

    assert "999" in str(error.value)


def test_read_card_wraps_yaml_errors_with_path(project_root: Path):
    path = project_root / "invalid.yaml"
    path.parent.mkdir(parents=True)
    path.write_text("schema_version: [", encoding="utf-8")

    with pytest.raises(ValueError) as error:
        read_card(path)

    assert str(path) in str(error.value)


def test_read_card_wraps_validation_errors_with_path(project_root: Path):
    path = project_root / "invalid-card.yaml"
    path.parent.mkdir(parents=True)
    path.write_text("schema_version: 1\nid: mem_20260609_001\n", encoding="utf-8")

    with pytest.raises(ValueError) as error:
        read_card(path)

    assert str(path) in str(error.value)
    assert "summary" in str(error.value)


def test_read_card_reports_non_mapping_yaml_with_path(project_root: Path):
    path = project_root / "list-card.yaml"
    path.parent.mkdir(parents=True)
    path.write_text("- not\n- a\n- card\n", encoding="utf-8")

    with pytest.raises(ValueError) as error:
        read_card(path)

    assert str(path) in str(error.value)
    assert "mapping" in str(error.value)


def test_assert_memory_layout_rejects_invalid_path_types(project_root: Path):
    memory_dir = project_root / ".project-memory"
    memory_dir.mkdir(parents=True)
    (memory_dir / "cards").write_text("", encoding="utf-8")
    (memory_dir / "rules").mkdir()
    (memory_dir / "config.yaml").write_text("", encoding="utf-8")
    (memory_dir / "rules" / "agent-rules.yaml").write_text("", encoding="utf-8")

    with pytest.raises(FileNotFoundError) as error:
        assert_memory_layout(project_root)

    assert str(memory_dir / "cards") in str(error.value)
