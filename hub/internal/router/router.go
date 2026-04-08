package router

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNoSourcesAvailable = errors.New("no sources available for label")
	ErrOperationNotFound  = errors.New("operation not found on source")
)

// SourceConnection represents a connected source
type SourceConnection struct {
	Name         string
	Label        string
	Operations   map[string]bool // operation name -> supported
	Stream       interface{}     // gRPC stream (type depends on proto generation)
	Healthy      bool
	ConnectedAt  time.Time
	LastHealthy  time.Time
	FailureCount int
	SuccessCount int
	QueryCount   int64
}

// Router manages source connections and routes queries
type Router struct {
	mu      sync.RWMutex
	sources map[string]*SourceConnection // source name -> connection
	labels  map[string][]string          // label -> source names
	robin   map[string]int               // label -> current index for round-robin
}

// NewRouter creates a new router
func NewRouter() *Router {
	return &Router{
		sources: make(map[string]*SourceConnection),
		labels:  make(map[string][]string),
		robin:   make(map[string]int),
	}
}

// RegisterSource registers a new source connection
func (r *Router) RegisterSource(name, label string, operations []string, stream interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if source already exists
	if _, exists := r.sources[name]; exists {
		return fmt.Errorf("source %s already registered", name)
	}

	// Create operation map
	ops := make(map[string]bool)
	for _, op := range operations {
		ops[op] = true
	}

	// Create source connection
	source := &SourceConnection{
		Name:        name,
		Label:       label,
		Operations:  ops,
		Stream:      stream,
		Healthy:     true,
		ConnectedAt: time.Now(),
		LastHealthy: time.Now(),
	}

	r.sources[name] = source

	// Add to label list
	if _, exists := r.labels[label]; !exists {
		r.labels[label] = []string{}
		r.robin[label] = 0
	}
	r.labels[label] = append(r.labels[label], name)

	return nil
}

// UnregisterSource removes a source connection
func (r *Router) UnregisterSource(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	source, exists := r.sources[name]
	if !exists {
		return
	}

	// Remove from sources
	delete(r.sources, name)

	// Remove from label list
	label := source.Label
	sources := r.labels[label]
	for i, s := range sources {
		if s == name {
			r.labels[label] = append(sources[:i], sources[i+1:]...)
			break
		}
	}

	// Clean up empty labels
	if len(r.labels[label]) == 0 {
		delete(r.labels, label)
		delete(r.robin, label)
	}
}

// SelectSource selects a source using round-robin for the given label and operation
func (r *Router) SelectSource(label, operation string) (*SourceConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sources, exists := r.labels[label]
	if !exists || len(sources) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoSourcesAvailable, label)
	}

	// Find healthy sources that support the operation
	healthySources := []string{}
	for _, name := range sources {
		source := r.sources[name]
		if source.Healthy && source.Operations[operation] {
			healthySources = append(healthySources, name)
		}
	}

	if len(healthySources) == 0 {
		return nil, fmt.Errorf("%w: %s/%s", ErrNoSourcesAvailable, label, operation)
	}

	// Round-robin selection — monotonically increment to avoid skipping
	// when the healthy set size changes between calls
	index := r.robin[label] % len(healthySources)
	r.robin[label]++

	selectedName := healthySources[index]
	source := r.sources[selectedName]
	source.QueryCount++

	return source, nil
}

// GetSource returns a copy of a source by name (safe to read without holding lock)
func (r *Router) GetSource(name string) (SourceConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	source, exists := r.sources[name]
	if !exists {
		return SourceConnection{}, false
	}
	return *source, true
}

// GetSourcesByLabel returns copies of all sources for a label
func (r *Router) GetSourcesByLabel(label string) []SourceConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sources := r.labels[label]
	result := make([]SourceConnection, 0, len(sources))
	for _, name := range sources {
		if source, exists := r.sources[name]; exists {
			result = append(result, *source)
		}
	}
	return result
}

// GetAllSources returns copies of all registered sources
func (r *Router) GetAllSources() []SourceConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]SourceConnection, 0, len(r.sources))
	for _, source := range r.sources {
		result = append(result, *source)
	}
	return result
}

// MarkHealthy marks a source as healthy
func (r *Router) MarkHealthy(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if source, exists := r.sources[name]; exists {
		source.Healthy = true
		source.LastHealthy = time.Now()
		source.SuccessCount++
		source.FailureCount = 0
	}
}

// MarkUnhealthy marks a source as unhealthy
func (r *Router) MarkUnhealthy(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if source, exists := r.sources[name]; exists {
		source.FailureCount++
		if source.FailureCount >= 3 { // Configurable threshold
			source.Healthy = false
		}
	}
}

// GetStats returns statistics about sources
func (r *Router) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalSources := len(r.sources)
	healthySources := 0
	for _, source := range r.sources {
		if source.Healthy {
			healthySources++
		}
	}

	labelStats := make(map[string]interface{})
	for label, sources := range r.labels {
		healthy := 0
		for _, name := range sources {
			if source, exists := r.sources[name]; exists && source.Healthy {
				healthy++
			}
		}
		labelStats[label] = map[string]interface{}{
			"total":   len(sources),
			"healthy": healthy,
		}
	}

	return map[string]interface{}{
		"total_sources":   totalSources,
		"healthy_sources": healthySources,
		"labels":          labelStats,
	}
}

// QueryRequest represents a query to be executed
type QueryRequest struct {
	QueryID    string
	Label      string
	Operation  string
	Parameters map[string]interface{}
	Metadata   map[string]string
	Context    context.Context
}

// QueryResponse represents a query response
type QueryResponse struct {
	QueryID         string
	Success         bool
	Data            map[string]interface{}
	Error           string
	SourceName      string
	ExecutionTimeMs int64
}
