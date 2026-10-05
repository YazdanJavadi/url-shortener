// Package metrics defines Prometheus metrics for the URL shortener and exposes
// them on a /metrics endpoint.
//
// Why a histogram (not an average) for latency:
//   - A single average hides the tail. A few slow requests (p99) are invisible
//     in a mean when most requests are fast — exactly the requests that hurt.
//   - A histogram records the full distribution, so Prometheus can compute
//     percentiles (p50/p95/p99) via histogram_quantile(). p99 ("99% of
//     requests are faster than X") is the SRE standard for latency SLOs because
//     it bounds the worst experience most users see, which an average cannot.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the instruments for the HTTP layer.
type Metrics struct {
	RequestsTotal *prometheus.CounterVec
	Latency       *prometheus.HistogramVec
	registry      *prometheus.Registry
}

// New registers the metrics on a fresh registry and returns them.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		RequestsTotal: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "urlshort_requests_total",
			Help: "Total HTTP requests, by method/route/status.",
		}, []string{"method", "route", "status"}),
		Latency: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "urlshort_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	// Include Go runtime + process metrics for free.
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	return m
}

// Handler returns the HTTP handler that exposes the metrics registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}
