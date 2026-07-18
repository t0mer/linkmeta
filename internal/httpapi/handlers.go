package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/t0mer/linkmeta/internal/config"
	"github.com/t0mer/linkmeta/internal/llm"
	"github.com/t0mer/linkmeta/internal/service"
)

// API holds handler dependencies.
type API struct {
	svc *service.Service
	llm llm.Client
	cfg config.Config
	log *slog.Logger
}

// NewAPI builds the API.
func NewAPI(svc *service.Service, l llm.Client, cfg config.Config, log *slog.Logger) *API {
	return &API{svc: svc, llm: l, cfg: cfg, log: log}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (a *API) handleExtract(w http.ResponseWriter, r *http.Request) {
	var rawURL string
	if r.Method == http.MethodPost {
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		rawURL = body.URL
	} else {
		rawURL = r.URL.Query().Get("url")
	}

	if rawURL == "" {
		writeErr(w, http.StatusBadRequest, "missing url")
		return
	}
	if err := service.ValidateURL(rawURL); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.svc.Extract(r.Context(), rawURL)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	ollama := "ok"
	if err := a.llm.Version(ctx); err != nil {
		ollama = "unreachable"
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"ollama": ollama,
		"model":  a.cfg.OllamaModel,
	})
}
