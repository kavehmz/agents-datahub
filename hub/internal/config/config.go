package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds hub configuration
type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Sources  SourcesConfig   `yaml:"sources"`
	Exposers []ExposerConfig `yaml:"exposers"`
	Logging  LoggingConfig   `yaml:"logging"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	GRPCPort    int       `yaml:"grpc_port"`
	HTTPPort    int       `yaml:"http_port"`
	MetricsPort int       `yaml:"metrics_port"`
	TLS         TLSConfig `yaml:"tls"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// SourcesConfig holds source connection configuration
type SourcesConfig struct {
	ConnectionTimeout   time.Duration `yaml:"connection_timeout"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	UnhealthyThreshold  int           `yaml:"unhealthy_threshold"`
	RecoveryThreshold   int           `yaml:"recovery_threshold"`
}

// ExposerConfig holds exposer configuration
type ExposerConfig struct {
	Name        string             `yaml:"name"`
	Permissions []PermissionConfig `yaml:"permissions"`
}

// PermissionConfig holds permission configuration
type PermissionConfig struct {
	Label      string   `yaml:"label"`
	Operations []string `yaml:"operations"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Load loads configuration from file and environment
func Load(configPath string) (*Config, error) {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Override with environment variables if present
	if port := os.Getenv("GRPC_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.GRPCPort = p
		}
	}

	if port := os.Getenv("HTTP_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.HTTPPort = p
		}
	}

	if port := os.Getenv("METRICS_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.MetricsPort = p
		}
	}

	// Set defaults if not specified
	if config.Server.GRPCPort == 0 {
		config.Server.GRPCPort = 50051
	}
	if config.Server.HTTPPort == 0 {
		config.Server.HTTPPort = 8080
	}
	if config.Server.MetricsPort == 0 {
		config.Server.MetricsPort = 9090
	}

	if config.Sources.ConnectionTimeout == 0 {
		config.Sources.ConnectionTimeout = 30 * time.Second
	}
	if config.Sources.HealthCheckInterval == 0 {
		config.Sources.HealthCheckInterval = 30 * time.Second
	}
	if config.Sources.UnhealthyThreshold == 0 {
		config.Sources.UnhealthyThreshold = 3
	}
	if config.Sources.RecoveryThreshold == 0 {
		config.Sources.RecoveryThreshold = 2
	}

	if config.Logging.Level == "" {
		config.Logging.Level = "INFO"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "json"
	}

	return &config, nil
}

// GetDatadogEndpoint returns the Datadog endpoint from environment
func GetDatadogEndpoint() string {
	return os.Getenv("DATADOG_ENDPOINT")
}
