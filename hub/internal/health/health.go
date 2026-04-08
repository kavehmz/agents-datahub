package health

import (
	"context"
	"sync"
	"time"

	"github.com/regentmarkets/agents-datahub/common/logging"
	"github.com/regentmarkets/agents-datahub/hub/internal/router"
)

// HealthChecker is an interface for sending health checks to sources
type HealthChecker interface {
	SendHealthCheck(sourceName string) error
}

// Monitor monitors source health
type Monitor struct {
	router          *router.Router
	logger          *logging.Logger
	checker         HealthChecker
	interval        time.Duration
	unhealthyThresh int
	recoveryThresh  int
	stopCh          chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
}

// NewMonitor creates a new health monitor
func NewMonitor(
	router *router.Router,
	logger *logging.Logger,
	interval time.Duration,
	unhealthyThreshold int,
	recoveryThreshold int,
) *Monitor {
	return &Monitor{
		router:          router,
		logger:          logger,
		interval:        interval,
		unhealthyThresh: unhealthyThreshold,
		recoveryThresh:  recoveryThreshold,
		stopCh:          make(chan struct{}),
	}
}

// SetChecker sets the health checker (called after GRPCServer is created)
func (m *Monitor) SetChecker(checker HealthChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checker = checker
}

// Start begins health monitoring
func (m *Monitor) Start(ctx context.Context) {
	m.wg.Add(1)
	go m.monitorLoop(ctx)
	m.logger.Info("health_monitor_started", "Health monitoring started", map[string]interface{}{
		"interval":            m.interval.String(),
		"unhealthy_threshold": m.unhealthyThresh,
		"recovery_threshold":  m.recoveryThresh,
	})
}

// Stop stops health monitoring
func (m *Monitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	m.logger.Info("health_monitor_stopped", "Health monitoring stopped", nil)
}

// monitorLoop runs the health check loop
func (m *Monitor) monitorLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkAllSources(ctx)
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// checkAllSources checks health of all sources
func (m *Monitor) checkAllSources(ctx context.Context) {
	sources := m.router.GetAllSources()

	for _, source := range sources {
		go m.checkSource(ctx, source)
	}
}

// checkSource checks health of a single source
func (m *Monitor) checkSource(ctx context.Context, source router.SourceConnection) {
	// Create health check context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Send health check request through the stream
	// This would use the actual proto-generated types when available
	err := m.sendHealthCheck(checkCtx, source)

	if err != nil {
		// Mark as unhealthy if check fails
		m.router.MarkUnhealthy(source.Name)
		m.logger.Warn("health_check_failed", "Source health check failed", map[string]interface{}{
			"source": source.Name,
			"label":  source.Label,
			"error":  err.Error(),
		})
	} else {
		// Mark as healthy if check succeeds
		m.router.MarkHealthy(source.Name)
		m.logger.Debug("health_check_success", "Source health check succeeded", map[string]interface{}{
			"source": source.Name,
			"label":  source.Label,
		})
	}
}

// sendHealthCheck sends a health check request to the source via the gRPC stream
func (m *Monitor) sendHealthCheck(ctx context.Context, source router.SourceConnection) error {
	m.mu.Lock()
	checker := m.checker
	m.mu.Unlock()

	if checker == nil {
		return nil // No checker set yet, skip
	}

	return checker.SendHealthCheck(source.Name)
}

// GetHealthStats returns health statistics
func (m *Monitor) GetHealthStats() map[string]interface{} {
	sources := m.router.GetAllSources()

	totalSources := len(sources)
	healthySources := 0
	unhealthySources := 0

	labelHealth := make(map[string]map[string]int)

	for _, source := range sources {
		if source.Healthy {
			healthySources++
		} else {
			unhealthySources++
		}

		if _, exists := labelHealth[source.Label]; !exists {
			labelHealth[source.Label] = map[string]int{
				"total":     0,
				"healthy":   0,
				"unhealthy": 0,
			}
		}

		labelHealth[source.Label]["total"]++
		if source.Healthy {
			labelHealth[source.Label]["healthy"]++
		} else {
			labelHealth[source.Label]["unhealthy"]++
		}
	}

	return map[string]interface{}{
		"total_sources":     totalSources,
		"healthy_sources":   healthySources,
		"unhealthy_sources": unhealthySources,
		"by_label":          labelHealth,
	}
}
