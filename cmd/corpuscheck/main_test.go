package main

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFileValuesAccumulateRepeatedFlags(t *testing.T) {
	var values fileValues
	if err := values.Set("first.json"); err != nil {
		t.Fatal(err)
	}
	if err := values.Set("second.json"); err != nil {
		t.Fatal(err)
	}
	if want := (fileValues{"first.json", "second.json"}); !reflect.DeepEqual(values, want) {
		t.Fatalf("fileValues = %v, want %v", values, want)
	}
}

func TestPlanCheckRejectsMissingPathByDefault(t *testing.T) {
	missing := "corpus/works/tang/missing.json"
	_, _, err := planCheck(fstest.MapFS{}, []string{missing}, false)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("planCheck() error = %v, want missing path error", err)
	}
}

func TestPlanCheckAllowsMissingPathWithFullValidation(t *testing.T) {
	files, full, err := planCheck(fstest.MapFS{}, []string{"corpus/works/tang/deleted.json"}, true)
	if err != nil {
		t.Fatalf("planCheck() error = %v", err)
	}
	if !full || files != nil {
		t.Fatalf("planCheck() = (%v, %t), want (nil, true)", files, full)
	}
}

func TestPlanCheckUsesFullValidationForSharedMetadata(t *testing.T) {
	dataFS := fstest.MapFS{
		"corpus/normalization.json": &fstest.MapFile{Data: []byte("{}")},
	}
	files, full, err := planCheck(dataFS, []string{"corpus/normalization.json"}, false)
	if err != nil {
		t.Fatalf("planCheck() error = %v", err)
	}
	if !full || files != nil {
		t.Fatalf("planCheck() = (%v, %t), want (nil, true)", files, full)
	}
}

func TestPlanCheckUsesIncrementalValidationForExistingWorks(t *testing.T) {
	work := "corpus/works/tang/example.json"
	dataFS := fstest.MapFS{
		work: &fstest.MapFile{Data: []byte("{}")},
	}
	files, full, err := planCheck(dataFS, []string{work, work}, false)
	if err != nil {
		t.Fatalf("planCheck() error = %v", err)
	}
	if full || len(files) != 1 || files[0] != work {
		t.Fatalf("planCheck() = (%v, %t), want ([%s], false)", files, full, work)
	}
}
