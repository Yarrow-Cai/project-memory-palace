from __future__ import annotations

from copy import deepcopy
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from pmem.constants import RELATION_KINDS
from pmem.index import MemoryIndex
from pmem.models import MemoryCard
from pmem.yaml_io import discover_cards, ensure_project_memory, next_card_identity, write_card


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
        card_id, _sequence = next_card_identity(self.project_root, now[:10])
        data = self._build_card(card_id, payload, now)
        path = write_card(self.project_root, data)
        card = MemoryCard.from_dict(data)
        self.index.upsert(card)
        return {
            "id": card.id,
            "path": str(path),
            "notification": self._notification(card),
        }

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
        data = deepcopy(self.open_memory(memory_id))
        if "status" in updates:
            data["status"] = updates["status"]
        if "confidence" in updates:
            data["confidence"] = updates["confidence"]
        if "tags" in updates:
            data["tags"] = updates["tags"]
        if "relations" in updates:
            relations = data.setdefault("relations", self._empty_relations())
            for relation, targets in updates["relations"].items():
                if relation not in RELATION_KINDS:
                    continue
                current = list(relations.get(relation, []))
                for target in targets:
                    if target not in current:
                        current.append(target)
                relations[relation] = current
        data["updated_at"] = self._now()
        write_card(self.project_root, data, overwrite=True)
        card = MemoryCard.from_dict(data)
        self.index.upsert(card)
        return card.to_dict()

    def rebuild_index(self) -> None:
        self.init_project()
        self.index.rebuild()

    def _build_card(
        self, card_id: str, payload: dict[str, Any], now: str
    ) -> dict[str, Any]:
        source = deepcopy(payload.get("source"))
        confidence = float(payload.get("confidence", 0.5))
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

        scope = deepcopy(payload.get("scope")) or {}
        scope.setdefault("project", "")
        scope.setdefault("modules", [])
        scope.setdefault("paths", [])

        relations = self._empty_relations()
        for relation, targets in (payload.get("relations") or {}).items():
            if relation in relations:
                relations[relation] = list(targets)

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
            "tags": list(payload.get("tags", [])),
            "relations": relations,
            "created_at": now,
            "updated_at": now,
        }

    def _empty_relations(self) -> dict[str, list[str]]:
        return {relation: [] for relation in RELATION_KINDS}

    def _now(self) -> str:
        return datetime.now(timezone.utc).astimezone().isoformat(timespec="seconds")

    def _notification(self, card: MemoryCard) -> str:
        data = card.to_dict()
        return (
            "Project memory written:\n"
            f"- ID: {data['id']}\n"
            f"- Type: {data['type']}\n"
            f"- Summary: {data['summary']}\n"
            f"- Source: {data['source']['kind']} - {data['source']['description']}\n"
            "- Future use: use recall to retrieve this summary, then open_memory for details."
        )
