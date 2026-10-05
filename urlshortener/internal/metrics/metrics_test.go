package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNew_RegistersMetrics verifies the metrics instruments are registered.
func TestNew_RegistersMetrics(t *testing.T) {
	m := New()

	assert.NotNil(t, m.RequestsTotal)
	assert.NotNil(t, m.Latency)
}

// TestMetricsHandler_Exposes verifies the /metrics handler exposes the counters.
func TestMetricsHandler_Exposes(t *testing.T) {
	expectedMetric := "urlshort_requests_total"

	m := New()
	// Record a request so the counter is non-zero.
	m.RequestsTotal.WithLabelValues("GET", "/:code", "302").Inc()

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	assert.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.True(t, strings.Contains(string(body), expectedMetric))
}
