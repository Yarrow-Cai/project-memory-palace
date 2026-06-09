from __future__ import annotations

import re
from datetime import date
from pathlib import Path
from typing import Any

import yaml

from pmem.defaults import DEFAULT_AGENT_RULES, DEFAULT_CONFIG
from pmem.models import MemoryCard
from pmem.paths import cards_dir, config_path, memory_dir, rules_dir, rules_path


ID_RE = re.compile(r"^mem_(\d{8})_(\d{3})$")


def ensure_project_memory(project_root: Path) -> None:
    cards_dir(project_root).mkdir(parents=True, exist_ok=True)
    rules_dir(project_root).mkdir(parents=True, exist_ok=True)
    if not config_path(project_root).exists():
        config_path(project_root).write_text(DEFAULT_CONFIG, encoding="utf-8")
    if not rules_path(project_root).exists():
        rules_path(project_root).write_text(DEFAULT_AGENT_RULES, encoding="utf-8")


def card_filename(card: dict[str, Any]) -> str:
    match = ID_RE.match(card["id"])
    if not match:
        raise ValueError(f"invalid memory id: {card['id']}")
    date_token = match.group(1)
    sequence = match.group(2)
    date_part = f"{date_token[0:4]}-{date_token[4:6]}-{date_token[6:8]}"
    return f"{date_part}_{sequence}_{card['type']}.yaml"


def write_card(project_root: Path, data: dict[str, Any], overwrite: bool = False) -> Path:
    ensure_project_memory(project_root)
    card = MemoryCard.from_dict(data)
    path = cards_dir(project_root) / card_filename(card.to_dict())
    if path.exists() and not overwrite:
        raise FileExistsError(f"memory card already exists: {path}")
    path.write_text(
        yaml.safe_dump(card.to_dict(), sort_keys=False, allow_unicode=True),
        encoding="utf-8",
    )
    return path


def read_card(path: Path) -> MemoryCard:
    try:
        data = yaml.safe_load(path.read_text(encoding="utf-8"))
    except yaml.YAMLError as error:
        raise ValueError(f"invalid card file {path}: {error}") from error
    if not isinstance(data, dict):
        raise ValueError(f"invalid card file {path}: card file must contain a mapping")
    try:
        return MemoryCard.from_dict(data)
    except ValueError as error:
        raise ValueError(f"invalid card file {path}: {error}") from error


def discover_cards(project_root: Path) -> list[MemoryCard]:
    assert_memory_layout(project_root)
    cards = [read_card(path) for path in cards_dir(project_root).glob("*.yaml")]
    return sorted(cards, key=lambda card: card.id)


def next_card_identity(project_root: Path, date_part: str) -> tuple[str, int]:
    ensure_project_memory(project_root)
    if not isinstance(date_part, str):
        raise ValueError(f"memory date must be an ISO date string: {date_part!r}")
    try:
        parsed_date = date.fromisoformat(date_part)
    except ValueError as error:
        raise ValueError(f"invalid memory date: {date_part}") from error
    date_token = parsed_date.strftime("%Y%m%d")
    max_sequence = 0
    for card in discover_cards(project_root):
        match = ID_RE.match(card.id)
        if match and match.group(1) == date_token:
            max_sequence = max(max_sequence, int(match.group(2)))
    next_sequence = max_sequence + 1
    if next_sequence > 999:
        raise ValueError(f"memory card sequence exceeds 999 for {date_part}")
    return f"mem_{date_token}_{next_sequence:03d}", next_sequence


def assert_memory_layout(project_root: Path) -> None:
    required_dirs = [memory_dir(project_root), cards_dir(project_root), rules_dir(project_root)]
    required_files = [config_path(project_root), rules_path(project_root)]
    invalid = [f"{path} (directory)" for path in required_dirs if not path.is_dir()]
    invalid.extend(f"{path} (file)" for path in required_files if not path.is_file())
    if invalid:
        raise FileNotFoundError(
            f"project memory layout is missing or invalid: {', '.join(invalid)}"
        )
