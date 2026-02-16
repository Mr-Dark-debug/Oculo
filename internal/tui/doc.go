// Package tui implements the Oculo terminal user interface.
//
// This is a professional AI debugging instrument built with
// Charmbracelet's BubbleTea, Lipgloss, and Bubbles libraries.
//
// Component architecture:
//
//	model.go     — root model, message routing, Init/Update
//	theme.go     — centralized color + style definitions
//	header.go    — top bar with trace context
//	timeline.go  — span tree with depth-aware rendering
//	detail.go    — span metadata + token usage bars
//	diffview.go  — unified memory mutation diff viewer
//	footer.go    — status line + keyboard hints
//	tracelist.go — trace selector (initial screen)
//	helpers.go   — span tree building, truncation, etc.
package tui
