package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/regentmarkets/agents-datahub/hub/internal/config"
	"github.com/regentmarkets/agents-datahub/hub/internal/server"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config/hub-config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Create server
	srv, err := server.NewServer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	// Start server
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigCh {
		switch sig {
		case syscall.SIGHUP:
			fmt.Println("Received SIGHUP, reloading tokens...")
			if err := srv.ReloadTokens(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to reload tokens: %v\n", err)
			} else {
				fmt.Println("Tokens reloaded successfully")
			}
		case syscall.SIGINT, syscall.SIGTERM:
			fmt.Println("Received shutdown signal, stopping server...")
			if err := srv.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "Error stopping server: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
}
