from pathlib import Path

from pmem.cli import run


def test_cli_init(project_root: Path):
    code = run(["--project-root", str(project_root), "init"])

    assert code == 0
    assert (project_root / ".project-memory" / "cards").is_dir()


def test_cli_remember_and_search(project_root: Path, capsys):
    run(["--project-root", str(project_root), "init"])
    card_file = project_root / "card.yaml"
    card_file.write_text(
        """
type: decision
title: YAML storage
summary: YAML is the source of truth.
content: SQLite is only an index.
confidence: 0.9
source:
  kind: conversation
  description: User confirmed YAML storage.
  files: []
  commits: []
scope:
  project: demo
  modules: [storage]
  paths: [src/storage.py]
tags: [yaml, storage]
relations:
  supersedes: []
  related_to: []
  explains: []
  caused_by: []
""".strip(),
        encoding="utf-8",
    )

    assert run(["--project-root", str(project_root), "remember", "--file", str(card_file)]) == 0
    assert run(["--project-root", str(project_root), "search", "YAML"]) == 0
    output = capsys.readouterr().out

    assert "YAML storage" in output
    assert "YAML is the source of truth." in output
    assert "SQLite is only an index" not in output
