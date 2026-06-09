from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import yaml

from pmem.audit import audit_project
from pmem.service import MemoryNotFoundError, MemoryService


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
    search.add_argument("--limit", type=int, default=5)

    open_cmd = subcommands.add_parser("open")
    open_cmd.add_argument("id")

    recent = subcommands.add_parser("recent")
    recent.add_argument("--limit", type=int, default=10)

    update = subcommands.add_parser("update")
    update.add_argument("id")
    update.add_argument("--status")
    update.add_argument("--confidence", type=float)

    subcommands.add_parser("rebuild-index")
    subcommands.add_parser("audit")
    return parser


def run(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    project_root = Path(args.project_root)
    service = MemoryService(project_root)
    try:
        if args.command == "init":
            service.init_project()
            print("Initialized project memory.")
        elif args.command == "remember":
            payload = yaml.safe_load(Path(args.file).read_text(encoding="utf-8"))
            result = service.remember(payload)
            print(result["notification"])
        elif args.command == "search":
            results = service.recall(args.query, {}, args.limit)
            print(
                yaml.safe_dump(
                    {"results": results},
                    sort_keys=False,
                    allow_unicode=True,
                )
            )
        elif args.command == "open":
            print(
                yaml.safe_dump(
                    service.open_memory(args.id),
                    sort_keys=False,
                    allow_unicode=True,
                )
            )
        elif args.command == "recent":
            print(
                yaml.safe_dump(
                    {"results": service.list_recent(args.limit)},
                    sort_keys=False,
                    allow_unicode=True,
                )
            )
        elif args.command == "update":
            updates = {}
            if args.status:
                updates["status"] = args.status
            if args.confidence is not None:
                updates["confidence"] = args.confidence
            print(
                yaml.safe_dump(
                    service.update_memory(args.id, updates),
                    sort_keys=False,
                    allow_unicode=True,
                )
            )
        elif args.command == "rebuild-index":
            service.rebuild_index()
            print("Rebuilt memory index.")
        elif args.command == "audit":
            print(json.dumps({"issues": audit_project(project_root)}, indent=2))
        return 0
    except (FileNotFoundError, MemoryNotFoundError, ValueError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


def main() -> None:
    raise SystemExit(run())
