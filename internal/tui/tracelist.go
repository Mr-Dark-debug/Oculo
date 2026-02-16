package tui

import (
	"fmt"
	"strings"

	"github.com/Mr-Dark-debug/oculo/pkg/timeutil"
	"github.com/charmbracelet/lipgloss"
)

// renderTraceList renders the trace selection screen.
func renderTraceList(m *Model) string {
	if len(m.traces) == 0 {
		empty := emptyStateStyle.Render(
			"No traces found.\n\n" +
				"Start an agent instrumented with the Oculo SDK,\n" +
				"then traces will appear here automatically.")
		return lipgloss.Place(
			m.width,
			m.height-3, // minus header + footer
			lipgloss.Center,
			lipgloss.Center,
			empty,
		)
	}

	title := panelTitleStyle.Render("Traces")
	count := traceDimStyle.Render(fmt.Sprintf("  %d total", len(m.traces)))
	heading := title + count

	var lines []string
	lines = append(lines, heading)
	lines = append(lines, "")

	// Visible range for scrolling
	maxVisible := m.height - 6
	if maxVisible < 5 {
		maxVisible = 5
	}

	startIdx := 0
	if m.selectedTrace >= maxVisible {
		startIdx = m.selectedTrace - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(m.traces) {
		endIdx = len(m.traces)
	}

	for i := startIdx; i < endIdx; i++ {
		t := m.traces[i]

		// Status indicator
		var statusDot string
		switch t.Status {
		case "completed":
			statusDot = traceStatusOk.Render("\u25cf")
		case "failed":
			statusDot = traceStatusFail.Render("\u25cf")
		case "running":
			statusDot = traceStatusRunning.Render("\u25cb")
		default:
			statusDot = traceDimStyle.Render("\u25cb")
		}

		id := traceDimStyle.Render(shortID(t.TraceID, 10))
		ts := traceDimStyle.Render(timeutil.FormatTimestampFull(t.StartTime))

		content := fmt.Sprintf("%s  %s  %s  %s", statusDot, t.AgentName, id, ts)

		if i == m.selectedTrace {
			line := traceSelectedStyle.Width(m.width - 4).Render(content)
			lines = append(lines, line)
		} else {
			line := traceItemStyle.Width(m.width - 4).Render(content)
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}
