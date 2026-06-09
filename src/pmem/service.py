from __future__ import annotations

from collections.abc import Mapping
from copy import deepcopy
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from pmem.constants import RELATION_KINDS
from pmem.index import MemoryIndex
from pmem.models import MemoryCard
from pmem.yaml_io import discover_cards, ensure_project_memory, next_card_identity, write_card


REMEMBER_ID_WRITE_ATTEMPTS = 3


class MemoryNotFoundError(KeyError):
    """Raised when a memory id is not found."""


class MemoryService:
    def __init__(self, project_root: Path):
        self.project_root = project_root
        self.index = MemoryIndex(project_root)

    def init_project(self) -> None:
        ensure_project_memory(self.project_root)
        self.index.initialize()

    def remember(self, payload: dict[str, Any]) -> dict[str, Any]:
        self.init_project()
        now = self._now()
        last_error: FileExistsError | None = None
        for _attempt in range(REMEMBER_ID_WRITE_ATTEMPTS):
            card_id, _sequence = next_card_identity(self.project_root, now[:10])
            data = self._build_card(card_id, payload, now)
            try:
                path = write_card(self.project_root, data)
            except FileExistsError as error:
                last_error = error
                continue

            card = MemoryCard.from_dict(data)
            try:
                self.index.upsert(card)
            except Exception as error:
                try:
                    path.unlink()
                except OSError as cleanup_error:
                    error.add_note(
                        f"failed to remove written memory card {path}: {cleanup_error}"
                    )
                raise
            return {
                "id": card.id,
                "path": str(path),
                "notification": self._notification(card),
            }

        raise RuntimeError(
            f"failed to allocate unique memory id after {REMEMBER_ID_WRITE_ATTEMPTS} attempts"
        ) from last_error

    def recall(
        self, query: str, filters: dict[str, Any] | None, limit: int = 5
    ) -> list[dict[str, Any]]:
        self.init_project()
        return self.index.search(query, filters or {}, limit)

    def open_memory(self, memory_id: str) -> dict[str, Any]:
        for card in discover_cards(self.project_root):
            if card.id == memory_id:
                return card.to_dict()
        raise MemoryNotFoundError(f"memory not found: {memory_id}")

    def list_recent(self, limit: int = 10) -> list[dict[str, Any]]:
        self.init_project()
        return self.index.recent(limit)

    def update_memory(self, memory_id: str, updates: dict[str, Any]) -> dict[str, Any]:
        existing = self.open_memory(memory_id)
        data = deepcopy(existing)
        if "status" in updates:
            data["status"] = updates["status"]
        if "confidence" in updates:
            data["confidence"] = self._validate_confidence(updates["confidence"])
        if "tags" in updates:
            data["tags"] = self._validate_string_list(updates["tags"], "tags")
        if "relations" in updates:
            relations = data.setdefault("relations", self._empty_relations())
            for relation, targets in self._validate_relation_items(
                updates["relations"]
            ).items():
                current = list(relations.get(relation, []))
                for target in targets:
                    if target not in current:
                        current.append(target)
                relations[relation] = current
        data["updated_at"] = self._now()
        write_card(self.project_root, data, overwrite=True)
        card = MemoryCard.from_dict(data)
        try:
            self.index.upsert(card)
        except Exception:
            write_card(self.project_root, existing, overwrite=True)
            raise
        return card.to_dict()

    def rebuild_index(self) -> None:
        self.init_project()
        self.index.rebuild()

    def _build_card(
        self, card_id: str, payload: dict[str, Any], now: str
    ) -> dict[str, Any]:
        source = self._source_from_payload(payload)
        confidence = self._validate_confidence(payload.get("confidence", 0.5))
        if not source:
            source = {
                "kind": "analysis",
                "description": "Source was not supplied by caller.",
                "files": [],
                "commits": [],
            }
            confidence = min(confidence, 0.5)
        source.setdefault("files", [])
        source.setdefault("commits", [])

        scope = self._scope_from_payload(payload)
        scope.setdefault("project", "")
        scope.setdefault("modules", [])
        scope.setdefault("paths", [])

        relations = self._empty_relations()
        if payload.get("relations") is not None:
            for relation, targets in self._validate_relation_items(
                payload["relations"]
            ).items():
                relations[relation] = targets

        return {
            "schema_version": 1,
            "id": card_id,
            "type": payload["type"],
            "status": payload.get("status", "active"),
            "confidence": confidence,
            "title": payload["title"],
            "summary": payload["summary"],
            "content": payload["content"],
            "source": source,
            "scope": scope,
            "tags": self._validate_string_list(payload.get("tags", []), "tags"),
            "relations": relations,
            "created_at": now,
            "updated_at": now,
        }

    def _source_from_payload(self, payload: dict[str, Any]) -> dict[str, Any] | None:
        source = payload.get("source")
        if source is None:
            return None
        if not isinstance(source, Mapping):
            raise ValueError("source must be a mapping")
        return dict(deepcopy(source))

    def _scope_from_payload(self, payload: dict[str, Any]) -> dict[str, Any]:
        scope = payload.get("scope")
        if scope is None:
            return {}
        if not isinstance(scope, Mapping):
            raise ValueError("scope must be a mapping")
        return dict(deepcopy(scope))

    def _validate_confidence(self, value: Any) -> float:
        if isinstance(value, bool) or not isinstance(value, int | float):
            raise ValueError("confidence must be an int or float")
        confidence = float(value)
        if not 0.0 <= confidence <= 1.0:
            raise ValueError("confidence must be between 0.0 and 1.0")
        return confidence

    def _validate_string_list(self, value: Any, field_name: str) -> list[str]:
        if not isinstance(value, list) or any(
            not isinstance(item, str) for item in value
        ):
            raise ValueError(f"{field_name} must be a list of strings")
        return list(value)

    def _validate_relation_items(self, value: Any) -> dict[str, list[str]]:
        if not isinstance(value, Mapping):
            raise ValueError("relations must be a mapping")

        relations: dict[str, list[str]] = {}
        for relation, targets in value.items():
            if relation not in RELATION_KINDS:
                raise ValueError(f"unknown relation: {relation}")
            relations[relation] = self._validate_string_list(
                targets, f"relations.{relation}"
            )
        return relations

    def _empty_relations(self) -> dict[str, list[str]]:
        return {relation: [] for relation in RELATION_KINDS}

    def _now(self) -> str:
        return datetime.now(timezone.utc).astimezone().isoformat(timespec="seconds")

    def _notification(self, card: MemoryCard) -> str:
        data = card.to_dict()
        lines = [
            "Project memory written:",
            f"- ID: {data['id']}",
            f"- Type: {data['type']}",
            f"- Summary: {data['summary']}",
            f"- Source: {data['source']['kind']} - {data['source']['description']}",
        ]
        supersedes = data["relations"].get("supersedes", [])
        superseded_by = data["relations"].get("superseded_by", [])
        if supersedes:
            lines.append(f"- Supersedes: {', '.join(supersedes)}")
        if superseded_by:
            lines.append(f"- Superseded by: {', '.join(superseded_by)}")
        lines.append(
            "- Future use: use recall to retrieve this summary, then open_memory for details."
        )
        return "\n".join(lines)
