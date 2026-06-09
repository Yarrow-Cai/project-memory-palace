from pathlib import Path


MEMORY_DIR = ".project-memory"


def memory_dir(project_root: Path) -> Path:
    return project_root / MEMORY_DIR


def cards_dir(project_root: Path) -> Path:
    return memory_dir(project_root) / "cards"


def rules_dir(project_root: Path) -> Path:
    return memory_dir(project_root) / "rules"


def config_path(project_root: Path) -> Path:
    return memory_dir(project_root) / "config.yaml"


def rules_path(project_root: Path) -> Path:
    return rules_dir(project_root) / "agent-rules.yaml"


def index_path(project_root: Path) -> Path:
    return memory_dir(project_root) / "index.sqlite3"
