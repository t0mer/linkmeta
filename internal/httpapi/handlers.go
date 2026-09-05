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
	svc     *service.Service
	llm     llm.Client
	cfg     config.Config
	version string
	log     *slog.Logger
}

// NewAPI builds the API. version is the build-injected version string.
func NewAPI(svc *service.Service, l llm.Client, cfg config.Config, version string, log *slog.Logger) *API {
	return &API{svc: svc, llm: l, cfg: cfg, version: version, log: log}
}

// healthResponse is the /healthz body. Ollama status is one of:
//   - "ok"            server reachable and the configured model is installed
//   - "unreachable"   server did not answer /api/version
//   - "model-missing" server is up but the model is not pulled: every
//     extraction degrades to category "Other" until it is
type healthResponse struct {
	Status         string `json:"status"`
	Ollama         string `json:"ollama"`
	Model          string `json:"model"`
	Version        string `json:"version"`
	LastLLMError   string `json:"last_llm_error,omitempty"`
	LastLLMErrorAt string `json:"last_llm_error_at,omitempty"`
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
	var rawURL, lang string
	if r.Method == http.MethodPost {
		var body struct {
			URL  string `json:"url"`
			Lang string `json:"lang"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		rawURL = body.URL
		lang = body.Lang
	} else {
		rawURL = r.URL.Query().Get("url")
		lang = r.URL.Query().Get("lang")
	}

	if rawURL == "" {
		writeErr(w, http.StatusBadRequest, "missing url")
		return
	}
	if err := service.ValidateURL(rawURL); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.svc.Extract(r.Context(), rawURL, lang)
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
	} else if err := a.llm.CheckModel(ctx); err != nil {
		// The server is up but cannot serve this model, so /api/chat will fail
		// on every request. Report it distinctly instead of a bare "ok".
		ollama = "model-missing"
		a.log.Warn("configured model unavailable", "model", a.cfg.OllamaModel, "err", err)
	}
	resp := healthResponse{
		Status:  "ok",
		Ollama:  ollama,
		Model:   a.cfg.OllamaModel,
		Version: a.version,
	}
	if msg, at := a.svc.LastLLMError(); msg != "" {
		resp.LastLLMError = msg
		resp.LastLLMErrorAt = at.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}
