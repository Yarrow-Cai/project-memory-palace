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
