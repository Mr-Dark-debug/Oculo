package tui

import (
	"fmt"
	"strings"

	"github.com/Mr-Dark-debug/oculo/pkg/timeutil"
)

// renderDiffView renders the memory mutation diff pane (bottom).
func renderDiffView(m *Model, width, height int) string {
	titleStyle := panelTitleDimStyle
	if m.activePane == PaneMemoryDiff {
		titleStyle = panelTitleStyle
	}

	title := titleStyle.Render("Memory Diff")

	if len(m.memoryDiffs) == 0 {
		return title + "\n" +
			diffContextStyle.Render("No memory mutations for this span.")
	}

	title += traceDimStyle.Render(
		fmt.Sprintf("  %d events", len(m.memoryDiffs)))

	var lines []string

	for _, ev := range m.memoryDiffs {
		ts := treeTimestampStyle.Render(timeutil.FormatTimestamp(ev.Timestamp))

		switch ev.Operation {
		case "ADD":
			val := ""
			if ev.NewValue != nil {
				val = truncate(*ev.NewValue, width-40)
			}
			key := fmt.Sprintf("%s.%s", ev.Namespace, ev.Key)
			lines = append(lines,
				ts+" "+diffAddStyle.Render("+ "+key+": "+val))

		case "DELETE":
			val := ""
			if ev.OldValue != nil {
				val = truncate(*ev.OldValue, width-40)
			}
			key := fmt.Sprintf("%s.%s", ev.Namespace, ev.Key)
			lines = append(lines,
				ts+" "+diffDelStyle.Render("- "+key+": "+val))

		case "UPDATE":
			key := fmt.Sprintf("%s.%s", ev.Namespace, ev.Key)
			lines = append(lines,
				ts+" "+diffModStyle.Render("~ "+key))
			if ev.OldValue != nil {
				lines = append(lines,
					"  "+diffDelStyle.Render("- "+truncate(*ev.OldValue, width-10)))
			}
			if ev.NewValue != nil {
				lines = append(lines,
					"  "+diffAddStyle.Render("+ "+truncate(*ev.NewValue, width-10)))
			}
		}
	}

	// Apply scroll offset
	contentHeight := height - 2
	if m.diffScroll > 0 && m.diffScroll < len(lines) {
		lines = lines[m.diffScroll:]
	}
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
	}

	return title + "\n" + strings.Join(lines, "\n")
}

// renderDiffPanel wraps the diff view in a styled panel.
func renderDiffPanel(m *Model, width, height int) string {
	content := renderDiffView(m, width-4, height-2)

	style := panelStyle
	if m.activePane == PaneMemoryDiff {
		style = panelActiveStyle
	}

	return style.Width(width).Height(height).Render(content)
}
