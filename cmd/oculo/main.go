// Oculo CLI — command-line interface for trace analysis and queries.
//
// Usage:
//
//	oculo <command> [flags]
//
// Commands:
//
//	analyze   Run semantic analysis on a trace
//	query     Query traces and spans
//	status    Show daemon status
//	version   Print version information
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Mr-Dark-debug/oculo/internal/analysis"
	"github.com/Mr-Dark-debug/oculo/internal/database"
	"github.com/Mr-Dark-debug/oculo/internal/ingestion"
)

var (
	Version   = "0.1.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	homeDir, _ := os.UserHomeDir()
	defaultDB := filepath.Join(homeDir, ".oculo", "oculo.db")

	switch os.Args[1] {
	case "analyze":
		cmdAnalyze(defaultDB)
	case "query":
		cmdQuery(defaultDB)
	case "status":
		cmdStatus()
	case "version":
		fmt.Printf("Oculo v%s (commit: %s, built: %s)\n", Version, GitCommit, BuildTime)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Oculo — The Glass Box for AI Agents

Usage:
  oculo <command> [flags]

Commands:
  analyze    Run semantic analysis on a trace
  query      Query traces and spans
  status     Show daemon status and metrics
  version    Print version information

Run 'oculo <command> --help' for details on each command.`)
}

// cmdAnalyze runs the full analysis suite on a trace and outputs a report.
func cmdAnalyze(defaultDB string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	traceID := fs.String("trace", "", "Trace ID to analyze (required)")
	dbPath := fs.String("db", defaultDB, "Path to SQLite database")
	outputFormat := fs.String("format", "markdown", "Output format: markdown, json")
	fs.Parse(os.Args[2:])

	if *traceID == "" {
		fmt.Fprintln(os.Stderr, "Error: --trace is required")
		fs.Usage()
		os.Exit(1)
	}

	store, err := database.NewDBService(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	analyzer := analysis.NewAnalyzer(store)
	report, err := analyzer.FullAnalysis(*traceID)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	switch *outputFormat {
	case "json":
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
	case "markdown":
		fmt.Print(analyzer.FormatReport(report))
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %s\n", *outputFormat)
		os.Exit(1)
	}
}

// cmdQuery lists traces or spans matching a filter.
func cmdQuery(defaultDB string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB, "Path to SQLite database")
	agentName := fs.String("agent", "", "Filter by agent name")
	traceID := fs.String("trace", "", "Show spans for a specific trace")
	search := fs.String("search", "", "Full-text search over prompts/completions")
	limit := fs.Int("limit", 20, "Maximum results")
	fs.Parse(os.Args[2:])

	store, err := database.NewDBService(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	if *search != "" {
		results, err := store.SearchContent(*search, *limit)
		if err != nil {
			log.Fatalf("Search failed: %v", err)
		}
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(b))
		return
	}

	if *traceID != "" {
		spans, err := store.QueryTimeline(*traceID)
		if err != nil {
			log.Fatalf("Query failed: %v", err)
		}
		b, _ := json.MarshalIndent(spans, "", "  ")
		fmt.Println(string(b))
		return
	}

	filter := database.TraceFilter{Limit: *limit}
	if *agentName != "" {
		filter.AgentName = agentName
	}

	traces, err := store.QueryTraces(filter)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	b, _ := json.MarshalIndent(traces, "", "  ")
	fmt.Println(string(b))
}

// cmdStatus shows the current daemon status by querying the metrics endpoint.
func cmdStatus() {
	cfg := ingestion.DefaultConfig()
	url := fmt.Sprintf("http://%s/api/metrics", cfg.MetricsAddr)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("⚠ Oculo daemon is not running.")
		fmt.Printf("  Start it with: oculo-daemon\n")
		fmt.Printf("  (tried: %s)\n", url)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var metrics ingestion.IngestionMetrics
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		log.Fatalf("Failed to decode metrics: %v", err)
	}

	fmt.Println("✅ Oculo daemon is running.")
	fmt.Println()
	fmt.Printf("  Traces ingested:     %d\n", metrics.TracesIngested)
	fmt.Printf("  Spans ingested:      %d\n", metrics.SpansIngested)
	fmt.Printf("  Memory events:       %d\n", metrics.MemoryEvents)
	fmt.Printf("  Batches committed:   %d\n", metrics.BatchesCommitted)
	fmt.Printf("  Errors:              %d\n", metrics.ErrorCount)
	fmt.Printf("  Uptime:              %ds\n", metrics.Uptime)
}
