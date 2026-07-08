package service

import "time"

// ParseIntParam parses an integer from a string parameter.
// Returns defaultVal if the string is empty or not a valid integer.
func ParseIntParam(s string, defaultVal int) int {
	if s == "" { return defaultVal }
	n := 0
	for _, c := range s { if c < 48 || c > 57 { return defaultVal }; n = n*10 + int(c-48) }
	return n
}

// IsAfterTime compares two RFC3339 timestamps and returns true if updated is
// strictly after since. Returns true when since is empty (no filter).
// Falls back to string comparison when timestamps cannot be parsed.
func IsAfterTime(updated, since string) bool {
	if since == "" { return true }
	updTime, err1 := time.Parse(time.RFC3339, updated)
	sinceTime, err2 := time.Parse(time.RFC3339, since)
	if err1 != nil || err2 != nil { return updated > since }
	return updTime.After(sinceTime)
}
