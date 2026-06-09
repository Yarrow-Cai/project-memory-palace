MEMORY_TYPES = {
    "project_goal",
    "design",
    "decision",
    "change_reason",
    "bugfix",
    "module",
    "convention",
    "open_question",
}

MEMORY_STATUSES = {
    "active",
    "stale",
    "superseded",
    "rejected",
}

SOURCE_KINDS = {
    "conversation",
    "file",
    "commit",
    "manual",
    "test",
    "analysis",
}

RELATION_KINDS = {
    "supersedes",
    "superseded_by",
    "related_to",
    "explains",
    "caused_by",
}

REQUIRED_FIELDS = {
    "schema_version",
    "id",
    "type",
    "status",
    "confidence",
    "title",
    "summary",
    "content",
    "source",
    "scope",
    "tags",
    "relations",
    "created_at",
    "updated_at",
}
