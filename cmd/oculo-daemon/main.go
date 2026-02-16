// Oculo Daemon — the high-throughput ingestion service for AI agent traces.
//
// Usage:
//
//	oculo-daemon [flags]
//
// Flags:
//
//	--listen    TCP/UDS address to listen on (default: 127.0.0.1:9876 on Windows)
//	--db        Path to SQLite database file (default: ~/.oculo/oculo.db)
//	--metrics   HTTP address for Prometheus metrics (default: 127.0.0.1:9877)
//	--batch     Batch size for flush (default: 1000)
//	--flush     Flush interval (default: 500ms)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Mr-Dark-debug/oculo/internal/database"
	"github.com/Mr-Dark-debug/oculo/internal/ingestion"
)

func main() {
	cfg := ingestion.DefaultConfig()

	flag.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "TCP/UDS listen address")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "Path to SQLite database file")
	flag.StringVar(&cfg.MetricsAddr, "metrics", cfg.MetricsAddr, "Prometheus metrics HTTP address")
	flag.IntVar(&cfg.BatchSize, "batch", cfg.BatchSize, "Batch size before flush")
	flag.Parse()

	// Ensure the database directory exists
	dbDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("Failed to create database directory %s: %v", dbDir, err)
	}

	// Initialize storage
	store, err := database.NewDBService(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.Close()

	// Create and start the daemon
	daemon := ingestion.NewDaemonIngester(cfg, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := daemon.Start(ctx); err != nil {
		log.Fatalf("Failed to start daemon: %v", err)
	}

	// Print startup banner
	fmt.Println()
	fmt.Println("  OCULO DAEMON")
	fmt.Println("  The Glass Box for AI Agents")
	fmt.Println()
	fmt.Printf("  Listen:  %s\n", cfg.ListenAddr)
	fmt.Printf("  DB:      %s\n", cfg.DBPath)
	fmt.Printf("  Metrics: http://%s/metrics\n", cfg.MetricsAddr)
	fmt.Println()
	fmt.Println("  Press Ctrl+C to stop.")
	fmt.Println()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n  Shutting down gracefully...")
	cancel()
	if err := daemon.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	fmt.Println("  Done.")
}
