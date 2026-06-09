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


@pytest.mark.parametrize("confidence", [True, False])
def test_validate_card_data_rejects_bool_confidence(confidence):
    data = valid_card_data()
    data["confidence"] = confidence

    with pytest.raises(ValidationError) as error:
        validate_card_data(data)

    assert "confidence" in str(error.value)


@pytest.mark.parametrize(
    ("field_path", "keys", "bad_value"),
    [
        ("source.files", ("source", "files"), 123),
        ("source.commits", ("source", "commits"), None),
        ("scope.modules", ("scope", "modules"), 123),
        ("scope.paths", ("scope", "paths"), None),
        ("tags", ("tags",), 123),
        ("relations.supersedes", ("relations", "supersedes"), 123),
        ("relations.superseded_by", ("relations", "superseded_by"), None),
        ("relations.related_to", ("relations", "related_to"), 123),
        ("relations.explains", ("relations", "explains"), None),
        ("relations.caused_by", ("relations", "caused_by"), 123),
    ],
)
def test_validate_card_data_rejects_non_string_list_items(field_path, keys, bad_value):
    data = valid_card_data()
    target = data
    for key in keys[:-1]:
        target = target[key]
    target[keys[-1]] = [bad_value]

    with pytest.raises(ValidationError) as error:
        validate_card_data(data)

    assert field_path in str(error.value)


def test_validate_card_data_rejects_non_mapping_card():
    with pytest.raises(ValidationError) as error:
        validate_card_data(None)

    assert "card" in str(error.value)


def test_memory_card_round_trips_to_dict():
    card = MemoryCard.from_dict(valid_card_data())

    assert card.id == "mem_20260609_001"
    assert card.to_dict()["relations"]["supersedes"] == []


def test_memory_card_does_not_share_nested_input_data():
    data = valid_card_data()
    card = MemoryCard.from_dict(data)

    data["relations"]["supersedes"].append("mem_old")

    assert card.to_dict()["relations"]["supersedes"] == []


def test_memory_card_to_dict_returns_deep_copy():
    card = MemoryCard.from_dict(valid_card_data())
    data = card.to_dict()

    data["relations"]["supersedes"].append("mem_old")

    assert card.to_dict()["relations"]["supersedes"] == []
