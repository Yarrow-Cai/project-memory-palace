package memory

var MemoryTypes = map[string]bool{
	"project_goal": true, "design": true, "decision": true,
	"change_reason": true, "bugfix": true, "module": true,
	"convention": true, "open_question": true,
	"architecture": true, "driver": true, "pinout": true,
	"hardware": true, "startup": true, "pattern": true,
	"knowledge": true, "insight": true, "fact": true,
	"note": true, "api": true, "trick": true,
}

var MemoryStatuses = map[string]bool{
	"active": true, "stale": true, "superseded": true, "rejected": true,
	"expired": true,
}

var SourceKinds = map[string]bool{
	"conversation": true, "file": true, "commit": true,
	"manual": true, "test": true, "analysis": true,
	"convention": true, "experiment": true, "observation": true,
	"documentation": true, "review": true, "specification": true,
}

var RelationKinds = []string{
	"supersedes", "superseded_by", "related_to", "explains", "caused_by",
}

var RequiredFields = []string{
	"schema_version", "id", "type", "status", "confidence",
	"title", "summary", "content", "source", "scope",
	"tags", "relations", "created_at", "updated_at",
}

const DefaultConfidence = 0.5
const SchemaVersion = 1
const MaxConfidenceNoSource = 0.5

var RememberRequiredFields = []string{"content", "summary", "title", "type"}

var UpdateAllowedFields = map[string]bool{
	"confidence": true, "reason": true, "relations": true,
	"status": true, "tags": true, "expires_at": true,
}
