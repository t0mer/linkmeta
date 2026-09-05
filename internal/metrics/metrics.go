package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// RequestsTotal counts HTTP requests by route pattern and status code.
	RequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "linkmeta_http_requests_total",
		Help: "HTTP requests by path and status code.",
	}, []string{"path", "code"})

	// RequestDuration observes HTTP request latency by route pattern.
	RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "linkmeta_http_request_duration_seconds",
		Help:    "HTTP request duration by path.",
		Buckets: prometheus.DefBuckets,
	}, []string{"path"})

	// LLMCallsTotal counts LLM calls by outcome (ok|error).
	LLMCallsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "linkmeta_llm_calls_total",
		Help: "LLM calls by outcome.",
	}, []string{"outcome"})

	// LLMDuration observes LLM call latency.
	LLMDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "linkmeta_llm_duration_seconds",
		Help:    "LLM call duration.",
		Buckets: []float64{1, 5, 15, 30, 60, 120, 180, 300},
	})

	// CacheTotal counts cache outcomes (hit|miss|bypass|error).
	CacheTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "linkmeta_cache_total",
		Help: "Cache lookups by result.",
	}, []string{"result"})

	// FallbackTotal counts requests degraded to category=Other on LLM failure.
	FallbackTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "linkmeta_fallback_total",
		Help: "Requests degraded to category=Other due to LLM failure.",
	})
)

var once sync.Once

// MustRegister registers all collectors exactly once (safe for tests + main).
func MustRegister(r prometheus.Registerer) {
	once.Do(func() {
		r.MustRegister(RequestsTotal, RequestDuration, LLMCallsTotal, LLMDuration, FallbackTotal, CacheTotal)
	})
}

// Handler returns the Prometheus exposition handler.
func Handler() http.Handler { return promhttp.Handler() }
