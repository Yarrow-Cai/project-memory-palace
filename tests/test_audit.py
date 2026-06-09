from pathlib import Path

from pmem.audit import audit_project
from pmem.service import MemoryService


def payload() -> dict:
    return {
        "type": "decision",
        "title": "Untaged low confidence",
        "summary": "This card needs audit.",
        "content": "The card has no tags and low confidence.",
        "confidence": 0.4,
        "source": {
            "kind": "analysis",
            "description": "AI inference.",
            "files": [],
            "commits": [],
        },
        "scope": {
            "project": "demo",
            "modules": [],
            "paths": [],
        },
        "tags": [],
        "relations": {},
    }


def test_audit_reports_low_quality_card(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(payload())

    report = audit_project(project_root)

    assert report[0]["id"] == created["id"]
    assert "low_confidence" in report[0]["issues"]
    assert "missing_tags" in report[0]["issues"]
    assert "missing_scope" in report[0]["issues"]
