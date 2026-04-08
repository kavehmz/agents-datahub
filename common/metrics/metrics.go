package metrics

import (
	"fmt"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var metricNameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)

// SanitizeMetricName replaces any characters not safe for metric names
// and truncates to a reasonable length to prevent cardinality explosion.
func SanitizeMetricName(name string) string {
	sanitized := metricNameRe.ReplaceAllString(name, "_")
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}
	return sanitized
}

// Counter is a simple atomic counter
type Counter struct {
	value uint64
}

// Inc increments the counter
func (c *Counter) Inc() {
	atomic.AddUint64(&c.value, 1)
}

// Add adds a value to the counter
func (c *Counter) Add(n uint64) {
	atomic.AddUint64(&c.value, n)
}

// Get returns the current counter value
func (c *Counter) Get() uint64 {
	return atomic.LoadUint64(&c.value)
}

// Histogram tracks response time percentiles using a ring buffer.
type Histogram struct {
	mu    sync.RWMutex
	buf   []float64
	pos   int  // next write position
	full  bool // whether the buffer has wrapped around
	max   int
}

// NewHistogram creates a new histogram with max capacity
func NewHistogram(maxSize int) *Histogram {
	return &Histogram{
		buf: make([]float64, maxSize),
		max: maxSize,
	}
}

// Observe adds a value to the histogram (O(1))
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.buf[h.pos] = value
	h.pos++
	if h.pos >= h.max {
		h.pos = 0
		h.full = true
	}
}

// snapshot returns a copy of the active values (caller must hold at least RLock)
func (h *Histogram) snapshot() []float64 {
	if h.full {
		cp := make([]float64, h.max)
		copy(cp, h.buf)
		return cp
	}
	cp := make([]float64, h.pos)
	copy(cp, h.buf[:h.pos])
	return cp
}

// Quantile returns the value at the given quantile (0.0-1.0)
func (h *Histogram) Quantile(q float64) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	sorted := h.snapshot()
	if len(sorted) == 0 {
		return 0
	}

	sort.Float64s(sorted)

	index := int(float64(len(sorted)-1) * q)
	return sorted[index]
}

// Average returns the average value
func (h *Histogram) Average() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	vals := h.snapshot()
	if len(vals) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// Gauge represents a value that can go up and down
type Gauge struct {
	value int64
}

// Set sets the gauge value
func (g *Gauge) Set(value int64) {
	atomic.StoreInt64(&g.value, value)
}

// Inc increments the gauge
func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1)
}

// Dec decrements the gauge
func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1)
}

// Get returns the current gauge value
func (g *Gauge) Get() int64 {
	return atomic.LoadInt64(&g.value)
}

// Metrics holds all metrics for a service
type Metrics struct {
	// Common metrics
	QueriesTotal   Counter
	QueriesSuccess Counter
	QueriesFailed  Counter
	QueryDuration  *Histogram

	// Hub-specific metrics
	SourcesConnected      Gauge
	SourcesHealthy        Gauge
	ExposersAuthenticated Counter

	// Source-specific metrics
	ActiveConnections Gauge
	QueriesProcessed  Counter

	// Exposer-specific metrics
	ClientRequests       Counter
	ClientRequestsFailed Counter

	StartTime time.Time
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		QueryDuration: NewHistogram(10000), // Keep last 10k queries
		StartTime:     time.Now(),
	}
}

// Uptime returns the service uptime in seconds
func (m *Metrics) Uptime() int64 {
	return int64(time.Since(m.StartTime).Seconds())
}

// PrometheusExporter exports metrics in Prometheus format
type PrometheusExporter struct {
	metrics *Metrics
	service string
	labels  map[string]string
}

// NewPrometheusExporter creates a Prometheus exporter
func NewPrometheusExporter(service string, metrics *Metrics, labels map[string]string) *PrometheusExporter {
	return &PrometheusExporter{
		metrics: metrics,
		service: service,
		labels:  labels,
	}
}

// Export generates Prometheus-formatted metrics
func (e *PrometheusExporter) Export() string {
	var result string

	labelStr := e.formatLabels()

	// Common metrics
	result += fmt.Sprintf("# TYPE %s_queries_total counter\n", e.service)
	result += fmt.Sprintf("%s_queries_total%s %d\n", e.service, labelStr, e.metrics.QueriesTotal.Get())

	result += fmt.Sprintf("# TYPE %s_queries_success counter\n", e.service)
	result += fmt.Sprintf("%s_queries_success%s %d\n", e.service, labelStr, e.metrics.QueriesSuccess.Get())

	result += fmt.Sprintf("# TYPE %s_queries_failed counter\n", e.service)
	result += fmt.Sprintf("%s_queries_failed%s %d\n", e.service, labelStr, e.metrics.QueriesFailed.Get())

	result += fmt.Sprintf("# TYPE %s_query_duration_seconds summary\n", e.service)
	result += fmt.Sprintf("%s_query_duration_seconds{quantile=\"0.5\"%s} %.6f\n",
		e.service, e.trimLabels(labelStr), e.metrics.QueryDuration.Quantile(0.5)/1000)
	result += fmt.Sprintf("%s_query_duration_seconds{quantile=\"0.9\"%s} %.6f\n",
		e.service, e.trimLabels(labelStr), e.metrics.QueryDuration.Quantile(0.9)/1000)
	result += fmt.Sprintf("%s_query_duration_seconds{quantile=\"0.99\"%s} %.6f\n",
		e.service, e.trimLabels(labelStr), e.metrics.QueryDuration.Quantile(0.99)/1000)

	result += fmt.Sprintf("# TYPE %s_uptime_seconds gauge\n", e.service)
	result += fmt.Sprintf("%s_uptime_seconds%s %d\n", e.service, labelStr, e.metrics.Uptime())

	// Hub-specific metrics
	if e.service == "hub" {
		result += fmt.Sprintf("# TYPE hub_sources_connected gauge\n")
		result += fmt.Sprintf("hub_sources_connected%s %d\n", labelStr, e.metrics.SourcesConnected.Get())

		result += fmt.Sprintf("# TYPE hub_sources_healthy gauge\n")
		result += fmt.Sprintf("hub_sources_healthy%s %d\n", labelStr, e.metrics.SourcesHealthy.Get())

		result += fmt.Sprintf("# TYPE hub_exposers_authenticated counter\n")
		result += fmt.Sprintf("hub_exposers_authenticated%s %d\n", labelStr, e.metrics.ExposersAuthenticated.Get())
	}

	// Source-specific metrics
	if e.service == "source" {
		result += fmt.Sprintf("# TYPE source_active_connections gauge\n")
		result += fmt.Sprintf("source_active_connections%s %d\n", labelStr, e.metrics.ActiveConnections.Get())

		result += fmt.Sprintf("# TYPE source_queries_processed counter\n")
		result += fmt.Sprintf("source_queries_processed%s %d\n", labelStr, e.metrics.QueriesProcessed.Get())
	}

	// Exposer-specific metrics
	if e.service == "exposer" {
		result += fmt.Sprintf("# TYPE exposer_client_requests counter\n")
		result += fmt.Sprintf("exposer_client_requests%s %d\n", labelStr, e.metrics.ClientRequests.Get())

		result += fmt.Sprintf("# TYPE exposer_client_requests_failed counter\n")
		result += fmt.Sprintf("exposer_client_requests_failed%s %d\n", labelStr, e.metrics.ClientRequestsFailed.Get())
	}

	return result
}

// formatLabels formats labels for Prometheus
func (e *PrometheusExporter) formatLabels() string {
	if len(e.labels) == 0 {
		return ""
	}

	result := "{"
	first := true
	for k, v := range e.labels {
		if !first {
			result += ","
		}
		result += fmt.Sprintf("%s=\"%s\"", k, v)
		first = false
	}
	result += "}"
	return result
}

// trimLabels removes the outer braces from labels (for quantile labels)
func (e *PrometheusExporter) trimLabels(labels string) string {
	if labels == "" {
		return ""
	}
	// Remove { and }
	return "," + labels[1:len(labels)-1]
}
