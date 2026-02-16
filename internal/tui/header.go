package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderHeader produces the top bar:
//
//	OCULO  |  Glass Box  |  Trace: a1b2c3  |  Agent: research-bot
func renderHeader(m *Model) string {
	brand := headerBrandStyle.Render("OCULO")
	sep := headerSepStyle.Render(" \u2502 ")

	var parts []string
	parts = append(parts, brand)

	if m.currentTrace != nil {
		parts = append(parts, sep)
		parts = append(parts, headerMetaStyle.Render(
			fmt.Sprintf("Trace %s", shortID(m.currentTrace.TraceID, 10))))
		parts = append(parts, sep)
		parts = append(parts, headerMetaStyle.Render(m.currentTrace.AgentName))

		if m.stats != nil {
			parts = append(parts, sep)
			parts = append(parts, headerMetaStyle.Render(
				fmt.Sprintf("%d spans", m.stats.TotalSpans)))
		}
	} else {
		parts = append(parts, sep)
		parts = append(parts, headerMetaStyle.Render("Trace Explorer"))
	}

	content := strings.Join(parts, "")

	return headerBarStyle.Width(m.width).Render(content)
}

// renderFooter produces the bottom status bar with keyboard hints.
func renderFooter(m *Model) string {
	var left, right string

	if m.searchMode {
		cursor := searchCursorStyle.Render(" ")
		left = searchBarStyle.Render(fmt.Sprintf("/ %s%s", m.searchQuery, cursor))
		right = renderHints([]hint{
			{"enter", "search"},
			{"esc", "cancel"},
		})
	} else if m.showTraceList {
		if m.statusMsg != "" {
			left = statusStyle.Render(m.statusMsg)
		}
		right = renderHints([]hint{
			{"\u2191\u2193", "navigate"},
			{"enter", "select"},
			{"/", "search"},
			{"q", "quit"},
		})
	} else {
		if m.statusMsg != "" {
			left = statusStyle.Render(m.statusMsg)
		}
		right = renderHints([]hint{
			{"\u2191\u2193", "navigate"},
			{"tab", "pane"},
			{"d", "diff"},
			{"/", "search"},
			{"esc", "back"},
			{"q", "quit"},
		})
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	bar := left + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().
		Background(colorBgSurface).
		Width(m.width).
		Render(bar)
}

type hint struct {
	key  string
	desc string
}

func renderHints(hints []hint) string {
	var parts []string
	for _, h := range hints {
		parts = append(parts,
			hintKeyStyle.Render(h.key)+" "+hintDescStyle.Render(h.desc))
	}
	return strings.Join(parts, hintDescStyle.Render("  "))
}
