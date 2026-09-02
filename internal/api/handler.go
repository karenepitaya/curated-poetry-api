package api

import (
	"encoding/json"
	"net/http"
	"strings"

	poetry "github.com/karenepitaya/curated-poetry-api"
)

type PoemSource interface {
	Count() int
	Random(typeName string) (poetry.Poem, error)
}

type Handler struct {
	source  PoemSource
	version string
	mux     *http.ServeMux
}

func New(source PoemSource, version string) http.Handler {
	h := &Handler{source: source, version: version, mux: http.NewServeMux()}
	h.mux.HandleFunc("/api/poems/random", h.randomPoem)
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

func (h *Handler) randomPoem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, OPTIONS")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}

	values, hasType := r.URL.Query()["type"]
	typeName := ""
	if hasType {
		if len(values) != 1 {
			writeError(w, http.StatusBadRequest, "invalid_type", "type must be specified once")
			return
		}
		typeName = values[0]
		if typeName != poetry.TypeFiveCharacter && typeName != poetry.TypeSevenCharacter {
			writeError(w, http.StatusBadRequest, "invalid_type", "type must be 五言绝句 or 七言绝句")
			return
		}
	}

	poem, err := h.source.Random(typeName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "selection_failed", "unable to select a poem")
		return
	}
	if len(poem.Verses) != 4 {
		writeError(w, http.StatusInternalServerError, "invalid_catalog", "selected poem is invalid")
		return
	}
	writeJSON(w, http.StatusOK, randomResponse{
		Data: responsePoem{
			ID:      poem.ID,
			Title:   poem.Title,
			Content: []string{joinCouplet(poem.Verses[0], poem.Verses[1]), joinCouplet(poem.Verses[2], poem.Verses[3])},
			Author:  namedValue{Name: poem.Author},
			Dynasty: namedValue{Name: poem.Dynasty},
			Type:    namedValue{Name: poem.Type},
		},
		Lang: "zh-Hans",
	})
}

func joinCouplet(first, second string) string {
	return strings.Join([]string{first, second}, "，") + "。"
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, OPTIONS")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": h.version,
		"poems":   h.source.Count(),
	})
}

type randomResponse struct {
	Data responsePoem `json:"data"`
	Lang string       `json:"lang"`
}

type responsePoem struct {
	ID      string     `json:"id"`
	Title   string     `json:"title"`
	Content []string   `json:"content"`
	Author  namedValue `json:"author"`
	Dynasty namedValue `json:"dynasty"`
	Type    namedValue `json:"type"`
}

type namedValue struct {
	Name string `json:"name"`
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
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
