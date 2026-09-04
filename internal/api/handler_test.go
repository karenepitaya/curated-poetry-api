package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	poetry "github.com/karenepitaya/curated-poetry-api"
)

type fakeSource struct {
	work      poetry.Work
	workErr   error
	lastQuery poetry.Query
	workCalls int
	revision  string
}

func (f *fakeSource) Count() int { return 50 }

func (f *fakeSource) Stats() poetry.CorpusStats {
	return poetry.CorpusStats{
		Works:          50,
		ByDynasty:      map[string]int{poetry.DynastyTang: 50, poetry.DynastySong: 0},
		CorpusRevision: f.revision,
	}
}

func (f *fakeSource) RandomWork(query poetry.Query) (poetry.Work, error) {
	f.workCalls++
	f.lastQuery = query
	return f.work, f.workErr
}

func testWork() poetry.Work {
	return poetry.Work{
		ID:      "tang-li-bai-jing-ye-si",
		Title:   poetry.LocalizedText{Hans: "静夜思", Hant: "靜夜思"},
		Author:  poetry.Author{Name: poetry.LocalizedText{Hans: "李白", Hant: "李白"}, AttributionStatus: "selected-edition"},
		Dynasty: poetry.DynastyTang,
		Genre:   poetry.GenreShi,
		Form:    poetry.FormJueju,
		Meter:   poetry.MeterFive,
		Sections: []poetry.Section{{
			ID:   "stanza-1",
			Kind: "stanza",
			Lines: []poetry.Line{
				{ID: "line-1", Hans: "床前明月光，", Hant: "牀前明月光，"},
				{ID: "line-2", Hans: "疑是地上霜。", Hant: "疑是地上霜。"},
				{ID: "line-3", Hans: "举头望明月，", Hant: "舉頭望明月，"},
				{ID: "line-4", Hans: "低头思故乡。", Hant: "低頭思故鄉。"},
			},
		}},
		Collections: []poetry.WorkCollection{{ID: "tangshi-sanbaishou-1933", PositionStatus: "pending"}},
		Evidence:    poetry.WorkEvidence{Level: poetry.EvidencePrimaryScanReviewed},
	}
}

func TestRandomWorkResponseAndFilters(t *testing.T) {
	work := testWork()
	unverifiedPosition := 9
	work.Collections[0].Position = &unverifiedPosition
	source := &fakeSource{work: work}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random?collection=tangshi-sanbaishou-1933&dynasty=tang&genre=shi&form=jueju&meter=5&max_chars=20&script=hans", nil)

	New(source, "v0.2.0").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	wantQuery := poetry.Query{Collection: "tangshi-sanbaishou-1933", Dynasty: "tang", Genre: "shi", Form: "jueju", Meter: "5", MaxChars: 20, Script: poetry.ScriptHans}
	if !reflect.DeepEqual(source.lastQuery, wantQuery) {
		t.Fatalf("query = %#v, want %#v", source.lastQuery, wantQuery)
	}

	var response randomWorkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Lang != "zh-Hans" || response.Data.ID != source.work.ID || response.Data.Title != "静夜思" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if got := response.Data.Sections[0].Lines[0]; got != "床前明月光，" {
		t.Fatalf("first line = %q", got)
	}
	if response.Data.Dynasty.Code != "tang" || response.Data.Genre.Name != "诗" || response.Data.Form.Name != "绝句" {
		t.Fatalf("coded metadata = %#v %#v %#v", response.Data.Dynasty, response.Data.Genre, response.Data.Form)
	}
	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	collection := raw["data"].(map[string]any)["collections"].([]any)[0].(map[string]any)
	if _, exists := collection["position"]; exists {
		t.Fatalf("pending collection position must be omitted: %#v", collection)
	}
	author := raw["data"].(map[string]any)["author"].(map[string]any)
	if _, exists := author["attributionStatus"]; exists {
		t.Fatalf("ordinary attribution status should be omitted: %#v", author)
	}
}

func TestRandomWorkDefaultsToHans(t *testing.T) {
	source := &fakeSource{work: testWork()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body)
	}
	if source.lastQuery.Script != poetry.ScriptHans {
		t.Fatalf("default script = %q", source.lastQuery.Script)
	}
}

func TestRandomWorkIncludesConfirmedCollectionPosition(t *testing.T) {
	work := testWork()
	position := 9
	work.Collections[0].Position = &position
	work.Collections[0].PositionStatus = "confirmed"
	source := &fakeSource{work: work}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	var response randomWorkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Collections[0].Position == nil || *response.Data.Collections[0].Position != 9 {
		t.Fatalf("confirmed position = %#v", response.Data.Collections[0])
	}
}

func TestRandomWorkRendersHant(t *testing.T) {
	source := &fakeSource{work: testWork()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random?script=hant", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	var response randomWorkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Lang != "zh-Hant" || response.Data.Title != "靜夜思" || response.Data.Sections[0].Lines[0] != "牀前明月光，" {
		t.Fatalf("traditional response = %#v", response)
	}
	if response.Data.Genre.Name != "詩" || response.Data.Form.Name != "絕句" {
		t.Fatalf("traditional labels = %#v %#v", response.Data.Genre, response.Data.Form)
	}
}

func TestRandomWorkDisclosesUncertainAttribution(t *testing.T) {
	work := testWork()
	work.Author.AttributionStatus = "disputed"
	work.Author.AttributionNote = "选本署名与后世考证不一"
	source := &fakeSource{work: work}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	var response randomWorkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Author.AttributionStatus != "disputed" || response.Data.Author.AttributionNote == "" {
		t.Fatalf("author attribution = %#v", response.Data.Author)
	}
}

func TestInvalidWorkParameters(t *testing.T) {
	tests := []string{
		"?unknown=value",
		"?genre=",
		"?genre=shi&genre=ci",
		"?collection=unknown",
		"?dynasty=yuan",
		"?genre=qu",
		"?form=pailu",
		"?meter=6",
		"?max_chars=0",
		"?max_chars=5001",
		"?max_chars=abc",
		"?script=latin",
		"?genre=%zz",
	}
	for _, suffix := range tests {
		t.Run(suffix, func(t *testing.T) {
			source := &fakeSource{work: testWork()}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random"+suffix, nil)
			New(source, "dev").ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body)
			}
			if source.workCalls != 0 {
				t.Fatalf("RandomWork calls = %d, want 0", source.workCalls)
			}
			assertErrorShape(t, recorder, "invalid_parameter")
		})
	}
}

func TestNoMatchingWorkReturns404(t *testing.T) {
	source := &fakeSource{work: testWork(), workErr: poetry.ErrNoMatchingWorks}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random?collection=songci-sanbaishou-zhu", nil)
	New(source, "dev").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	assertErrorShape(t, recorder, "no_matching_work")
}

func TestWorkSelectionFailureReturns500(t *testing.T) {
	source := &fakeSource{work: testWork(), workErr: errors.New("entropy unavailable")}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random", nil)
	New(source, "dev").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	assertErrorShape(t, recorder, "selection_failed")
}

func TestMalformedWorkDoesNotPanic(t *testing.T) {
	work := testWork()
	work.Sections = nil
	source := &fakeSource{work: work}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/works/random", nil)
	New(source, "dev").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	assertErrorShape(t, recorder, "invalid_catalog")
}

func TestRemovedLegacyRouteReturns404(t *testing.T) {
	source := &fakeSource{work: testWork()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/poems/random", nil)

	New(source, "v0.2.1").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", recorder.Code, recorder.Body)
	}
	if source.workCalls != 0 {
		t.Fatalf("RandomWork calls = %d, want 0", source.workCalls)
	}
}

func TestWrongMethod(t *testing.T) {
	source := &fakeSource{work: testWork()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/works/random", nil)
	New(source, "dev").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET, OPTIONS" {
		t.Fatalf("Allow = %q, want GET, OPTIONS", got)
	}
	assertErrorShape(t, recorder, "method_not_allowed")
}

func TestHealth(t *testing.T) {
	source := &fakeSource{work: testWork(), revision: "abc123"}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	New(source, "v0.2.0").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response healthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.Status != "ok" || response.Version != "v0.2.0" || response.Works != 50 || response.Dynasties.Tang != 50 || response.Dynasties.Song != 0 || response.CorpusRevision != "abc123" {
		t.Fatalf("health response = %#v", response)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 5 {
		t.Fatalf("health top-level fields = %v, want exactly 5", keys)
	}
}

func TestOptionsAdvertisesReadOnlyCORS(t *testing.T) {
	source := &fakeSource{work: testWork()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/works/random", nil)
	New(source, "dev").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); got != "GET, OPTIONS" {
		t.Fatalf("Access-Control-Allow-Methods = %q", got)
	}
}

func assertErrorShape(t *testing.T, recorder *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != wantCode || response.Error.Message == "" {
		t.Fatalf("error response = %#v", response)
	}
}

func TestEmbeddedSongCiEndpoint(t *testing.T) {
	catalog, err := poetry.Load()
	if err != nil {
		t.Fatal(err)
	}
	handler := New(catalog, "test")
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var status struct {
		Works     int `json:"works"`
		Dynasties struct {
			Tang int `json:"tang"`
			Song int `json:"song"`
		} `json:"dynasties"`
	}
	if err := json.Unmarshal(health.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if health.Code != http.StatusOK || status.Works != 326 || status.Dynasties.Tang != 50 || status.Dynasties.Song != 276 {
		t.Fatalf("health = %s", health.Body.String())
	}
	for _, script := range []string{"hans", "hant"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/works/random?collection=songci-digital-selection&dynasty=song&genre=ci&max_chars=120&script="+script, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", script, recorder.Code, recorder.Body.String())
		}
		var response randomWorkResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Data.EvidenceLevel != poetry.EvidenceDigitalTextChecked || response.Data.Tune == nil || response.Data.Dynasty.Code != "song" || len(response.Data.Sections) == 0 {
			t.Fatalf("response = %#v", response)
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
	}
}
