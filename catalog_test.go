package poetryapi

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEmbeddedCatalogLoads(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if catalog.Count() != 50 {
		t.Fatalf("Count() = %d, want 50", catalog.Count())
	}

	counts := map[string]int{}
	for _, poem := range catalog.Poems() {
		counts[poem.Type]++
		wantLength := 5
		if poem.Type == TypeSevenCharacter {
			wantLength = 7
		}
		for i, verse := range poem.Verses {
			if got := utf8.RuneCountInString(verse); got != wantLength {
				t.Errorf("%s verse %d has %d characters, want %d", poem.ID, i, got, wantLength)
			}
		}
	}
	if counts[TypeFiveCharacter] != 25 || counts[TypeSevenCharacter] != 25 {
		t.Fatalf("type counts = %#v, want 25 each", counts)
	}
}

func TestValidateRejectsDuplicateID(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	poems := catalog.Poems()
	poems[1].ID = poems[0].ID
	if err := validate(poems, catalog.evidence, catalog.editions); err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("validate() error = %v, want duplicate id", err)
	}
}

func TestValidateRejectsPunctuationInsideVerse(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	poems := catalog.Poems()
	poems[0].Verses[0] = "床前，月光"
	if err := validate(poems, catalog.evidence, catalog.editions); err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("validate() error = %v, want invalid character", err)
	}
}

func TestValidateRejectsMissingBaseWitness(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	evidence := append([]PoemEvidence(nil), catalog.evidence...)
	for evidenceIndex, item := range evidence {
		baseEditionID := requiredBaseEditionID(item.PoemID)
		item.Witnesses = append([]Witness(nil), item.Witnesses...)
		for witnessIndex, witness := range item.Witnesses {
			if witness.EditionID != baseEditionID {
				continue
			}
			item.Witnesses = append(item.Witnesses[:witnessIndex], item.Witnesses[witnessIndex+1:]...)
			evidence[evidenceIndex] = item
			want := "missing required base edition " + baseEditionID
			if err := validate(catalog.poems, evidence, catalog.editions); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("validate() error = %v, want %q", err, want)
			}
			return
		}
	}
	t.Fatal("test data has no required base witness")
}

func TestValidateRejectsMissingNormalization(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	poemIndex := -1
	for i, poem := range catalog.poems {
		if poem.Title != poem.TitleTraditional || strings.Join(poem.Verses, "") != strings.Join(poem.VersesTraditional, "") {
			poemIndex = i
			break
		}
	}
	if poemIndex < 0 {
		t.Fatal("test data has no simplified/traditional difference")
	}
	evidence := append([]PoemEvidence(nil), catalog.evidence...)
	for i := range evidence {
		if evidence[i].PoemID != catalog.poems[poemIndex].ID {
			continue
		}
		item := evidence[i]
		item.Normalizations = []Normalization{}
		evidence[i] = item
		if err := validate(catalog.poems, evidence, catalog.editions); err == nil || !strings.Contains(err.Error(), "missing normalization") {
			t.Fatalf("validate() error = %v, want missing normalization", err)
		}
		return
	}
	t.Fatal("test data has no matching evidence")
}

func TestRandomRejectsUnknownType(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, err := catalog.Random("词"); err == nil {
		t.Fatal("Random() error = nil, want unsupported type error")
	}
}

func TestRandomIndexRejectsEmptyCollection(t *testing.T) {
	if _, err := randomIndex(0); err == nil {
		t.Fatal("randomIndex(0) error = nil")
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

func TestRequiredBaseEditionID(t *testing.T) {
	if got := requiredBaseEditionID(alternateBasePoemID); got != alternateBaseEditionID {
		t.Fatalf("alternate base = %q, want %q", got, alternateBaseEditionID)
	}
	if got := requiredBaseEditionID("tang-li-bai-jing-ye-si"); got != defaultBaseEditionID {
		t.Fatalf("default base = %q, want %q", got, defaultBaseEditionID)
	}
}

func TestPoemsReturnsCopy(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	poems := catalog.Poems()
	original := poems[0].Verses[0]
	poems[0].Verses[0] = "篡改"
	if got := catalog.Poems()[0].Verses[0]; got != original {
		t.Fatalf("catalog data mutated through Poems(): got %q, want %q", got, original)
	}
}
