package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/t0mer/linkmeta/internal/metrics"
)

// Router builds the chi mux with middleware, routes and metrics.
func (a *API) Router() http.Handler {
	metrics.MustRegister(prometheus.DefaultRegisterer)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(instrument)

	r.Handle("/metrics", metrics.Handler())
	r.Get("/healthz", a.handleHealthz)
	r.Get("/extract", a.handleExtract)
	r.Post("/extract", a.handleExtract)
	return r
}

// instrument records request count + duration per route pattern.
func instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		path := chi.RouteContext(r.Context()).RoutePattern()
		if path == "" {
			path = r.URL.Path
		}
		metrics.RequestDuration.WithLabelValues(path).Observe(time.Since(start).Seconds())
		metrics.RequestsTotal.WithLabelValues(path, strconv.Itoa(ww.Status())).Inc()
	})
}
