package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/regentmarkets/agents-datahub/source/internal/client"
	"github.com/regentmarkets/agents-datahub/source/internal/config"
	"github.com/regentmarkets/agents-datahub/source/internal/operations"
	"github.com/regentmarkets/agents-datahub/source/internal/postgres"
	"github.com/regentmarkets/agents-datahub/source/internal/restapi"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config/source-config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Create database client (optional - only if DATABASE_URL is set)
	var dbClient *postgres.Client
	databaseURL := config.GetDatabaseURL()
	if databaseURL != "" {
		dbClient, err = postgres.NewClient(databaseURL, cfg.Database.MaxConnections)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create database client: %v\n", err)
			os.Exit(1)
		}
		defer dbClient.Close()
		fmt.Println("PostgreSQL client initialized")
	}

	// Create REST API client (optional - only if enabled in config)
	var restClient *restapi.Client
	if cfg.RestAPI.Enabled {
		authToken := config.GetRestAPIToken()
		restClient = restapi.NewClient(cfg.RestAPI.BaseURL, authToken, cfg.RestAPI.Timeout)
		fmt.Printf("REST API client initialized for %s\n", cfg.RestAPI.BaseURL)
	}

	// Create unified operations handler
	opsHandler := operations.NewHandler(dbClient, restClient)

	// Create source client
	sourceClient, err := client.NewClient(cfg, opsHandler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create source client: %v\n", err)
		os.Exit(1)
	}

	// Start client
	if err := sourceClient.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start client: %v\n", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	fmt.Println("Received shutdown signal, stopping client...")

	if err := sourceClient.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping client: %v\n", err)
		os.Exit(1)
	}
}
