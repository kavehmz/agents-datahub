package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents log severity
type Level string

const (
	DEBUG Level = "DEBUG"
	INFO  Level = "INFO"
	WARN  Level = "WARN"
	ERROR Level = "ERROR"
)

// Logger provides structured JSON logging
type Logger struct {
	serviceName string
	level       Level
	output      io.Writer
	mu          sync.Mutex
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Service   string                 `json:"service"`
	Event     string                 `json:"event"`
	Message   string                 `json:"message,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// NewLogger creates a new logger instance
func NewLogger(serviceName string, level Level) *Logger {
	return &Logger{
		serviceName: serviceName,
		level:       level,
		output:      os.Stdout,
	}
}

// SetOutput sets the output writer (useful for testing)
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	l.output = w
	l.mu.Unlock()
}

// log writes a log entry
func (l *Logger) log(level Level, event, message string, data map[string]interface{}, err error) {
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     string(level),
		Service:   l.serviceName,
		Event:     event,
		Message:   message,
		Data:      data,
	}

	if err != nil {
		entry.Error = err.Error()
	}

	jsonData, _ := json.Marshal(entry)
	l.mu.Lock()
	fmt.Fprintln(l.output, string(jsonData))
	l.mu.Unlock()
}

// Debug logs a debug message
func (l *Logger) Debug(event, message string, data map[string]interface{}) {
	if l.shouldLog(DEBUG) {
		l.log(DEBUG, event, message, data, nil)
	}
}

// Info logs an info message
func (l *Logger) Info(event, message string, data map[string]interface{}) {
	if l.shouldLog(INFO) {
		l.log(INFO, event, message, data, nil)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(event, message string, data map[string]interface{}) {
	if l.shouldLog(WARN) {
		l.log(WARN, event, message, data, nil)
	}
}

// Error logs an error message
func (l *Logger) Error(event, message string, err error, data map[string]interface{}) {
	if l.shouldLog(ERROR) {
		l.log(ERROR, event, message, data, err)
	}
}

// shouldLog checks if the message should be logged based on level
func (l *Logger) shouldLog(level Level) bool {
	levels := map[Level]int{
		DEBUG: 0,
		INFO:  1,
		WARN:  2,
		ERROR: 3,
	}

	return levels[level] >= levels[l.level]
}

// QueryLogger provides specific query logging
type QueryLogger struct {
	logger *Logger
}

// NewQueryLogger creates a query logger
func NewQueryLogger(logger *Logger) *QueryLogger {
	return &QueryLogger{logger: logger}
}

// LogQuery logs a query execution
func (ql *QueryLogger) LogQuery(queryID, exposer, client, label, operation, source, status string,
	hubMs, sourceMs, totalMs int64, err error) {

	data := map[string]interface{}{
		"query": map[string]interface{}{
			"id":        queryID,
			"exposer":   exposer,
			"label":     label,
			"operation": operation,
			"source":    source,
			"status":    status,
			"duration": map[string]interface{}{
				"hub_ms":    hubMs,
				"source_ms": sourceMs,
				"total_ms":  totalMs,
			},
		},
	}

	if client != "" {
		data["query"].(map[string]interface{})["client"] = client
	}

	if err != nil {
		data["query"].(map[string]interface{})["error"] = err.Error()
		ql.logger.Error("query_executed", "Query execution failed", err, data)
	} else {
		ql.logger.Info("query_executed", "Query executed successfully", data)
	}
}

// ConnectionLogger provides specific connection logging
type ConnectionLogger struct {
	logger *Logger
}

// NewConnectionLogger creates a connection logger
func NewConnectionLogger(logger *Logger) *ConnectionLogger {
	return &ConnectionLogger{logger: logger}
}

// LogSourceConnected logs source connection
func (cl *ConnectionLogger) LogSourceConnected(name, label string, operations []string, remoteAddr string) {
	data := map[string]interface{}{
		"connection": map[string]interface{}{
			"type":        "source",
			"name":        name,
			"label":       label,
			"operations":  operations,
			"remote_addr": remoteAddr,
		},
	}
	cl.logger.Info("source_connected", "Source connected successfully", data)
}

// LogSourceDisconnected logs source disconnection
func (cl *ConnectionLogger) LogSourceDisconnected(name, label, reason string) {
	data := map[string]interface{}{
		"connection": map[string]interface{}{
			"type":   "source",
			"name":   name,
			"label":  label,
			"reason": reason,
		},
	}
	cl.logger.Info("source_disconnected", "Source disconnected", data)
}

// LogExposerAuthenticated logs exposer authentication
func (cl *ConnectionLogger) LogExposerAuthenticated(name, remoteAddr string, success bool) {
	event := "exposer_authenticated"
	data := map[string]interface{}{
		"connection": map[string]interface{}{
			"type":        "exposer",
			"name":        name,
			"remote_addr": remoteAddr,
			"success":     success,
		},
	}

	if success {
		cl.logger.Info(event, "Exposer authenticated successfully", data)
	} else {
		cl.logger.Warn(event, "Exposer authentication failed", data)
	}
}
