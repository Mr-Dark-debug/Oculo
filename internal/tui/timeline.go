package tui

import (
	"fmt"
	"strings"

	"github.com/Mr-Dark-debug/oculo/pkg/timeutil"
	"github.com/charmbracelet/lipgloss"
)

// renderTimeline renders the span tree in the left pane.
func renderTimeline(m *Model, width, height int) string {
	titleStyle := panelTitleDimStyle
	if m.activePane == PaneTimeline {
		titleStyle = panelTitleStyle
	}

	title := titleStyle.Render("Timeline")
	if m.stats != nil {
		title += traceDimStyle.Render(
			fmt.Sprintf("  %d spans", m.stats.TotalSpans))
	}

	if len(m.spanTree) == 0 {
		return title + "\n\n" +
			emptyStateStyle.Render("No spans in this trace.")
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	contentHeight := height - 2

	// Scroll so selected span is visible
	scrollStart := 0
	if m.selectedSpan >= contentHeight {
		scrollStart = m.selectedSpan - contentHeight + 1
	}

	end := scrollStart + contentHeight
	if end > len(m.spanTree) {
		end = len(m.spanTree)
	}

	for i := scrollStart; i < end; i++ {
		node := m.spanTree[i]

		// Tree connectors
		indent := strings.Repeat("  ", node.depth)
		connector := treeBranchStyle.Render("\u251c\u2500")
		if i == len(m.spanTree)-1 || (i+1 < len(m.spanTree) && m.spanTree[i+1].depth <= node.depth) {
			connector = treeBranchStyle.Render("\u2514\u2500")
		}

		// Operation tag
		tag := opTag(node.span.OperationType)

		// Name
		name := node.span.OperationName
		if name == "" {
			name = node.span.OperationType
		}
		maxNameLen := width - (node.depth*2 + 20)
		if maxNameLen < 10 {
			maxNameLen = 10
		}
		name = truncate(name, maxNameLen)

		// Duration
		dur := treeDurationStyle.Render(timeutil.FormatDuration(node.span.DurationMs))

		line := fmt.Sprintf("%s%s %s %s %s", indent, connector, tag, name, dur)

		if i == m.selectedSpan {
			line = spanSelectedStyle.Width(width).Render(
				fmt.Sprintf("%s%s %s %s %s", indent, "\u251c\u2500", opTag(node.span.OperationType), name, timeutil.FormatDuration(node.span.DurationMs)))
		} else {
			line = opStyle(node.span.OperationType).Render(line)
		}

		lines = append(lines, line)
	}

	// Scroll indicator
	if len(m.spanTree) > contentHeight {
		pct := 0
		if len(m.spanTree) > 1 {
			pct = m.selectedSpan * 100 / (len(m.spanTree) - 1)
		}
		indicator := traceDimStyle.Render(
			fmt.Sprintf(" %d/%d (%d%%)", m.selectedSpan+1, len(m.spanTree), pct))
		lines = append(lines, indicator)
	}

	return strings.Join(lines, "\n")
}

// renderTimelinePanel wraps the timeline in a styled panel.
func renderTimelinePanel(m *Model, width, height int) string {
	content := renderTimeline(m, width-4, height-2)

	style := panelStyle
	if m.activePane == PaneTimeline {
		style = panelActiveStyle
	}

	return style.Width(width).Height(height).Render(content)
}

// We need this for the lipgloss package import
var _ = lipgloss.Left
