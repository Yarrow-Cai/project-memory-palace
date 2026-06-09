from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any

from pmem.models import MemoryCard
from pmem.paths import index_path
from pmem.yaml_io import discover_cards, ensure_project_memory


class MemoryIndex:
    def __init__(self, project_root: Path):
        self.project_root = project_root
        ensure_project_memory(project_root)
        self.path = index_path(project_root)

    def connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(self.path)
        connection.row_factory = sqlite3.Row
        return connection

    def initialize(self) -> None:
        with self.connect() as connection:
            connection.executescript(
                """
                CREATE TABLE IF NOT EXISTS memories (
                    id TEXT PRIMARY KEY,
                    type TEXT NOT NULL,
                    status TEXT NOT NULL,
                    title TEXT NOT NULL,
                    summary TEXT NOT NULL,
                    source_kind TEXT NOT NULL,
                    confidence REAL NOT NULL,
                    tags_json TEXT NOT NULL,
                    modules_json TEXT NOT NULL,
                    paths_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );

                CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
                    id UNINDEXED,
                    title,
                    summary,
                    content,
                    tags,
                    modules,
                    paths,
                    tokenize='unicode61'
                );

                CREATE TABLE IF NOT EXISTS relations (
                    source_id TEXT NOT NULL,
                    relation TEXT NOT NULL,
                    target_id TEXT NOT NULL,
                    PRIMARY KEY (source_id, relation, target_id)
                );
                """
            )

    def clear(self) -> None:
        self.initialize()
        with self.connect() as connection:
            connection.execute("DELETE FROM relations")
            connection.execute("DELETE FROM memory_fts")
            connection.execute("DELETE FROM memories")

    def rebuild(self) -> None:
        self.clear()
        for card in discover_cards(self.project_root):
            self.upsert(card)

    def upsert(self, card: MemoryCard) -> None:
        self.initialize()
        data = card.to_dict()
        tags = data["tags"]
        modules = data["scope"].get("modules", [])
        paths = data["scope"].get("paths", [])
        source_kind = data["source"]["kind"]
        relations = data["relations"]
        with self.connect() as connection:
            connection.execute(
                """
                INSERT INTO memories (
                    id, type, status, title, summary, source_kind, confidence,
                    tags_json, modules_json, paths_json, updated_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT(id) DO UPDATE SET
                    type=excluded.type,
                    status=excluded.status,
                    title=excluded.title,
                    summary=excluded.summary,
                    source_kind=excluded.source_kind,
                    confidence=excluded.confidence,
                    tags_json=excluded.tags_json,
                    modules_json=excluded.modules_json,
                    paths_json=excluded.paths_json,
                    updated_at=excluded.updated_at
                """,
                (
                    data["id"],
                    data["type"],
                    data["status"],
                    data["title"],
                    data["summary"],
                    source_kind,
                    data["confidence"],
                    json.dumps(tags),
                    json.dumps(modules),
                    json.dumps(paths),
                    data["updated_at"],
                ),
            )
            connection.execute("DELETE FROM memory_fts WHERE id = ?", (data["id"],))
            connection.execute(
                """
                INSERT INTO memory_fts (id, title, summary, content, tags, modules, paths)
                VALUES (?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    data["id"],
                    data["title"],
                    data["summary"],
                    data["content"],
                    " ".join(tags),
                    " ".join(modules),
                    " ".join(paths),
                ),
            )
            connection.execute("DELETE FROM relations WHERE source_id = ?", (data["id"],))
            for relation, targets in relations.items():
                for target in targets:
                    connection.execute(
                        "INSERT OR IGNORE INTO relations (source_id, relation, target_id) VALUES (?, ?, ?)",
                        (data["id"], relation, target),
                    )

    def search(self, query: str, filters: dict[str, Any], limit: int) -> list[dict[str, Any]]:
        self.initialize()
        status_filter = filters.get("status") or ["active"]
        path_filter = filters.get("paths") or []
        params: list[Any] = [query]
        where = ["m.status IN ({})".format(",".join("?" for _ in status_filter))]
        params.extend(status_filter)
        if path_filter:
            path_checks = []
            for path in path_filter:
                path_checks.append("m.paths_json LIKE ?")
                params.append(f"%{path}%")
            where.append("(" + " OR ".join(path_checks) + ")")
        params.append(limit)
        sql = f"""
            SELECT
                m.id, m.type, m.status, m.title, m.summary, m.source_kind,
                m.confidence, m.updated_at, bm25(memory_fts) AS rank
            FROM memory_fts
            JOIN memories m ON m.id = memory_fts.id
            WHERE memory_fts MATCH ?
              AND {' AND '.join(where)}
            ORDER BY rank ASC, m.updated_at DESC
            LIMIT ?
        """
        with self.connect() as connection:
            rows = connection.execute(sql, params).fetchall()
        return [self._summary_row(row, ["fts"]) for row in rows]

    def recent(self, limit: int) -> list[dict[str, Any]]:
        self.initialize()
        with self.connect() as connection:
            rows = connection.execute(
                """
                SELECT id, type, status, title, summary, source_kind, confidence, updated_at
                FROM memories
                ORDER BY updated_at DESC
                LIMIT ?
                """,
                (limit,),
            ).fetchall()
        return [self._summary_row(row, ["recent"]) for row in rows]

    def _summary_row(self, row: sqlite3.Row, matched_by: list[str]) -> dict[str, Any]:
        return {
            "id": row["id"],
            "type": row["type"],
            "status": row["status"],
            "title": row["title"],
            "summary": row["summary"],
            "confidence": row["confidence"],
            "source_hint": row["source_kind"],
            "matched_by": matched_by,
            "updated_at": row["updated_at"],
        }
