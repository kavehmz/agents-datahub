package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds exposer configuration
type Config struct {
	Exposer ExposerConfig `yaml:"exposer"`
	Hub     HubConfig     `yaml:"hub"`
	API     APIConfig     `yaml:"api"`
}

// ExposerConfig holds exposer identity configuration
type ExposerConfig struct {
	Name string `yaml:"name"`
}

// HubConfig holds hub connection configuration
type HubConfig struct {
	Endpoint string        `yaml:"endpoint"`
	Timeout  time.Duration `yaml:"timeout"`
	TLS      bool          `yaml:"tls"`
}

// APIConfig holds API server configuration
type APIConfig struct {
	Port      int        `yaml:"port"`
	RateLimit RateLimit  `yaml:"rate_limit"`
	CORS      CORSConfig `yaml:"cors"`
}

// RateLimit holds rate limiting configuration
type RateLimit struct {
	Enabled bool `yaml:"enabled"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Enabled bool     `yaml:"enabled"`
	Origins []string `yaml:"origins"`
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
	if name := os.Getenv("EXPOSER_NAME"); name != "" {
		config.Exposer.Name = name
	}

	if endpoint := os.Getenv("HUB_ENDPOINT"); endpoint != "" {
		config.Hub.Endpoint = endpoint
	}

	if port := os.Getenv("API_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.API.Port = p
		}
	}

	// Set defaults
	if config.Hub.Timeout == 0 {
		config.Hub.Timeout = 30 * time.Second
	}

	if config.API.Port == 0 {
		config.API.Port = 3000
	}

	return &config, nil
}

// GetExposerToken returns the exposer authentication token from environment
func GetExposerToken(exposerName string) string {
	// Exposer authenticates to hub with its token
	return os.Getenv("EXPOSER_AUTH_TOKEN")
}

// GetDatadogEndpoint returns the Datadog endpoint from environment
func GetDatadogEndpoint() string {
	return os.Getenv("DATADOG_ENDPOINT")
}
