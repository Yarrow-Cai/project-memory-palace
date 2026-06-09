from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from pmem.constants import (
    MEMORY_STATUSES,
    MEMORY_TYPES,
    RELATION_KINDS,
    REQUIRED_FIELDS,
    SOURCE_KINDS,
)


class ValidationError(ValueError):
    """Raised when a memory card does not match the required schema."""


def _require_mapping(value: Any, field_name: str) -> dict:
    if not isinstance(value, dict):
        raise ValidationError(f"{field_name} must be a mapping")
    return value


def _require_list(value: Any, field_name: str) -> list:
    if not isinstance(value, list):
        raise ValidationError(f"{field_name} must be a list")
    return value


def validate_card_data(data: dict[str, Any]) -> None:
    missing = sorted(REQUIRED_FIELDS.difference(data))
    if missing:
        raise ValidationError(f"missing required fields: {', '.join(missing)}")

    if data["schema_version"] != 1:
        raise ValidationError("schema_version must be 1")

    if data["type"] not in MEMORY_TYPES:
        raise ValidationError(f"type must be one of: {', '.join(sorted(MEMORY_TYPES))}")

    if data["status"] not in MEMORY_STATUSES:
        raise ValidationError(f"status must be one of: {', '.join(sorted(MEMORY_STATUSES))}")

    confidence = data["confidence"]
    if not isinstance(confidence, int | float) or not 0.0 <= float(confidence) <= 1.0:
        raise ValidationError("confidence must be a number between 0.0 and 1.0")

    for field_name in ("id", "title", "summary", "content", "created_at", "updated_at"):
        if not isinstance(data[field_name], str) or not data[field_name].strip():
            raise ValidationError(f"{field_name} must be a non-empty string")

    source = _require_mapping(data["source"], "source")
    if source.get("kind") not in SOURCE_KINDS:
        raise ValidationError(f"source.kind must be one of: {', '.join(sorted(SOURCE_KINDS))}")
    if not isinstance(source.get("description"), str) or not source["description"].strip():
        raise ValidationError("source.description must be a non-empty string")
    _require_list(source.get("files", []), "source.files")
    _require_list(source.get("commits", []), "source.commits")

    scope = _require_mapping(data["scope"], "scope")
    if not isinstance(scope.get("project"), str):
        raise ValidationError("scope.project must be a string")
    _require_list(scope.get("modules", []), "scope.modules")
    _require_list(scope.get("paths", []), "scope.paths")

    _require_list(data["tags"], "tags")

    relations = _require_mapping(data["relations"], "relations")
    for relation in RELATION_KINDS:
        _require_list(relations.get(relation, []), f"relations.{relation}")


@dataclass(frozen=True)
class MemoryCard:
    data: dict[str, Any]

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "MemoryCard":
        validate_card_data(data)
        return cls(data=dict(data))

    @property
    def id(self) -> str:
        return self.data["id"]

    @property
    def type(self) -> str:
        return self.data["type"]

    @property
    def status(self) -> str:
        return self.data["status"]

    @property
    def updated_at(self) -> str:
        return self.data["updated_at"]

    def to_dict(self) -> dict[str, Any]:
        return dict(self.data)
