package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/atop/project-memory-palace/internal/memory"
	"gopkg.in/yaml.v3"
)

// CardFilename produces the canonical filename for a memory card based on its
// ID and type, e.g. "2026-06-09_001_decision.yaml". Returns an empty string when
// the card ID cannot be parsed.
func CardFilename(card *memory.MemoryCard) string {
	m := idRe.FindStringSubmatch(card.ID)
	if m == nil {
		return ""
	}
	date := m[1]
	seq := m[2]
	formatted := date[0:4] + "-" + date[4:6] + "-" + date[6:8]
	return fmt.Sprintf("%s_%s_%s.yaml", formatted, seq, card.Type)
}

// WriteCard serialises the card to YAML and writes it atomically inside the
// project cards directory.  When overwrite is false and the target file already
// exists an error is returned.  The card is validated before writing.  Returns
// the full path written.
func WriteCard(projectRoot string, card *memory.MemoryCard, overwrite bool) (string, error) {
	if err := memory.ValidateCard(card); err != nil {
		return "", fmt.Errorf("validate: %w", err)
	}

	filename := CardFilename(card)
	if filename == "" {
		return "", errors.New("cannot derive filename: invalid card ID")
	}

	if err := EnsureCardsDir(projectRoot); err != nil {
		return "", err
	}

	targetPath := filepath.Join(CardsDir(projectRoot), filename)

	if !overwrite {
		if _, err := os.Stat(targetPath); err == nil {
			return "", fmt.Errorf("card file already exists: %s (use overwrite)", filename)
		}
	}

	data, err := yaml.Marshal(card)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	tmp, err := os.CreateTemp(CardsDir(projectRoot), ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("tempfile: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write temp: %w", err)
	}
	tmp.Close()

	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename: %w", err)
	}

	return targetPath, nil
}

// ReadCard reads and validates a single memory card from the given YAML file
// path.  Returns the parsed card or an error.
func ReadCard(path string) (*memory.MemoryCard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var card memory.MemoryCard
	if err := yaml.Unmarshal(data, &card); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}

	if err := memory.ValidateCard(&card); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}

	return &card, nil
}

// SaveHistory copies the current YAML card file to the history directory.
// The old card data is preserved before UpdateMemory overwrites it.
func SaveHistory(projectRoot string, cardFilename string) error {
	srcPath := filepath.Join(CardsDir(projectRoot), cardFilename)
	histDir := filepath.Join(HistoryDir(projectRoot), cardFilename[:len(cardFilename)-len(".yaml")])
	if err := os.MkdirAll(histDir, 0755); err != nil { return err }
	ts := time.Now().Format("20060102T150405")
	dstPath := filepath.Join(histDir, ts+".yaml")
	src, err := os.ReadFile(srcPath)
	if err != nil { return err }
	return os.WriteFile(dstPath, src, 0644)
}

// DiscoverCards scans the cards directory for *.yaml files, parses and
// validates each one, and returns them sorted by card ID.  If the cards
// directory does not exist an empty slice is returned.
func DiscoverCards(projectRoot string) ([]*memory.MemoryCard, error) {
	pattern := filepath.Join(CardsDir(projectRoot), "*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}

	cards := make([]*memory.MemoryCard, 0, len(matches))
	var skipped []string
	for _, p := range matches {
		card, err := ReadCard(p)
		if err != nil {
			skipped = append(skipped, filepath.Base(p))
			continue
		}
		cards = append(cards, card)
	}

	if len(skipped) > 0 {
		fmt.Fprintf(os.Stderr, "pmem: WARNING — skipped %d unreadable card(s): %s\n", len(skipped), strings.Join(skipped, ", "))
	}

	sort.Slice(cards, func(i, j int) bool {
		return cards[i].ID < cards[j].ID
	})

	return cards, nil
}
