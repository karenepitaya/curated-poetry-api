package poetryapi

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDigitalSourceAcceptsElectronicTextWithoutScanClaims(t *testing.T) {
	catalog := mustLoad(t)
	work, err := catalog.RandomWork(Query{Collection: "songci-digital-selection", Dynasty: DynastySong, Genre: GenreCi, Script: ScriptHant})
	if err != nil {
		t.Fatal(err)
	}
	if work.Evidence.Level != EvidenceDigitalTextChecked || work.Evidence.Status != "validated" || len(work.Evidence.Witnesses) != 0 || work.Tune == nil {
		t.Fatalf("wrong digital evidence: %#v", work.Evidence)
	}
	if err := CheckFilesFS(embeddedCorpus, []string{"corpus/works/song/" + work.ID + ".json"}); err != nil {
		t.Fatal(err)
	}
	original := work.Evidence.DigitalSource.RecordIndex
	work.Evidence.DigitalSource.RecordIndex = -1
	for _, stored := range catalog.Works() {
		if stored.ID == work.ID && stored.Evidence.DigitalSource.RecordIndex != original {
			t.Fatal("RandomWork leaked digital source pointer")
		}
	}
}

func TestDigitalSourceRejectsTampering(t *testing.T) {
	catalog := mustLoad(t)
	var original Work
	for _, work := range catalog.Works() {
		if work.Evidence.Level == EvidenceDigitalTextChecked {
			original = work
			break
		}
	}
	cases := []struct {
		name, want string
		change     func(*Work)
	}{
		{"body", "paragraph 0 differs", func(w *Work) { w.Sections[0].Lines[0].Hans = "篡改正文。" }},
		{"truncated", "paragraphs differ", func(w *Work) { w.Sections[0].Lines = w.Sections[0].Lines[:1] }},
		{"author", "author, tune or title differs", func(w *Work) { w.Author.Name.Hans = "李白" }},
		{"locator", "out of range", func(w *Work) { w.Evidence.DigitalSource.RecordIndex = 10000 }},
		{"conversion", "invalid digital source or conversion", func(w *Work) { w.Evidence.DigitalSource.Conversion = "invented" }},
		{"status", "requires validated status", func(w *Work) { w.Evidence.Status = "verified" }},
		{"fake-scan", "scan review must not use digital", func(w *Work) { w.Evidence.Level = EvidencePrimaryScanReviewed }},
		{"missing-source", "source locator", func(w *Work) { w.Evidence.DigitalSource = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := cloneWork(original)
			tc.change(&work)
			raw, err := json.Marshal(work)
			if err != nil {
				t.Fatal(err)
			}
			name := "corpus/works/song/" + work.ID + ".json"
			overlay := overlayFS{base: embeddedCorpus, files: fstest.MapFS{name: &fstest.MapFile{Data: raw}}}
			err = CheckFilesFS(overlay, []string{name})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v; want %q", err, tc.want)
			}
		})
	}
}

func TestDigitalSourceHashPreventsSnapshotDrift(t *testing.T) {
	name := "sources/chinese-poetry-songci/songci-300.json"
	raw, err := fs.ReadFile(embeddedCorpus, name)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	overlay := overlayFS{base: embeddedCorpus, files: fstest.MapFS{name: &fstest.MapFile{Data: raw}}}
	_, err = LoadFS(overlay)
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("got %v", err)
	}
}
