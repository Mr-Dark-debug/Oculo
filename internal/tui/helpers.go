package tui

import (
	"github.com/Mr-Dark-debug/oculo/internal/database"
	"github.com/charmbracelet/lipgloss"
)

// ────────────────────────────────────────────────────────────
// Span tree construction
// ────────────────────────────────────────────────────────────

// spanNode represents a span in the tree view with its depth level.
type spanNode struct {
	span  *database.Span
	depth int
}

// buildSpanTree constructs a flat depth-ordered list from parent relationships.
func buildSpanTree(spans []*database.Span) []spanNode {
	if len(spans) == 0 {
		return nil
	}

	childrenOf := make(map[string][]*database.Span)
	for _, s := range spans {
		parentID := ""
		if s.ParentSpanID != nil {
			parentID = *s.ParentSpanID
		}
		childrenOf[parentID] = append(childrenOf[parentID], s)
	}

	var result []spanNode
	var walk func(parentID string, depth int)
	walk = func(parentID string, depth int) {
		for _, child := range childrenOf[parentID] {
			result = append(result, spanNode{span: child, depth: depth})
			walk(child.SpanID, depth+1)
		}
	}

	walk("", 0)

	// Fallback: if no root spans found, use flat list
	if len(result) == 0 {
		for _, s := range spans {
			result = append(result, spanNode{span: s, depth: 0})
		}
	}

	return result
}

// ────────────────────────────────────────────────────────────
// Operation type rendering
// ────────────────────────────────────────────────────────────

// opTag returns a short colored label for an operation type.
func opTag(opType string) string {
	switch opType {
	case "LLM":
		return spanLLMStyle.Render("llm")
	case "TOOL":
		return spanToolStyle.Render("tool")
	case "MEMORY":
		return spanMemoryStyle.Render("mem")
	case "PLANNING":
		return spanPlanningStyle.Render("plan")
	case "RETRIEVAL":
		return spanRetrievalStyle.Render("retrieval")
	default:
		return spanNormalStyle.Render("op")
	}
}

// opStyle returns the style for an operation type.
func opStyle(opType string) lipgloss.Style {
	switch opType {
	case "LLM":
		return spanLLMStyle
	case "TOOL":
		return spanToolStyle
	case "MEMORY":
		return spanMemoryStyle
	case "PLANNING":
		return spanPlanningStyle
	case "RETRIEVAL":
		return spanRetrievalStyle
	default:
		return spanNormalStyle
	}
}

// ────────────────────────────────────────────────────────────
// String helpers
// ────────────────────────────────────────────────────────────

// truncate cuts a string to maxLen and appends "..." if truncated.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// shortID returns first n characters of an ID string.
func shortID(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}

// clamp restricts val to [lo, hi].
func clamp(val, lo, hi int) int {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

// max returns the larger of a and b.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min returns the smaller of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
