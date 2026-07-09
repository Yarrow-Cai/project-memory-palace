package memory

import "sort"

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
	"depends_on", "contradicts", "abstracts", "implements", "derives_from",
	"replaces", "documents",
}

var RelationSemantics = map[string]map[string]string{
	"supersedes":    {"inverse": "superseded_by", "desc": "新版本替代旧版本"},
	"superseded_by": {"inverse": "supersedes", "desc": "被新版本替代"},
	"depends_on":    {"inverse": "depended_by", "desc": "依赖"},
	"contradicts":   {"inverse": "contradicts", "desc": "矛盾/互斥", "symmetric": "true"},
	"abstracts":     {"inverse": "abstracted_by", "desc": "抽象概括"},
	"implements":    {"inverse": "implemented_by", "desc": "具体实现"},
	"derives_from":  {"inverse": "derived_by", "desc": "派生自"},
	"replaces":      {"inverse": "replaced_by", "desc": "取代旧版"},
	"documents":     {"inverse": "documented_by", "desc": "文档记录"},
	"explains":      {"inverse": "explained_by", "desc": "解释说明"},
	"caused_by":     {"inverse": "causes", "desc": "因果关系"},
	"related_to":    {"inverse": "related_to", "desc": "一般关联", "symmetric": "true"},
}

func IsValidRelation(kind string) bool {
	if kind == "" {
		return false
	}
	for _, r := range RelationKinds {
		if r == kind {
			return true
		}
	}
	return false
}

var RequiredFields = []string{
	"schema_version", "id", "type", "status", "confidence",
	"title", "summary", "content", "source", "scope",
	"tags", "relations", "created_at", "updated_at",
	"source_agent", "knowledge_kind",
}

const DefaultConfidence = 0.5
const SchemaVersion = 1
const MaxConfidenceNoSource = 0.5

// AgentTrustProfiles maps source_agent identifiers to confidence caps.
// When an agent creates a card, its confidence is capped at its trust level.
// Agents not in this map default to 0.5 (conservative).
var AgentTrustProfiles = map[string]float64{
	"claude-code":   0.85,
	"codex-cli":     0.80,
	"hermes-agent":  0.80,
	"manual":        1.00,
	"human":         1.00,
}
var RememberRequiredFields = []string{"content", "summary", "title", "type"}

var UpdateAllowedFields = map[string]bool{
	"confidence": true, "reason": true, "relations": true,
	"status": true, "tags": true, "expires_at": true,
	"source_agent": true, "knowledge_kind": true, "priority": true,
}

var KnowledgeKinds = map[string]bool{"fact": true, "interpretation": true, "rule": true}

// sortedKeys returns the sorted keys of a map[string]bool.
// Used by MCP tool schema generation for stable enum ordering.
func SortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
