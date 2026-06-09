from __future__ import annotations

from pathlib import Path
from typing import Any

from pmem.yaml_io import discover_cards


def audit_project(project_root: Path) -> list[dict[str, Any]]:
    report: list[dict[str, Any]] = []
    seen_titles: dict[str, str] = {}
    for card in discover_cards(project_root):
        data = card.to_dict()
        issues: list[str] = []
        if data["confidence"] <= 0.5:
            issues.append("low_confidence")
        if not data["tags"]:
            issues.append("missing_tags")
        if not data["scope"].get("modules") and not data["scope"].get("paths"):
            issues.append("missing_scope")
        if data["source"]["kind"] == "analysis" and data["confidence"] > 0.7:
            issues.append("high_confidence_inference")
        title_key = data["title"].strip().lower()
        if title_key in seen_titles:
            issues.append(f"possible_duplicate:{seen_titles[title_key]}")
        else:
            seen_titles[title_key] = data["id"]
        if data["status"] == "stale":
            issues.append("stale")
        if issues:
            report.append(
                {
                    "id": data["id"],
                    "title": data["title"],
                    "status": data["status"],
                    "issues": issues,
                }
            )
    return report
