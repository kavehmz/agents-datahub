package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds source configuration
type Config struct {
	Source     SourceConfig      `yaml:"source"`
	Hub        HubConfig         `yaml:"hub"`
	Database   DatabaseConfig    `yaml:"database"`
	RestAPI    RestAPIConfig     `yaml:"restapi"`
	Operations []OperationConfig `yaml:"operations"`
}

// SourceConfig holds source identity configuration
type SourceConfig struct {
	Name    string `yaml:"name"`
	Label   string `yaml:"label"`
	Version string `yaml:"version"`
}

// HubConfig holds hub connection configuration
type HubConfig struct {
	Endpoint             string        `yaml:"endpoint"`
	ReconnectInterval    time.Duration `yaml:"reconnect_interval"`
	MaxReconnectAttempts int           `yaml:"max_reconnect_attempts"`
	TLS                  TLSConfig     `yaml:"tls"`
}

// TLSConfig holds TLS configuration for connecting to the hub
type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	CAFile  string `yaml:"ca_file"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Type           string        `yaml:"type"`
	MaxConnections int           `yaml:"max_connections"`
	QueryTimeout   time.Duration `yaml:"query_timeout"`
}

// RestAPIConfig holds REST API configuration
type RestAPIConfig struct {
	Enabled bool          `yaml:"enabled"`
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
}

// OperationConfig holds operation configuration
type OperationConfig struct {
	Name     string        `yaml:"name"`
	Timeout  time.Duration `yaml:"timeout"`
	CacheTTL time.Duration `yaml:"cache_ttl"`
	MaxLimit int           `yaml:"max_limit"`
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

	// Override with environment variables
	if name := os.Getenv("SOURCE_NAME"); name != "" {
		config.Source.Name = name
	}

	if endpoint := os.Getenv("HUB_ENDPOINT"); endpoint != "" {
		config.Hub.Endpoint = endpoint
	}

	// Database URL is loaded from environment for security (optional —
	// sources that only use REST APIs don't need a database).

	// Set defaults
	if config.Source.Version == "" {
		config.Source.Version = "1.0.0"
	}

	if config.Hub.ReconnectInterval == 0 {
		config.Hub.ReconnectInterval = 5 * time.Second
	}

	if config.Hub.MaxReconnectAttempts == 0 {
		config.Hub.MaxReconnectAttempts = 10
	}

	if config.Database.MaxConnections == 0 {
		config.Database.MaxConnections = 20
	}

	if config.Database.QueryTimeout == 0 {
		config.Database.QueryTimeout = 10 * time.Second
	}

	// Set REST API defaults
	if config.RestAPI.Timeout == 0 {
		config.RestAPI.Timeout = 10 * time.Second
	}

	return &config, nil
}

// GetDatabaseURL returns the database URL from environment
func GetDatabaseURL() string {
	return os.Getenv("DATABASE_URL")
}

// GetRestAPIToken returns the REST API auth token from environment
func GetRestAPIToken() string {
	return os.Getenv("RESTAPI_AUTH_TOKEN")
}

// GetSourceToken returns the source authentication token from environment
func GetSourceToken(sourceName string) string {
	// Source authenticates to hub with TOKEN_SOURCE_<name>
	// But the source itself doesn't need to know this, it just needs to get the token
	return os.Getenv("SOURCE_AUTH_TOKEN")
}

// GetDatadogEndpoint returns the Datadog endpoint from environment
func GetDatadogEndpoint() string {
	return os.Getenv("DATADOG_ENDPOINT")
}
