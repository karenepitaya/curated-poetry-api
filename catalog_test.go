package poetryapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedCatalogLoadsMigratedCorpus(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if catalog.Count() != 326 {
		t.Fatalf("Count() = %d, want 326", catalog.Count())
	}
	stats := catalog.Stats()
	if stats.Works != 326 || stats.ByDynasty[DynastyTang] != 50 || stats.ByDynasty[DynastySong] != 276 {
		t.Fatalf("Stats() = %#v", stats)
	}
	if len(stats.CorpusRevision) != 64 {
		t.Fatalf("corpus revision length = %d, want 64", len(stats.CorpusRevision))
	}
	if len(catalog.editions) != 33 {
		t.Fatalf("edition count = %d, want 33", len(catalog.editions))
	}
	for _, edition := range catalog.editions {
		if edition.ID == "nlc-tang300-1933" && edition.RevisionID != "845982203" {
			t.Fatalf("nlc-tang300-1933 revisionId = %q", edition.RevisionID)
		}
	}

	witnesses := 0
	variants := 0
	for _, work := range catalog.works {
		witnesses += len(work.Evidence.Witnesses)
		variants += len(work.Evidence.Variants)
	}
	applications := 0
	for _, rule := range catalog.normalizations {
		applications += len(rule.AuditedWorkIDs)
	}
	if witnesses != 100 || variants != 58 || applications != 416 {
		t.Fatalf("migrated evidence totals = witnesses:%d variants:%d normalization applications:%d", witnesses, variants, applications)
	}
}

func TestCollectionMembershipSeparatesSupplementalWork(t *testing.T) {
	catalog := mustLoad(t)
	counts := map[string]int{}
	for _, work := range catalog.works {
		for _, membership := range work.Collections {
			counts[membership.ID]++
			if membership.ID == "tangshi-sanbaishou-1933" && (membership.Position != nil || membership.PositionStatus != "pending") {
				t.Fatalf("unverified global position exposed for %s: %#v", work.ID, membership)
			}
			if membership.ID == "supplemental-classics" && (membership.Position == nil || *membership.Position != 1 || membership.PositionStatus != "confirmed") {
				t.Fatalf("supplemental position = %#v", membership)
			}
		}
	}
	if counts["tangshi-sanbaishou-1933"] != 49 || counts["supplemental-classics"] != 1 {
		t.Fatalf("collection membership counts = %#v", counts)
	}

	work, err := catalog.RandomWork(Query{Collection: "supplemental-classics", Script: ScriptHans})
	if err != nil {
		t.Fatalf("RandomWork() error = %v", err)
	}
	if work.ID != "tang-cui-hu-ti-du-cheng-nan-zhuang" {
		t.Fatalf("supplemental selection = %q", work.ID)
	}
}

func TestRandomWorkFiltersAndMaxCharacters(t *testing.T) {
	catalog := mustLoad(t)
	catalog.randomReader = bytes.NewReader(make([]byte, 32))
	work, err := catalog.RandomWork(Query{
		Collection: "tangshi-sanbaishou-1933",
		Dynasty:    DynastyTang,
		Genre:      GenreShi,
		Form:       FormJueju,
		Meter:      MeterFive,
		MaxChars:   20,
		Script:     ScriptHant,
	})
	if err != nil {
		t.Fatalf("RandomWork() error = %v", err)
	}
	if work.Dynasty != DynastyTang || work.Genre != GenreShi || work.Form != FormJueju || work.Meter != MeterFive {
		t.Fatalf("selected work does not match query: %#v", work)
	}
	if got := workCharacterCount(work, ScriptHant); got != 20 {
		t.Fatalf("workCharacterCount() = %d, want 20", got)
	}

	_, err = catalog.RandomWork(Query{MaxChars: 19, Script: ScriptHans})
	if !errors.Is(err, ErrNoMatchingWorks) {
		t.Fatalf("RandomWork(max_chars=19) error = %v, want ErrNoMatchingWorks", err)
	}
	_, err = catalog.RandomWork(Query{Collection: "songci-sanbaishou-zhu", Script: ScriptHans})
	if !errors.Is(err, ErrNoMatchingWorks) {
		t.Fatalf("RandomWork(song collection) error = %v, want ErrNoMatchingWorks", err)
	}
}

func TestValidateQueryRejectsInvalidValues(t *testing.T) {
	tests := []Query{
		{Dynasty: "yuan"},
		{Genre: "qu"},
		{Form: "pailu"},
		{Meter: "6"},
		{MaxChars: -1},
		{MaxChars: 5001},
		{Script: "latin"},
		{Collection: "Invalid ID"},
	}
	for _, query := range tests {
		if err := ValidateQuery(query); err == nil {
			t.Errorf("ValidateQuery(%#v) error = nil", query)
		}
	}
}

func TestWorkCharacterCountIgnoresUnicodePunctuationAndWhitespace(t *testing.T) {
	work := Work{Sections: []Section{{Lines: []Line{{Hans: "甲， 乙。\n丙　", Hant: "甲， 乙。\n丙　"}}}}}
	if got := workCharacterCount(work, ScriptHans); got != 3 {
		t.Fatalf("workCharacterCount() = %d, want 3", got)
	}
}

func TestWorksAndStatsReturnCopies(t *testing.T) {
	catalog := mustLoad(t)
	works := catalog.Works()
	original := works[0].Sections[0].Lines[0].Hans
	works[0].Sections[0].Lines[0].Hans = "篡改。"
	if got := catalog.Works()[0].Sections[0].Lines[0].Hans; got != original {
		t.Fatalf("catalog work mutated through Works(): got %q, want %q", got, original)
	}
	stats := catalog.Stats()
	stats.ByDynasty[DynastyTang] = 0
	if got := catalog.Stats().ByDynasty[DynastyTang]; got != 50 {
		t.Fatalf("catalog stats mutated through Stats(): got %d", got)
	}
}

func TestCheckFilesFSValidatesOneWork(t *testing.T) {
	if err := CheckFilesFS(embeddedCorpus, []string{"corpus/works/tang/tang-li-bai-jing-ye-si.json"}); err != nil {
		t.Fatalf("CheckFilesFS() error = %v", err)
	}
}

func TestCheckFilesFSRejectsMissingPath(t *testing.T) {
	missing := "corpus/works/tang/definitely-missing.json"
	err := CheckFilesFS(embeddedCorpus, []string{missing})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("CheckFilesFS() error = %v, want missing path error", err)
	}
}

func TestCheckFilesFSFallsBackToFullForSharedMetadata(t *testing.T) {
	if err := CheckFilesFS(embeddedCorpus, []string{"corpus/normalization.json"}); err != nil {
		t.Fatalf("CheckFilesFS() error = %v", err)
	}
}

func TestCheckFilesFSCatchesDuplicateAgainstUnselectedWork(t *testing.T) {
	selectedPath := "corpus/works/tang/tang-wang-wei-lu-zhai.json"
	modifiedPath := "corpus/works/tang/tang-wang-wei-zhu-li-guan.json"
	selectedData, err := fs.ReadFile(embeddedCorpus, modifiedPath)
	if err != nil {
		t.Fatal(err)
	}
	var work Work
	if err := json.Unmarshal(selectedData, &work); err != nil {
		t.Fatal(err)
	}
	work.Title = LocalizedText{Hans: "鹿柴", Hant: "鹿柴"}
	selectedData, err = json.Marshal(work)
	if err != nil {
		t.Fatal(err)
	}
	overlay := overlayFS{
		base: embeddedCorpus,
		files: fstest.MapFS{
			modifiedPath: &fstest.MapFile{Data: selectedData},
		},
	}
	var overlaid Work
	if err := decodeJSONFile(overlay, modifiedPath, &overlaid); err != nil {
		t.Fatal(err)
	}
	if overlaid.Title.Hans != "鹿柴" {
		t.Fatalf("overlay title = %q", overlaid.Title.Hans)
	}
	err = CheckFilesFS(overlay, []string{selectedPath})
	if err == nil || !strings.Contains(err.Error(), "author/title duplicates") {
		t.Fatalf("CheckFilesFS() error = %v, want cross-work duplicate", err)
	}
}

func TestValidateCorpusRejectsMissingVariant(t *testing.T) {
	catalog := mustLoad(t)
	works := catalog.Works()
	for i := range works {
		if len(works[i].Evidence.Variants) == 0 {
			continue
		}
		works[i].Evidence.Variants = []Variant{}
		err := validateCorpus(corpusData{
			works:          works,
			editions:       catalog.editions,
			collections:    catalog.collections,
			normalizations: catalog.normalizations,
			digitalRecords: mustDigitalRecords(t),
		}, false)
		if err == nil || !strings.Contains(err.Error(), "variants reconstruct") {
			t.Fatalf("validateCorpus() error = %v, want witness reconstruction failure", err)
		}
		return
	}
	t.Fatal("embedded corpus has no variants")
}

func TestValidateCorpusRejectsUntrackedNormalization(t *testing.T) {
	catalog := mustLoad(t)
	works := catalog.Works()
	for i := range works {
		if works[i].Dynasty == DynastyTang {
			works[i].Sections[0].Lines[0].Hans = "不一致。"
			break
		}
	}
	err := validateCorpus(corpusData{
		works:          works,
		editions:       catalog.editions,
		collections:    catalog.collections,
		normalizations: catalog.normalizations,
		digitalRecords: mustDigitalRecords(t),
	}, false)
	if err == nil || !strings.Contains(err.Error(), "normalization produced") {
		t.Fatalf("validateCorpus() error = %v, want normalization mismatch", err)
	}
}

func TestCompleteCollectionRequiresExpectedCount(t *testing.T) {
	catalog := mustLoad(t)
	collections := cloneCollections(catalog.collections)
	collections[0].Status = "complete"
	collections[0].ExpectedCount = 0
	err := validateCorpus(corpusData{
		works:          catalog.Works(),
		editions:       catalog.editions,
		collections:    collections,
		normalizations: catalog.normalizations,
		digitalRecords: mustDigitalRecords(t),
	}, false)
	if err == nil || !strings.Contains(err.Error(), "complete collection requires expectedCount") {
		t.Fatalf("validateCorpus() error = %v", err)
	}
}

func TestCompleteCollectionRequiresContiguousPositions(t *testing.T) {
	position := 2
	collection := Collection{
		ID:     "test-collection",
		Title:  LocalizedText{Hans: "测试集", Hant: "測試集"},
		Status: "complete", PrimaryEditionID: "test-edition", ExpectedCount: 1,
		Members: []CollectionMember{{WorkID: "test-work", Position: &position, PositionStatus: "confirmed"}},
	}
	problems := strings.Join(validateCollection("collection", collection, map[string]Edition{"test-edition": {ID: "test-edition"}}), "; ")
	if !strings.Contains(problems, "positions must be contiguous from 1") {
		t.Fatalf("validateCollection() problems = %q", problems)
	}
}

func TestCollectionRejectsNegativeExpectedCount(t *testing.T) {
	catalog := mustLoad(t)
	collections := cloneCollections(catalog.collections)
	collections[0].ExpectedCount = -1
	err := validateCorpus(corpusData{
		works:          catalog.Works(),
		editions:       catalog.editions,
		collections:    collections,
		normalizations: catalog.normalizations,
		digitalRecords: mustDigitalRecords(t),
	}, false)
	if err == nil || !strings.Contains(err.Error(), "expectedCount must not be negative") {
		t.Fatalf("validateCorpus() error = %v", err)
	}
}

func TestWorkRejectsDuplicateSectionID(t *testing.T) {
	catalog := mustLoad(t)
	works := catalog.Works()
	works[0].Sections = append(works[0].Sections, works[0].Sections[0])
	err := validateCorpus(corpusData{
		works:          works,
		editions:       catalog.editions,
		collections:    catalog.collections,
		normalizations: catalog.normalizations,
		digitalRecords: mustDigitalRecords(t),
	}, false)
	if err == nil || !strings.Contains(err.Error(), "duplicate section id") {
		t.Fatalf("validateCorpus() error = %v", err)
	}
}

func TestNormalizationOverridesSupportReplacementInsertionAndDeletion(t *testing.T) {
	tests := []struct {
		name     string
		hans     string
		override NormalizationOverride
	}{
		{
			name: "multi-character replacement",
			hans: "甲戊己丁。",
			override: NormalizationOverride{
				Location: VariantLocation{LineID: "line-1", Start: 1, End: 3},
				From:     "乙丙", To: "戊己", Reason: "语境替换",
			},
		},
		{
			name: "insertion",
			hans: "甲戊乙丙丁。",
			override: NormalizationOverride{
				Location: VariantLocation{LineID: "line-1", Start: 1, End: 1},
				From:     "", To: "戊", Reason: "简体展示增字",
			},
		},
		{
			name: "deletion",
			hans: "甲丙丁。",
			override: NormalizationOverride{
				Location: VariantLocation{LineID: "line-1", Start: 1, End: 2},
				From:     "乙", To: "", Reason: "简体展示脱字",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			work := normalizationTestWork(test.hans, []NormalizationOverride{test.override})
			if problems := validateNormalization("work", work, nil); len(problems) != 0 {
				t.Fatalf("validateNormalization() problems = %v", problems)
			}
		})
	}
}

func TestNormalizationOverridesRejectInvalidRangesAndOverlap(t *testing.T) {
	tests := []struct {
		name      string
		overrides []NormalizationOverride
		want      string
	}{
		{
			name: "past end",
			overrides: []NormalizationOverride{{
				Location: VariantLocation{LineID: "line-1", Start: 3, End: 5},
				From:     "丁", To: "戊", Reason: "越界",
			}},
			want: "invalid zero-based half-open Unicode range",
		},
		{
			name: "source mismatch",
			overrides: []NormalizationOverride{{
				Location: VariantLocation{LineID: "line-1", Start: 1, End: 2},
				From:     "丙", To: "戊", Reason: "错误来源",
			}},
			want: "from does not match",
		},
		{
			name: "overlap",
			overrides: []NormalizationOverride{
				{Location: VariantLocation{LineID: "line-1", Start: 0, End: 2}, From: "甲乙", To: "戊己", Reason: "第一处"},
				{Location: VariantLocation{LineID: "line-1", Start: 1, End: 3}, From: "乙丙", To: "庚辛", Reason: "第二处"},
			},
			want: "overlapping ranges",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			work := normalizationTestWork("甲乙丙丁。", test.overrides)
			problems := strings.Join(validateNormalization("work", work, nil), "; ")
			if !strings.Contains(problems, test.want) {
				t.Fatalf("validateNormalization() problems = %q, want %q", problems, test.want)
			}
		})
	}
}

func TestNormalizationIncludesTune(t *testing.T) {
	work := normalizationTestWork("甲乙丙丁。", nil)
	work.Tune = &LocalizedText{Hans: "词牌", Hant: "詞牌"}
	rule := NormalizationRule{From: "詞", To: "词", Reason: "通用简繁规则"}
	if problems := validateNormalization("work", work, []NormalizationRule{rule}); len(problems) != 0 {
		t.Fatalf("validateNormalization() problems = %v", problems)
	}
	if !workUsesRule(work, rule) {
		t.Fatal("workUsesRule() did not inspect tune")
	}
	if problems := strings.Join(validateNormalization("work", work, nil), "; "); !strings.Contains(problems, ".tune: normalization produced") {
		t.Fatalf("missing tune normalization problem: %q", problems)
	}
}

func TestVariantRangesSupportMultiCharacterAdditionAndOmission(t *testing.T) {
	tests := []struct {
		name       string
		selected   string
		witnessOne string
		witnessTwo string
		location   VariantLocation
		chosen     string
		readingOne string
		readingTwo string
	}{
		{
			name:       "multi-character replacement",
			selected:   "甲乙丙丁",
			witnessOne: "甲乙丙丁",
			witnessTwo: "甲戊己丁",
			location:   VariantLocation{LineID: "line-1", Start: 1, End: 3},
			chosen:     "乙丙",
			readingOne: "乙丙",
			readingTwo: "戊己",
		},
		{
			name:       "selected text added relative to witness",
			selected:   "甲乙丙丁",
			witnessOne: "甲乙丙丁",
			witnessTwo: "甲丁",
			location:   VariantLocation{LineID: "line-1", Start: 1, End: 3},
			chosen:     "乙丙",
			readingOne: "乙丙",
			readingTwo: "",
		},
		{
			name:       "selected text omits witness characters",
			selected:   "甲丁",
			witnessOne: "甲丁",
			witnessTwo: "甲乙丙丁",
			location:   VariantLocation{LineID: "line-1", Start: 1, End: 1},
			chosen:     "",
			readingOne: "",
			readingTwo: "乙丙",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			work := evidenceTestWork(test.selected, test.witnessOne, test.witnessTwo, Variant{
				Location: test.location,
				Readings: []VariantReading{
					{EditionID: "edition-one", Text: test.readingOne},
					{EditionID: "edition-two", Text: test.readingTwo},
				},
				Chosen: test.chosen, Rationale: "测试异文范围",
			})
			lines := map[string]Line{"line-1": work.Sections[0].Lines[0]}
			editions := map[string]Edition{"edition-one": {ID: "edition-one"}, "edition-two": {ID: "edition-two"}}
			if problems := validateEvidence("evidence", work, lines, editions); len(problems) != 0 {
				t.Fatalf("validateEvidence() problems = %v", problems)
			}
		})
	}
}

func TestVariantRangeRejectsInvalidBoundariesWithoutPanicking(t *testing.T) {
	locations := []VariantLocation{
		{LineID: "line-1", Start: -1, End: 1},
		{LineID: "line-1", Start: 1, End: 100},
	}
	for _, location := range locations {
		work := evidenceTestWork("甲乙", "甲乙", "甲丙", Variant{
			Location: location,
			Readings: []VariantReading{
				{EditionID: "edition-one", Text: "乙"},
				{EditionID: "edition-two", Text: "丙"},
			},
			Chosen: "乙", Rationale: "越界测试",
		})
		lines := map[string]Line{"line-1": work.Sections[0].Lines[0]}
		editions := map[string]Edition{"edition-one": {ID: "edition-one"}, "edition-two": {ID: "edition-two"}}
		problems := strings.Join(validateEvidence("evidence", work, lines, editions), "; ")
		if !strings.Contains(problems, "invalid zero-based half-open Unicode range") {
			t.Fatalf("validateEvidence(%#v) problems = %q", location, problems)
		}
	}
}

func TestVariantRangesRejectOverlap(t *testing.T) {
	first := Variant{
		Location: VariantLocation{LineID: "line-1", Start: 0, End: 2}, Chosen: "甲乙", Rationale: "第一处",
		Readings: []VariantReading{{EditionID: "edition-one", Text: "甲乙"}, {EditionID: "edition-two", Text: "甲乙"}},
	}
	second := Variant{
		Location: VariantLocation{LineID: "line-1", Start: 1, End: 3}, Chosen: "乙丙", Rationale: "第二处",
		Readings: []VariantReading{{EditionID: "edition-one", Text: "乙丙"}, {EditionID: "edition-two", Text: "乙丙"}},
	}
	work := evidenceTestWork("甲乙丙丁", "甲乙丙丁", "甲乙丙丁", first)
	work.Evidence.Variants = append(work.Evidence.Variants, second)
	lines := map[string]Line{"line-1": work.Sections[0].Lines[0]}
	editions := map[string]Edition{"edition-one": {ID: "edition-one"}, "edition-two": {ID: "edition-two"}}
	problems := strings.Join(validateEvidence("evidence", work, lines, editions), "; ")
	if !strings.Contains(problems, "overlapping variant ranges") {
		t.Fatalf("validateEvidence() problems = %q", problems)
	}
}

func evidenceTestWork(selected, witnessOne, witnessTwo string, variant Variant) Work {
	return Work{
		Sections: []Section{{ID: "stanza-1", Kind: "stanza", Lines: []Line{{ID: "line-1", Hans: selected + "。", Hant: selected + "。"}}}},
		Evidence: WorkEvidence{
			Level:  EvidencePrimaryScanReviewed,
			Status: "verified",
			Witnesses: []Witness{
				{EditionID: "edition-one", ScanPage: "1", PrintedFolio: "一", Verses: []string{witnessOne}},
				{EditionID: "edition-two", ScanPage: "2", PrintedFolio: "二", Verses: []string{witnessTwo}},
			},
			Variants:     []Variant{variant},
			ReviewedAt:   "2026-09-05",
			ReviewMethod: "单元测试",
		},
	}
}

func normalizationTestWork(hans string, overrides []NormalizationOverride) Work {
	return Work{
		Title:  LocalizedText{Hans: "题", Hant: "题"},
		Author: Author{Name: LocalizedText{Hans: "甲", Hant: "甲"}},
		Sections: []Section{{
			ID: "stanza-1", Kind: "stanza",
			Lines: []Line{{ID: "line-1", Hans: hans, Hant: "甲乙丙丁。"}},
		}},
		NormalizationOverrides: overrides,
	}
}

func TestIsValidScanPage(t *testing.T) {
	tests := map[string]bool{
		"1":       true,
		"135-136": true,
		"0":       false,
		"01":      false,
		"136-135": false,
		"135-135": false,
		"page-1":  false,
	}
	for value, want := range tests {
		if got := isValidScanPage(value); got != want {
			t.Errorf("isValidScanPage(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestRandomIndexFromRejectsEmptyCollection(t *testing.T) {
	if _, err := randomIndexFrom(bytes.NewReader(nil), 0); err == nil {
		t.Fatal("randomIndexFrom(length=0) error = nil")
	}
}

func mustLoad(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return catalog
}

type overlayFS struct {
	base  fs.FS
	files fstest.MapFS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if _, exists := o.files[name]; exists {
		return o.files.Open(name)
	}
	return o.base.Open(name)
}

func mustDigitalRecords(t *testing.T) map[string][]digitalRecord {
	t.Helper()
	data, err := loadCorpus(embeddedCorpus, nil)
	if err != nil {
		t.Fatal(err)
	}
	return data.digitalRecords
}

func BenchmarkRandomSongCiLengthFilter(b *testing.B) {
	catalog, err := Load()
	if err != nil {
		b.Fatal(err)
	}
	query := Query{Collection: "songci-digital-selection", MaxChars: 120, Script: ScriptHans}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := catalog.RandomWork(query); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLengthFilterUsesSelectedScriptAndIncludesPreface(t *testing.T) {
	catalog := newCatalog(corpusData{works: []Work{{Sections: []Section{
		{Kind: "preface", Lines: []Line{{Hans: "序。", Hant: "序。"}}},
		{Kind: "stanza", Lines: []Line{{Hans: "甲，乙。", Hant: "甲乙丙。"}}},
	}}}})
	if _, err := catalog.RandomWork(Query{MaxChars: 3, Script: ScriptHans}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.RandomWork(Query{MaxChars: 3, Script: ScriptHant}); !errors.Is(err, ErrNoMatchingWorks) {
		t.Fatalf("hant boundary: %v", err)
	}
	if _, err := catalog.RandomWork(Query{MaxChars: 2, Script: ScriptHans}); !errors.Is(err, ErrNoMatchingWorks) {
		t.Fatalf("preface boundary: %v", err)
	}
	if _, err := catalog.RandomWork(Query{MaxChars: 4, Script: ScriptHant}); err != nil {
		t.Fatal(err)
	}
}
