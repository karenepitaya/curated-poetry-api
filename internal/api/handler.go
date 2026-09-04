package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	poetry "github.com/karenepitaya/curated-poetry-api"
)

type WorkSource interface {
	Stats() poetry.CorpusStats
	RandomWork(query poetry.Query) (poetry.Work, error)
}

type Handler struct {
	source  WorkSource
	version string
	mux     *http.ServeMux
}

func New(source WorkSource, version string) http.Handler {
	h := &Handler{source: source, version: version, mux: http.NewServeMux()}
	h.mux.HandleFunc("/api/v1/works/random", h.randomWork)
	h.mux.HandleFunc("/healthz", h.health)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) randomWork(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	query, err := parseWorkQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_parameter", err.Error())
		return
	}
	work, err := h.source.RandomWork(query)
	if errors.Is(err, poetry.ErrNoMatchingWorks) {
		writeError(w, http.StatusNotFound, "no_matching_work", "no work matches the requested filters")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "selection_failed", "unable to select a work")
		return
	}
	response, err := renderWork(work, query.Script)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid_catalog", "selected work is invalid")
		return
	}
	writeJSON(w, http.StatusOK, randomWorkResponse{Data: response, Lang: languageTag(query.Script)})
}

var workQueryParameters = map[string]struct{}{
	"collection": {},
	"dynasty":    {},
	"genre":      {},
	"form":       {},
	"meter":      {},
	"max_chars":  {},
	"script":     {},
}

func parseWorkQuery(r *http.Request) (poetry.Query, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return poetry.Query{}, fmt.Errorf("malformed query: %w", err)
	}
	for key, items := range values {
		if _, allowed := workQueryParameters[key]; !allowed {
			return poetry.Query{}, fmt.Errorf("unknown parameter %q", key)
		}
		if len(items) != 1 || strings.TrimSpace(items[0]) == "" {
			return poetry.Query{}, fmt.Errorf("parameter %q must be specified exactly once with a non-empty value", key)
		}
	}

	query := poetry.Query{Script: poetry.ScriptHans}
	query.Collection = values.Get("collection")
	query.Dynasty = values.Get("dynasty")
	query.Genre = values.Get("genre")
	query.Form = values.Get("form")
	query.Meter = values.Get("meter")
	if raw := values.Get("max_chars"); raw != "" {
		maxChars, err := strconv.Atoi(raw)
		if err != nil || maxChars < 1 || maxChars > 5000 {
			return poetry.Query{}, errors.New("max_chars must be an integer between 1 and 5000")
		}
		query.MaxChars = maxChars
	}
	if raw := values.Get("script"); raw != "" {
		query.Script = poetry.Script(raw)
	}
	if err := validatePublicQuery(query); err != nil {
		return poetry.Query{}, err
	}
	return query, nil
}

func validatePublicQuery(query poetry.Query) error {
	if query.Collection != "" && query.Collection != "tangshi-sanbaishou-1933" && query.Collection != "songci-sanbaishou-zhu" && query.Collection != "songci-digital-selection" && query.Collection != "supplemental-classics" {
		return fmt.Errorf("unsupported collection %q", query.Collection)
	}
	return poetry.ValidateQuery(query)
}

func renderWork(work poetry.Work, script poetry.Script) (responseWork, error) {
	if work.ID == "" || work.Title.For(script) == "" || work.Author.Name.For(script) == "" || len(work.Sections) == 0 || len(work.Collections) == 0 || work.Evidence.Level == "" {
		return responseWork{}, errors.New("incomplete work")
	}
	sections := make([]responseSection, len(work.Sections))
	for i, section := range work.Sections {
		if section.Kind == "" || len(section.Lines) == 0 {
			return responseWork{}, errors.New("incomplete section")
		}
		sections[i].Kind = section.Kind
		for _, line := range section.Lines {
			text := line.Text(script)
			if strings.TrimSpace(text) == "" {
				return responseWork{}, errors.New("empty line")
			}
			sections[i].Lines = append(sections[i].Lines, text)
		}
	}
	collections := make([]responseCollection, len(work.Collections))
	for i, collection := range work.Collections {
		collections[i].ID = collection.ID
		switch collection.PositionStatus {
		case "pending":
			// An unverified internal position is never part of the public response.
		case "confirmed":
			if collection.Position == nil || *collection.Position <= 0 {
				return responseWork{}, errors.New("confirmed collection position is missing")
			}
			position := *collection.Position
			collections[i].Position = &position
		default:
			return responseWork{}, errors.New("invalid collection position status")
		}
	}
	response := responseWork{
		ID:            work.ID,
		Title:         work.Title.For(script),
		Author:        responseAuthor{Name: work.Author.Name.For(script)},
		Dynasty:       codedValue{Code: work.Dynasty, Name: dynastyName(work.Dynasty)},
		Genre:         codedValue{Code: work.Genre, Name: genreName(work.Genre, script)},
		Form:          codedValue{Code: work.Form, Name: formName(work.Form, script)},
		Sections:      sections,
		Collections:   collections,
		EvidenceLevel: work.Evidence.Level,
	}
	if work.Author.AttributionStatus != "selected-edition" || work.Author.AttributionNote != "" {
		response.Author.AttributionStatus = work.Author.AttributionStatus
		response.Author.AttributionNote = work.Author.AttributionNote
	}
	if work.Tune != nil {
		response.Tune = &namedValue{Name: work.Tune.For(script)}
	}
	return response, nil
}

func dynastyName(code string) string {
	switch code {
	case poetry.DynastyTang:
		return "唐"
	case poetry.DynastySong:
		return "宋"
	default:
		return code
	}
}

func genreName(code string, script poetry.Script) string {
	if code == poetry.GenreCi {
		if script == poetry.ScriptHant {
			return "詞"
		}
		return "词"
	}
	if script == poetry.ScriptHant {
		return "詩"
	}
	return "诗"
}

func formName(code string, script poetry.Script) string {
	namesHans := map[string]string{poetry.FormGushi: "古诗", poetry.FormLushi: "律诗", poetry.FormJueju: "绝句", poetry.FormCi: "词"}
	namesHant := map[string]string{poetry.FormGushi: "古詩", poetry.FormLushi: "律詩", poetry.FormJueju: "絕句", poetry.FormCi: "詞"}
	if script == poetry.ScriptHant {
		return namesHant[code]
	}
	return namesHans[code]
}

func languageTag(script poetry.Script) string {
	if script == poetry.ScriptHant {
		return "zh-Hant"
	}
	return "zh-Hans"
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	stats := h.source.Stats()
	writeJSON(w, http.StatusOK, healthResponse{
		Status:  "ok",
		Version: h.version,
		Works:   stats.Works,
		Dynasties: dynastyCounts{
			Tang: stats.ByDynasty[poetry.DynastyTang],
			Song: stats.ByDynasty[poetry.DynastySong],
		},
		CorpusRevision: stats.CorpusRevision,
	})
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, OPTIONS")
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
}

type randomWorkResponse struct {
	Data responseWork `json:"data"`
	Lang string       `json:"lang"`
}

type responseWork struct {
	ID            string               `json:"id"`
	Title         string               `json:"title"`
	Author        responseAuthor       `json:"author"`
	Dynasty       codedValue           `json:"dynasty"`
	Genre         codedValue           `json:"genre"`
	Form          codedValue           `json:"form"`
	Tune          *namedValue          `json:"tune,omitempty"`
	Sections      []responseSection    `json:"sections"`
	Collections   []responseCollection `json:"collections"`
	EvidenceLevel string               `json:"evidenceLevel"`
}

type responseAuthor struct {
	Name              string `json:"name"`
	AttributionStatus string `json:"attributionStatus,omitempty"`
	AttributionNote   string `json:"attributionNote,omitempty"`
}

type responseSection struct {
	Kind  string   `json:"kind"`
	Lines []string `json:"lines"`
}

type responseCollection struct {
	ID       string `json:"id"`
	Position *int   `json:"position,omitempty"`
}

type codedValue struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type namedValue struct {
	Name string `json:"name"`
}

type healthResponse struct {
	Status         string        `json:"status"`
	Version        string        `json:"version"`
	Works          int           `json:"works"`
	Dynasties      dynastyCounts `json:"dynasties"`
	CorpusRevision string        `json:"corpusRevision"`
}

type dynastyCounts struct {
	Tang int `json:"tang"`
	Song int `json:"song"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var response errorResponse
	response.Error.Code = code
	response.Error.Message = message
	writeJSON(w, status, response)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
