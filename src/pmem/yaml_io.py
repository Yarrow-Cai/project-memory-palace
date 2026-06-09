from __future__ import annotations

import re
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


def write_card(project_root: Path, data: dict[str, Any]) -> Path:
    ensure_project_memory(project_root)
    card = MemoryCard.from_dict(data)
    path = cards_dir(project_root) / card_filename(card.to_dict())
    path.write_text(
        yaml.safe_dump(card.to_dict(), sort_keys=False, allow_unicode=True),
        encoding="utf-8",
    )
    return path


def read_card(path: Path) -> MemoryCard:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise ValueError(f"card file must contain a mapping: {path}")
    return MemoryCard.from_dict(data)


def discover_cards(project_root: Path) -> list[MemoryCard]:
    ensure_project_memory(project_root)
    cards = [read_card(path) for path in cards_dir(project_root).glob("*.yaml")]
    return sorted(cards, key=lambda card: card.id)


def next_card_identity(project_root: Path, date_part: str) -> tuple[str, int]:
    ensure_project_memory(project_root)
    date_token = date_part.replace("-", "")
    max_sequence = 0
    for card in discover_cards(project_root):
        match = ID_RE.match(card.id)
        if match and match.group(1) == date_token:
            max_sequence = max(max_sequence, int(match.group(2)))
    next_sequence = max_sequence + 1
    return f"mem_{date_token}_{next_sequence:03d}", next_sequence


def assert_memory_layout(project_root: Path) -> None:
    required = [memory_dir(project_root), cards_dir(project_root), config_path(project_root), rules_path(project_root)]
    missing = [str(path) for path in required if not path.exists()]
    if missing:
        raise FileNotFoundError(f"project memory is not initialized: {', '.join(missing)}")
