package metrics

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// DatadogClient sends metrics to Datadog via DogStatsD (UDP)
type DatadogClient struct {
	conn    *net.UDPConn
	addr    *net.UDPAddr
	prefix  string
	tags    []string
	enabled bool
}

// NewDatadogClient creates a new Datadog client
// endpoint should be in format: "host:port" (e.g., "localhost:8125")
func NewDatadogClient(endpoint, serviceName string, tags map[string]string) (*DatadogClient, error) {
	if endpoint == "" {
		return &DatadogClient{enabled: false}, nil
	}

	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Datadog endpoint: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Datadog: %w", err)
	}

	// Build tags
	tagSlice := []string{fmt.Sprintf("service:%s", serviceName)}
	for k, v := range tags {
		tagSlice = append(tagSlice, fmt.Sprintf("%s:%s", k, v))
	}

	return &DatadogClient{
		conn:    conn,
		addr:    addr,
		prefix:  serviceName,
		tags:    tagSlice,
		enabled: true,
	}, nil
}

// Count sends a counter metric
func (d *DatadogClient) Count(name string, value int64) {
	if !d.enabled {
		return
	}
	d.send(fmt.Sprintf("%s.%s:%d|c%s", d.prefix, name, value, d.formatTags()))
}

// Gauge sends a gauge metric
func (d *DatadogClient) Gauge(name string, value float64) {
	if !d.enabled {
		return
	}
	d.send(fmt.Sprintf("%s.%s:%.2f|g%s", d.prefix, name, value, d.formatTags()))
}

// Histogram sends a histogram metric
func (d *DatadogClient) Histogram(name string, value float64) {
	if !d.enabled {
		return
	}
	d.send(fmt.Sprintf("%s.%s:%.2f|h%s", d.prefix, name, value, d.formatTags()))
}

// Timing sends a timing metric (in milliseconds)
func (d *DatadogClient) Timing(name string, duration time.Duration) {
	if !d.enabled {
		return
	}
	ms := float64(duration.Milliseconds())
	d.send(fmt.Sprintf("%s.%s:%.2f|ms%s", d.prefix, name, ms, d.formatTags()))
}

// send sends a metric to Datadog
func (d *DatadogClient) send(metric string) {
	if d.conn != nil {
		d.conn.Write([]byte(metric))
	}
}

// formatTags formats tags for DogStatsD
func (d *DatadogClient) formatTags() string {
	if len(d.tags) == 0 {
		return ""
	}
	return "|#" + strings.Join(d.tags, ",")
}

// Close closes the Datadog connection
func (d *DatadogClient) Close() error {
	if d.conn != nil {
		return d.conn.Close()
	}
	return nil
}

// SendMetrics sends current metrics to Datadog
func (d *DatadogClient) SendMetrics(m *Metrics) {
	if !d.enabled {
		return
	}

	// Send counters
	d.Count("queries.total", int64(m.QueriesTotal.Get()))
	d.Count("queries.success", int64(m.QueriesSuccess.Get()))
	d.Count("queries.failed", int64(m.QueriesFailed.Get()))

	// Send gauges
	d.Gauge("sources.connected", float64(m.SourcesConnected.Get()))
	d.Gauge("sources.healthy", float64(m.SourcesHealthy.Get()))
	d.Gauge("active.connections", float64(m.ActiveConnections.Get()))

	// Send query duration histogram
	d.Histogram("query.duration.p50", m.QueryDuration.Quantile(0.5))
	d.Histogram("query.duration.p90", m.QueryDuration.Quantile(0.9))
	d.Histogram("query.duration.p99", m.QueryDuration.Quantile(0.99))
	d.Histogram("query.duration.avg", m.QueryDuration.Average())

	// Send uptime
	d.Gauge("uptime.seconds", float64(m.Uptime()))
}

// StartPeriodicSend starts sending metrics to Datadog periodically
func (d *DatadogClient) StartPeriodicSend(m *Metrics, interval time.Duration, stopCh <-chan struct{}) {
	if !d.enabled {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.SendMetrics(m)
		case <-stopCh:
			return
		}
	}
}
