package tui

import (
	"fmt"
	"strings"

	"github.com/Mr-Dark-debug/oculo/pkg/timeutil"
	"github.com/charmbracelet/lipgloss"
)

// renderDetail renders the span detail pane (right side).
func renderDetail(m *Model, width, height int) string {
	titleStyle := panelTitleDimStyle
	if m.activePane == PaneDetail {
		titleStyle = panelTitleStyle
	}
	title := titleStyle.Render("Detail")

	if len(m.spanTree) == 0 || m.selectedSpan >= len(m.spanTree) {
		return title + "\n\n" +
			emptyStateStyle.Render("Select a span to view details.")
	}

	span := m.spanTree[m.selectedSpan].span
	var lines []string

	lines = append(lines, title)
	lines = append(lines, "")

	// ── Metadata ──

	lines = append(lines, detailRow("Type", span.OperationType))
	lines = append(lines, detailRow("Name", span.OperationName))
	lines = append(lines, detailRow("ID", shortID(span.SpanID, 16)))
	lines = append(lines, detailRow("Duration", timeutil.FormatDuration(span.DurationMs)))
	lines = append(lines, detailRow("Status", span.Status))

	if span.Model != nil {
		lines = append(lines, detailRow("Model", *span.Model))
	}

	// ── Token usage ──

	if span.PromptTokens > 0 || span.CompletionTokens > 0 {
		lines = append(lines, "")
		lines = append(lines, detailSectionStyle.Render("Token Usage"))

		total := span.PromptTokens + span.CompletionTokens
		lines = append(lines, detailRow("Prompt", fmt.Sprintf("%d", span.PromptTokens)))
		lines = append(lines, detailRow("Completion", fmt.Sprintf("%d", span.CompletionTokens)))
		lines = append(lines, detailRow("Total", fmt.Sprintf("%d", total)))

		// Horizontal bar
		barWidth := width - 6
		if barWidth > 50 {
			barWidth = 50
		}
		if barWidth > 4 && total > 0 {
			promptW := barWidth * span.PromptTokens / total
			compW := barWidth - promptW

			bar := tokenBarPromptStyle.Render(strings.Repeat("\u2588", promptW)) +
				tokenBarCompletionStyle.Render(strings.Repeat("\u2588", compW))

			promptPct := span.PromptTokens * 100 / total
			legend := traceDimStyle.Render(
				fmt.Sprintf("prompt %d%%  completion %d%%", promptPct, 100-promptPct))

			lines = append(lines, bar)
			lines = append(lines, legend)
		}
	}

	// ── Trace-level summary ──

	if m.stats != nil {
		lines = append(lines, "")
		lines = append(lines, detailSectionStyle.Render("Trace Summary"))

		totalTokens := m.stats.TotalPromptTokens + m.stats.TotalCompletionTokens
		lines = append(lines, detailRow("LLM Calls", fmt.Sprintf("%d", m.stats.LLMCalls)))
		lines = append(lines, detailRow("Tool Calls", fmt.Sprintf("%d", m.stats.ToolCalls)))
		lines = append(lines, detailRow("Memory Ops", fmt.Sprintf("%d", m.stats.MemoryEventCount)))
		lines = append(lines, detailRow("Total Tokens", fmt.Sprintf("%d", totalTokens)))
		lines = append(lines, detailRow("Duration",
			timeutil.FormatDuration(m.stats.TotalDurationMs)))

		// Token distribution bars
		if totalTokens > 0 {
			barWidth := width - 6
			if barWidth > 50 {
				barWidth = 50
			}

			lines = append(lines, "")
			lines = append(lines, renderUsageBar("LLM", m.stats.LLMCalls, m.stats.TotalSpans, barWidth, colorPurple))
			lines = append(lines, renderUsageBar("Tool", m.stats.ToolCalls, m.stats.TotalSpans, barWidth, colorGreen))
			lines = append(lines, renderUsageBar("Memory", m.stats.MemoryEventCount,
				m.stats.LLMCalls+m.stats.ToolCalls+m.stats.MemoryEventCount, barWidth, colorYellow))
		}
	}

	// ── Prompt preview ──

	if span.Prompt != nil && *span.Prompt != "" {
		lines = append(lines, "")
		lines = append(lines, detailSectionStyle.Render("Prompt"))
		preview := truncate(*span.Prompt, (width-4)*3)
		for _, line := range strings.Split(preview, "\n") {
			lines = append(lines, traceDimStyle.Render(line))
		}
	}

	// ── Completion preview ──

	if span.Completion != nil && *span.Completion != "" {
		lines = append(lines, "")
		lines = append(lines, detailSectionStyle.Render("Completion"))
		preview := truncate(*span.Completion, (width-4)*3)
		for _, line := range strings.Split(preview, "\n") {
			lines = append(lines, detailValueStyle.Render(line))
		}
	}

	// Truncate to available height
	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

// renderDetailPanel wraps detail in a styled panel.
func renderDetailPanel(m *Model, width, height int) string {
	content := renderDetail(m, width-4, height-2)

	style := panelStyle
	if m.activePane == PaneDetail {
		style = panelActiveStyle
	}

	return style.Width(width).Height(height).Render(content)
}

// ── helpers ──

func detailRow(label, value string) string {
	return detailLabelStyle.Render(label) + "  " + detailValueStyle.Render(value)
}

func renderUsageBar(label string, count, total, barWidth int, color lipgloss.Color) string {
	if total == 0 {
		return ""
	}
	pct := count * 100 / total
	filled := barWidth * count / total
	if filled < 1 && count > 0 {
		filled = 1
	}
	empty := barWidth - filled

	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("\u2588", filled)) +
		tokenBarEmptyStyle.Render(strings.Repeat("\u2591", empty))

	return fmt.Sprintf("%-8s %s %d%%", label, bar, pct)
}
