from pathlib import Path

import pytest

from pmem.audit import audit_project
from pmem.service import MemoryService


def payload(**overrides) -> dict:
    data = {
        "type": "decision",
        "title": "Tagged high confidence",
        "summary": "This card is audit clean.",
        "content": "The card has tags, scope, and a conversation source.",
        "confidence": 0.9,
        "source": {
            "kind": "conversation",
            "description": "User confirmed the decision.",
            "files": [],
            "commits": [],
        },
        "scope": {
            "project": "demo",
            "modules": ["audit"],
            "paths": ["src/pmem/audit.py"],
        },
        "tags": ["audit"],
        "relations": {},
    }
    data.update(overrides)
    return data


def low_quality_payload() -> dict:
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


def test_audit_returns_empty_report_for_clean_valid_card(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    service.remember(payload())

    report = audit_project(project_root)

    assert report == []


def test_audit_reports_low_quality_card(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(low_quality_payload())

    report = audit_project(project_root)

    assert report[0]["id"] == created["id"]
    assert "low_confidence" in report[0]["issues"]
    assert "missing_tags" in report[0]["issues"]
    assert "missing_scope" in report[0]["issues"]


def test_audit_reports_stale_card(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(payload())
    service.update_memory(created["id"], {"status": "stale"})

    report = audit_project(project_root)

    assert report == [
        {
            "id": created["id"],
            "title": "Tagged high confidence",
            "status": "stale",
            "issues": ["stale"],
        }
    ]


def test_audit_reports_duplicate_title_against_first_sorted_id(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    first = service.remember(payload(title="Shared title"))
    second = service.remember(payload(title="Shared title"))

    report = audit_project(project_root)

    assert len(report) == 1
    assert report[0]["id"] == second["id"]
    assert report[0]["issues"] == [f"possible_duplicate:{first['id']}"]


def test_audit_reports_high_confidence_analysis_source(project_root: Path):
    service = MemoryService(project_root)
    service.init_project()
    created = service.remember(
        payload(
            confidence=0.8,
            source={
                "kind": "analysis",
                "description": "Inferred from code structure.",
                "files": [],
                "commits": [],
            },
        )
    )

    report = audit_project(project_root)

    assert report == [
        {
            "id": created["id"],
            "title": "Tagged high confidence",
            "status": "active",
            "issues": ["high_confidence_inference"],
        }
    ]


def test_audit_uninitialized_project_memory_raises(project_root: Path):
    with pytest.raises(FileNotFoundError):
        audit_project(project_root)
