from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import yaml

from pmem.audit import audit_project
from pmem.service import MemoryNotFoundError, MemoryService
from pmem.yaml_io import assert_memory_layout


def _positive_int(value: str) -> int:
    try:
        parsed = int(value)
    except ValueError as error:
        raise argparse.ArgumentTypeError(
            "limit must be a positive integer"
        ) from error
    if parsed <= 0:
        raise argparse.ArgumentTypeError("limit must be a positive integer")
    return parsed


def _load_remember_payload(path: Path) -> dict:
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as error:
        raise ValueError(f"unable to read YAML file {path}: {error}") from error
    try:
        payload = yaml.safe_load(text)
    except yaml.YAMLError as error:
        raise ValueError(f"invalid YAML file {path}: {error}") from error
    if payload is None:
        raise ValueError(f"invalid YAML file {path}: card file is empty")
    if not isinstance(payload, dict):
        raise ValueError(
            f"invalid YAML file {path}: card file must contain a mapping"
        )
    return payload


def _error_message(error: Exception) -> str:
    if isinstance(error, MemoryNotFoundError) and error.args:
        return str(error.args[0])
    return str(error)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="pmem")
    parser.add_argument(
        "--project-root",
        default=".",
        help="Project root containing .project-memory",
    )
    subcommands = parser.add_subparsers(dest="command", required=True)

    subcommands.add_parser("init")

    remember = subcommands.add_parser("remember")
    remember.add_argument("--file", required=True)

    search = subcommands.add_parser("search")
    search.add_argument("query")
    search.add_argument("--limit", type=_positive_int, default=5)

    open_cmd = subcommands.add_parser("open")
    open_cmd.add_argument("id")

    recent = subcommands.add_parser("recent")
    recent.add_argument("--limit", type=_positive_int, default=10)

    update = subcommands.add_parser("update")
    update.add_argument("id")
    update.add_argument("--status")
    update.add_argument("--confidence", type=float)

    subcommands.add_parser("rebuild-index")
    subcommands.add_parser("audit")
    return parser


def run(argv: list[str] | None = None) -> int:
    try:
        args = build_parser().parse_args(argv)
    except SystemExit as error:
        return error.code if isinstance(error.code, int) else 2

    project_root = Path(args.project_root)
    try:
        if args.command == "init":
            service = MemoryService(project_root)
            service.init_project()
            print("Initialized project memory.")
        elif args.command == "remember":
            payload = _load_remember_payload(Path(args.file))
            service = MemoryService(project_root)
            result = service.remember(payload)
            print(result["notification"])
        elif args.command == "search":
            assert_memory_layout(project_root)
            service = MemoryService(project_root)
            results = service.recall(args.query, {}, args.limit)
            print(
                yaml.safe_dump(
                    {"results": results},
                    sort_keys=False,
                    allow_unicode=True,
                )
            )
        elif args.command == "open":
            assert_memory_layout(project_root)
            service = MemoryService(project_root)
            print(
                yaml.safe_dump(
                    service.open_memory(args.id),
                    sort_keys=False,
                    allow_unicode=True,
                )
            )
        elif args.command == "recent":
            assert_memory_layout(project_root)
            service = MemoryService(project_root)
            print(
                yaml.safe_dump(
                    {"results": service.list_recent(args.limit)},
                    sort_keys=False,
                    allow_unicode=True,
                )
            )
        elif args.command == "update":
            updates = {}
            if args.status is not None:
                updates["status"] = args.status
            if args.confidence is not None:
                updates["confidence"] = args.confidence
            if not updates:
                raise ValueError("update requires at least one update flag")
            assert_memory_layout(project_root)
            service = MemoryService(project_root)
            print(
                yaml.safe_dump(
                    service.update_memory(args.id, updates),
                    sort_keys=False,
                    allow_unicode=True,
                )
            )
        elif args.command == "rebuild-index":
            assert_memory_layout(project_root)
            service = MemoryService(project_root)
            service.rebuild_index()
            print("Rebuilt memory index.")
        elif args.command == "audit":
            assert_memory_layout(project_root)
            print(json.dumps({"issues": audit_project(project_root)}, indent=2))
        return 0
    except (OSError, MemoryNotFoundError, ValueError) as error:
        print(f"error: {_error_message(error)}", file=sys.stderr)
        return 1


def main() -> None:
    raise SystemExit(run())
