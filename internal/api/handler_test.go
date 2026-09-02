package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	poetry "github.com/karenepitaya/curated-poetry-api"
)

type fakeSource struct {
	poem     poetry.Poem
	err      error
	lastType string
	calls    int
}

func (f *fakeSource) Count() int { return 50 }

func (f *fakeSource) Random(typeName string) (poetry.Poem, error) {
	f.calls++
	f.lastType = typeName
	return f.poem, f.err
}

func testPoem() poetry.Poem {
	return poetry.Poem{
		ID:      "tang-li-bai-jing-ye-si",
		Title:   "静夜思",
		Author:  "李白",
		Dynasty: "唐",
		Type:    poetry.TypeFiveCharacter,
		Verses:  []string{"床前明月光", "疑是地上霜", "举头望明月", "低头思故乡"},
	}
}

func TestRandomPoemResponse(t *testing.T) {
	source := &fakeSource{poem: testPoem()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/poems/random?type="+poetry.TypeFiveCharacter, nil)

	New(source, "v0.1.0").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if source.lastType != poetry.TypeFiveCharacter {
		t.Errorf("Random type = %q, want %q", source.lastType, poetry.TypeFiveCharacter)
	}

	var response randomResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Lang != "zh-Hans" || response.Data.ID != source.poem.ID {
		t.Fatalf("unexpected response: %#v", response)
	}
	if got, want := response.Data.Content[0], "床前明月光，疑是地上霜。"; got != want {
		t.Fatalf("first couplet = %q, want %q", got, want)
	}
}

func TestRandomPoemWithoutType(t *testing.T) {
	source := &fakeSource{poem: testPoem()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/poems/random", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if source.lastType != "" {
		t.Errorf("Random type = %q, want empty", source.lastType)
	}
}

func TestInvalidType(t *testing.T) {
	source := &fakeSource{poem: testPoem()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/poems/random?type=律诗", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if source.calls != 0 {
		t.Fatalf("Random calls = %d, want 0", source.calls)
	}
}

func TestMalformedCatalogEntryDoesNotPanic(t *testing.T) {
	poem := testPoem()
	poem.Verses = poem.Verses[:3]
	source := &fakeSource{poem: poem}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/poems/random", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestSelectionFailureReturnsServerError(t *testing.T) {
	source := &fakeSource{poem: testPoem(), err: errors.New("entropy unavailable")}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/poems/random", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestWrongMethod(t *testing.T) {
	source := &fakeSource{poem: testPoem()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/poems/random", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET, OPTIONS" {
		t.Fatalf("Allow = %q, want GET, OPTIONS", got)
	}
}

func TestHealth(t *testing.T) {
	source := &fakeSource{poem: testPoem()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	New(source, "v0.1.0").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Poems   int    `json:"poems"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.Status != "ok" || response.Version != "v0.1.0" || response.Poems != 50 {
		t.Fatalf("health response = %#v", response)
	}
}

func TestOptionsAdvertisesReadOnlyCORS(t *testing.T) {
	source := &fakeSource{poem: testPoem()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/poems/random", nil)

	New(source, "dev").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); got != "GET, OPTIONS" {
		t.Fatalf("Access-Control-Allow-Methods = %q", got)
	}
}
