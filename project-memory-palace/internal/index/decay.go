package index

import (
	"fmt"
	"time"
)

// EffectivePriority computes a decayed priority based on manual priority
// and time since last access.  Returns the manual priority unchanged when
// lastAccessedAt is empty or cannot be parsed.
//
// Decay schedule:
//   < 7 days:   1.0x
//   < 30 days:  0.85x
//   < 60 days:  0.6x
//   < 180 days: 0.4x
//   >= 180 days: 0.25x
func EffectivePriority(manualPriority int, lastAccessedAt string) float64 {
	if lastAccessedAt == "" {
		return float64(manualPriority)
	}
	t, err := time.Parse(time.RFC3339, lastAccessedAt)
	if err != nil {
		return float64(manualPriority)
	}
	days := time.Since(t).Hours() / 24
	var factor float64
	switch {
	case days < 7:
		factor = 1.0
	case days < 30:
		factor = 0.85
	case days < 60:
		factor = 0.6
	case days < 180:
		factor = 0.4
	default:
		factor = 0.25
	}
	return float64(manualPriority) * factor
}

// EffectivePriorityString returns a formatted string showing original and
// decayed priority, e.g. "3->2.6".
func EffectivePriorityString(manualPriority int, lastAccessedAt string) string {
	ep := EffectivePriority(manualPriority, lastAccessedAt)
	return fmt.Sprintf("%d->%.1f", manualPriority, ep)
}
