// Oculo TUI — the "Glass Box" interactive debugger for AI agent traces.
//
// Usage:
//
//	oculo-tui [flags]
//
// Flags:
//
//	--db    Path to SQLite database file (default: ~/.oculo/oculo.db)
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Mr-Dark-debug/oculo/internal/database"
	"github.com/Mr-Dark-debug/oculo/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	homeDir, _ := os.UserHomeDir()
	defaultDB := filepath.Join(homeDir, ".oculo", "oculo.db")

	dbPath := flag.String("db", defaultDB, "Path to SQLite database file")
	flag.Parse()

	// Open the database in read-only mode for the TUI
	store, err := database.NewDBService(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database at %s: %v\n"+
			"Is the Oculo daemon running? Start it with: oculo-daemon", *dbPath, err)
	}
	defer store.Close()

	model := tui.NewModel(store)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
