from pathlib import Path

import pytest

from pmem.cli import run


def assert_memory_not_created(project_root: Path):
    assert not (project_root / ".project-memory").exists()


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


def test_cli_parse_failure_returns_code_2(project_root: Path, capsys):
    code = run(["--project-root", str(project_root)])

    assert code == 2
    assert "error:" in capsys.readouterr().err
    assert_memory_not_created(project_root)


@pytest.mark.parametrize("command", ["search", "recent"])
def test_cli_rejects_non_positive_limit(
    project_root: Path, capsys, command: str
):
    args = ["--project-root", str(project_root), command]
    if command == "search":
        args.append("YAML")
    args.extend(["--limit", "0"])

    code = run(args)

    assert code == 2
    assert "positive integer" in capsys.readouterr().err
    assert_memory_not_created(project_root)


def test_cli_remember_missing_file_returns_error(project_root: Path, capsys):
    code = run(
        [
            "--project-root",
            str(project_root),
            "remember",
            "--file",
            str(project_root / "missing.yaml"),
        ]
    )

    assert code == 1
    assert "error:" in capsys.readouterr().err


def test_cli_remember_invalid_yaml_returns_error(project_root: Path, capsys):
    card_file = project_root / "card.yaml"
    project_root.mkdir()
    card_file.write_text("title: [", encoding="utf-8")

    code = run(
        ["--project-root", str(project_root), "remember", "--file", str(card_file)]
    )

    assert code == 1
    assert "invalid YAML file" in capsys.readouterr().err


def test_cli_remember_empty_yaml_returns_error(project_root: Path, capsys):
    card_file = project_root / "card.yaml"
    project_root.mkdir()
    card_file.write_text("", encoding="utf-8")

    code = run(
        ["--project-root", str(project_root), "remember", "--file", str(card_file)]
    )

    assert code == 1
    assert "card file is empty" in capsys.readouterr().err


def test_cli_remember_non_mapping_yaml_returns_error(project_root: Path, capsys):
    card_file = project_root / "card.yaml"
    project_root.mkdir()
    card_file.write_text("- YAML storage", encoding="utf-8")

    code = run(
        ["--project-root", str(project_root), "remember", "--file", str(card_file)]
    )

    assert code == 1
    assert "card file must contain a mapping" in capsys.readouterr().err


def test_cli_open_missing_memory_id_omits_keyerror_quotes(
    project_root: Path, capsys
):
    run(["--project-root", str(project_root), "init"])

    code = run(["--project-root", str(project_root), "open", "mem_20990101_999"])

    assert code == 1
    assert capsys.readouterr().err == "error: memory not found: mem_20990101_999\n"


@pytest.mark.parametrize(
    "args",
    [
        ["audit"],
        ["open", "mem_20990101_999"],
        ["recent"],
        ["search", "YAML"],
    ],
)
def test_cli_read_failures_do_not_initialize_project(
    project_root: Path, capsys, args: list[str]
):
    code = run(["--project-root", str(project_root), *args])

    assert code == 1
    assert "error:" in capsys.readouterr().err
    assert_memory_not_created(project_root)
