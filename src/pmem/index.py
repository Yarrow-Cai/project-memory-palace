from __future__ import annotations

from collections.abc import Iterator
from contextlib import contextmanager
import json
import re
import sqlite3
from pathlib import Path
from typing import Any

from pmem.models import MemoryCard
from pmem.paths import index_path
from pmem.yaml_io import discover_cards, ensure_project_memory


FTS_TOKEN_RE = re.compile(r"[^\W_]+", re.UNICODE)


def _plain_text_fts_query(query: str) -> str | None:
    terms = FTS_TOKEN_RE.findall(query)
    if not terms:
        return None
    return " ".join('"' + term.replace('"', '""') + '"' for term in terms)


def _normalize_status_filter(filters: dict[str, Any]) -> list[str]:
    if "status" not in filters:
        return ["active"]

    raw_status = filters["status"]
    if isinstance(raw_status, str):
        statuses = [raw_status]
    elif isinstance(raw_status, list):
        statuses = raw_status
    else:
        raise ValueError("status filter must be a string or a non-empty list of strings")

    if not statuses:
        raise ValueError("status filter must be a non-empty list of strings")
    if any(not isinstance(status, str) or not status for status in statuses):
        raise ValueError("status filter must contain only non-empty strings")
    return statuses


class MemoryIndex:
    def __init__(self, project_root: Path):
        self.project_root = project_root
        ensure_project_memory(project_root)
        self.path = index_path(project_root)

    def connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(self.path)
        connection.row_factory = sqlite3.Row
        return connection

    @contextmanager
    def _connection(self) -> Iterator[sqlite3.Connection]:
        connection = self.connect()
        try:
            yield connection
        except Exception:
            connection.rollback()
            raise
        else:
            connection.commit()
        finally:
            connection.close()

    def initialize(self) -> None:
        with self._connection() as connection:
            self._initialize(connection)

    def _initialize(self, connection: sqlite3.Connection) -> None:
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

            CREATE TABLE IF NOT EXISTS memory_paths (
                memory_id TEXT NOT NULL,
                path TEXT NOT NULL,
                PRIMARY KEY (memory_id, path)
            );

            CREATE INDEX IF NOT EXISTS idx_memory_paths_path
                ON memory_paths(path);
            """
        )

    def clear(self) -> None:
        with self._connection() as connection:
            self._initialize(connection)
            self._clear(connection)

    def _clear(self, connection: sqlite3.Connection) -> None:
        connection.execute("DELETE FROM memory_paths")
        connection.execute("DELETE FROM relations")
        connection.execute("DELETE FROM memory_fts")
        connection.execute("DELETE FROM memories")

    def rebuild(self) -> None:
        cards = discover_cards(self.project_root)
        with self._connection() as connection:
            self._initialize(connection)
            self._clear(connection)
            for card in cards:
                self._upsert(connection, card)

    def upsert(self, card: MemoryCard) -> None:
        with self._connection() as connection:
            self._initialize(connection)
            self._upsert(connection, card)

    def _upsert(self, connection: sqlite3.Connection, card: MemoryCard) -> None:
        data = card.to_dict()
        tags = data["tags"]
        modules = data["scope"].get("modules", [])
        paths = data["scope"].get("paths", [])
        source_kind = data["source"]["kind"]
        relations = data["relations"]

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
        connection.execute("DELETE FROM memory_paths WHERE memory_id = ?", (data["id"],))
        for path in paths:
            connection.execute(
                "INSERT OR IGNORE INTO memory_paths (memory_id, path) VALUES (?, ?)",
                (data["id"], path),
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
        fts_query = _plain_text_fts_query(query)
        status_filter = _normalize_status_filter(filters)
        if fts_query is None:
            return []

        path_filter = filters.get("paths") or []
        params: list[Any] = [fts_query]
        where = ["m.status IN ({})".format(",".join("?" for _ in status_filter))]
        params.extend(status_filter)
        if path_filter:
            path_placeholders = ",".join("?" for _ in path_filter)
            where.append(
                f"""
                EXISTS (
                    SELECT 1
                    FROM memory_paths mp
                    WHERE mp.memory_id = m.id
                      AND mp.path IN ({path_placeholders})
                )
                """
            )
            params.extend(path_filter)
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
        with self._connection() as connection:
            rows = connection.execute(sql, params).fetchall()
        return [self._summary_row(row, ["fts"]) for row in rows]

    def recent(self, limit: int) -> list[dict[str, Any]]:
        self.initialize()
        with self._connection() as connection:
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
