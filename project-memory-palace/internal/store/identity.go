package store

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// idRe matches card IDs of the form mem_YYYYMMDD_NNN.
var idRe = regexp.MustCompile(`^mem_(\d{8})_(\d{3})$`)

// filenameRe matches filenames of the form YYYY-MM-DD_NNN_type.yaml.
var filenameRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})_(\d{3})_.+\.yaml$`)

// NextCardIdentity scans the cards directory for the given date string
// ("YYYY-MM-DD") and returns the next available card identity string
// (mem_YYYYMMDD_NNN) and its sequence number.
//
// The first card on a date receives sequence 1. Returns an error if the
// cards directory cannot be read.
func NextCardIdentity(projectRoot, dateStr string) (string, int, error) {
	entries, err := os.ReadDir(CardsDir(projectRoot))
	if err != nil {
		return "", 0, fmt.Errorf("readdir cards: %w", err)
	}

	maxSeq := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := filenameRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if m[1] != dateStr {
			continue
		}
		seq, _ := strconv.Atoi(m[2])
		if seq > maxSeq {
			maxSeq = seq
		}
	}

	nextSeq := maxSeq + 1
	compactDate := strings.ReplaceAll(dateStr, "-", "")
	id := fmt.Sprintf("mem_%s_%03d", compactDate, nextSeq)
	return id, nextSeq, nil
}
