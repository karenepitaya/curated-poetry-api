package poetryapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

type digitalRecord struct {
	Author     string   `json:"author"`
	Paragraphs []string `json:"paragraphs"`
	Rhythmic   string   `json:"rhythmic"`
	Tags       []string `json:"tags"`
}

func validSourcePath(name string) bool {
	return fs.ValidPath(name) && strings.HasPrefix(name, "sources/") && path.Ext(name) == ".json"
}

func loadDigitalSources(dataFS fs.FS, editions []Edition) (map[string][]digitalRecord, error) {
	result := make(map[string][]digitalRecord)
	for _, edition := range editions {
		if edition.Kind != "digital-text" {
			continue
		}
		if !validSourcePath(edition.SourcePath) {
			return nil, fmt.Errorf("invalid digital source path %q", edition.SourcePath)
		}
		raw, err := fs.ReadFile(dataFS, edition.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("read digital source: %w", err)
		}
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != edition.SHA256 {
			return nil, fmt.Errorf("digital source %s: SHA-256 mismatch", edition.ID)
		}
		var records []digitalRecord
		if err := json.Unmarshal(raw, &records); err != nil {
			return nil, fmt.Errorf("decode digital source %s: %w", edition.ID, err)
		}
		if len(records) == 0 {
			return nil, fmt.Errorf("digital source %s is empty", edition.ID)
		}
		result[edition.ID] = records
	}
	return result, nil
}

// Digital titles use the source tune and incipit because the dataset has no titles.
func digitalTitle(record digitalRecord) string {
	if len(record.Paragraphs) == 0 {
		return record.Rhythmic
	}
	first := strings.FieldsFunc(record.Paragraphs[0], func(r rune) bool { return strings.ContainsRune("，。！？；、", r) })
	if len(first) == 0 {
		return record.Rhythmic
	}
	return record.Rhythmic + "·" + first[0]
}

func validateDigitalRecord(label string, work Work, records map[string][]digitalRecord) []string {
	source := work.Evidence.DigitalSource
	if source == nil {
		return nil
	} // Reported by validateEvidence.
	rows, exists := records[source.EditionID]
	if !exists || source.RecordIndex < 0 || source.RecordIndex >= len(rows) {
		return []string{label + ": digital record locator is out of range"}
	}
	record := rows[source.RecordIndex]
	var problems []string
	if work.Author.Name.Hans != record.Author || work.Tune == nil || work.Tune.Hans != record.Rhythmic || work.Title.Hans != digitalTitle(record) {
		problems = append(problems, label+": author, tune or title differs from pinned digital record")
	}
	if work.Dynasty != DynastySong || work.Genre != GenreCi {
		problems = append(problems, label+": digital Song Ci source requires song/ci")
	}
	if len(work.Sections) != 1 || work.Sections[0].Kind != "stanza" || len(work.Sections[0].Lines) != len(record.Paragraphs) {
		return append(problems, label+": paragraphs differ from pinned digital record")
	}
	for i, line := range work.Sections[0].Lines {
		if line.Hans != record.Paragraphs[i] {
			problems = append(problems, fmt.Sprintf("%s: paragraph %d differs from pinned digital record", label, i))
		}
	}
	return problems
}
