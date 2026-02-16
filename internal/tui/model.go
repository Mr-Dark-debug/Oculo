package tui

import (
	"fmt"

	"github.com/Mr-Dark-debug/oculo/internal/database"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ────────────────────────────────────────────────────────────
// Pane focuses
// ────────────────────────────────────────────────────────────

// Pane represents which UI pane currently has keyboard focus.
type Pane int

const (
	PaneTimeline Pane = iota
	PaneDetail
	PaneMemoryDiff
)

// ────────────────────────────────────────────────────────────
// Model
// ────────────────────────────────────────────────────────────

// Model is the root BubbleTea model for the Oculo TUI.
// State is organized by concern; rendering is delegated
// to component functions in separate files.
type Model struct {
	store database.Store

	// Data
	traces       []*database.Trace
	currentTrace *database.Trace
	spans        []*database.Span
	spanTree     []spanNode
	memoryDiffs  []*database.MemoryEvent
	stats        *database.TraceStats

	// UI state
	activePane    Pane
	selectedSpan  int
	selectedTrace int
	scrollOffset  int
	diffScroll    int
	width         int
	height        int
	showTraceList bool
	searchMode    bool
	searchQuery   string

	// Status
	statusMsg string
	err       error
}

// NewModel creates a new TUI model backed by the given store.
func NewModel(store database.Store) Model {
	return Model{
		store:         store,
		showTraceList: true,
		statusMsg:     "Loading traces...",
	}
}

// ────────────────────────────────────────────────────────────
// Messages
// ────────────────────────────────────────────────────────────

type tracesLoadedMsg []*database.Trace
type timelineLoadedMsg struct {
	spans []*database.Span
	stats *database.TraceStats
}
type memoryDiffsLoadedMsg []*database.MemoryEvent
type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// ────────────────────────────────────────────────────────────
// Init
// ────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return m.loadTraces()
}

func (m Model) loadTraces() tea.Cmd {
	return func() tea.Msg {
		traces, err := m.store.QueryTraces(database.TraceFilter{Limit: 100})
		if err != nil {
			return errMsg{err}
		}
		return tracesLoadedMsg(traces)
	}
}

func (m Model) loadTimeline(traceID string) tea.Cmd {
	return func() tea.Msg {
		spans, err := m.store.QueryTimeline(traceID)
		if err != nil {
			return errMsg{err}
		}
		stats, err := m.store.GetTraceStats(traceID)
		if err != nil {
			return errMsg{err}
		}
		return timelineLoadedMsg{spans: spans, stats: stats}
	}
}

func (m Model) loadMemoryDiffs(spanID string) tea.Cmd {
	return func() tea.Msg {
		diffs, err := m.store.GetMemoryDiffs(spanID)
		if err != nil {
			return errMsg{err}
		}
		return memoryDiffsLoadedMsg(diffs)
	}
}

// ────────────────────────────────────────────────────────────
// Update
// ────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tracesLoadedMsg:
		m.traces = []*database.Trace(msg)
		if len(m.traces) > 0 {
			m.statusMsg = fmt.Sprintf("%d traces", len(m.traces))
		} else {
			m.statusMsg = "No traces"
		}
		return m, nil

	case timelineLoadedMsg:
		m.spans = msg.spans
		m.stats = msg.stats
		m.spanTree = buildSpanTree(msg.spans)
		m.selectedSpan = 0
		m.showTraceList = false
		m.activePane = PaneTimeline
		m.statusMsg = fmt.Sprintf("%d spans  %d LLM calls  %d tokens",
			msg.stats.TotalSpans, msg.stats.LLMCalls,
			msg.stats.TotalPromptTokens+msg.stats.TotalCompletionTokens)
		if len(m.spanTree) > 0 {
			return m, m.loadMemoryDiffs(m.spanTree[0].span.SpanID)
		}
		return m, nil

	case memoryDiffsLoadedMsg:
		m.memoryDiffs = []*database.MemoryEvent(msg)
		m.diffScroll = 0
		return m, nil

	case errMsg:
		m.err = msg.err
		m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
		return m, nil
	}

	return m, nil
}

// handleKey routes keyboard input based on current mode.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// ── Global ──

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		if !m.showTraceList && !m.searchMode {
			m.activePane = (m.activePane + 1) % 3
		}
		return m, nil

	case "shift+tab":
		if !m.showTraceList && !m.searchMode {
			m.activePane = (m.activePane + 2) % 3
		}
		return m, nil

	case "esc":
		if m.searchMode {
			m.searchMode = false
			m.searchQuery = ""
		} else if !m.showTraceList {
			m.showTraceList = true
			m.activePane = PaneTimeline
		}
		return m, nil

	case "/":
		if !m.searchMode {
			m.searchMode = true
			m.searchQuery = ""
		}
		return m, nil
	}

	// ── Search mode ──

	if m.searchMode {
		switch key {
		case "enter":
			m.searchMode = false
			return m, nil
		case "backspace":
			if len(m.searchQuery) > 0 {
				m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			}
			return m, nil
		default:
			if len(key) == 1 {
				m.searchQuery += key
			}
			return m, nil
		}
	}

	// ── Trace list mode ──

	if m.showTraceList {
		switch key {
		case "j", "down":
			if m.selectedTrace < len(m.traces)-1 {
				m.selectedTrace++
			}
		case "k", "up":
			if m.selectedTrace > 0 {
				m.selectedTrace--
			}
		case "enter":
			if m.selectedTrace < len(m.traces) {
				m.currentTrace = m.traces[m.selectedTrace]
				return m, m.loadTimeline(m.currentTrace.TraceID)
			}
		}
		return m, nil
	}

	// ── Pane-specific ──

	switch m.activePane {
	case PaneTimeline:
		switch key {
		case "j", "down":
			if m.selectedSpan < len(m.spanTree)-1 {
				m.selectedSpan++
				return m, m.loadMemoryDiffs(m.spanTree[m.selectedSpan].span.SpanID)
			}
		case "k", "up":
			if m.selectedSpan > 0 {
				m.selectedSpan--
				return m, m.loadMemoryDiffs(m.spanTree[m.selectedSpan].span.SpanID)
			}
		}

	case PaneDetail:
		// Detail is read-only; scrolling could be added later.

	case PaneMemoryDiff:
		switch key {
		case "j", "down":
			m.diffScroll++
		case "k", "up":
			if m.diffScroll > 0 {
				m.diffScroll--
			}
		}
	}

	return m, nil
}

// ────────────────────────────────────────────────────────────
// View
// ────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	header := renderHeader(&m)
	footer := renderFooter(&m)

	bodyHeight := m.height - 2 // header + footer

	var body string
	if m.showTraceList {
		body = renderTraceList(&m)
	} else {
		body = m.renderMainLayout(bodyHeight)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// renderMainLayout assembles the three-pane debugger view.
func (m Model) renderMainLayout(totalHeight int) string {
	// Responsive: collapse to single pane on narrow terminals
	if m.width < 60 {
		return m.renderCompactLayout(totalHeight)
	}

	// Split proportions
	leftWidth := m.width * 45 / 100
	rightWidth := m.width - leftWidth
	topHeight := totalHeight * 65 / 100
	bottomHeight := totalHeight - topHeight

	// Render panes
	timeline := renderTimelinePanel(&m, leftWidth, topHeight)
	detail := renderDetailPanel(&m, rightWidth, topHeight)
	diff := renderDiffPanel(&m, m.width, bottomHeight)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, timeline, detail)
	return lipgloss.JoinVertical(lipgloss.Left, topRow, diff)
}

// renderCompactLayout is used when the terminal is narrow (< 60 cols).
// Only the focused pane is shown.
func (m Model) renderCompactLayout(totalHeight int) string {
	switch m.activePane {
	case PaneTimeline:
		return renderTimelinePanel(&m, m.width, totalHeight)
	case PaneDetail:
		return renderDetailPanel(&m, m.width, totalHeight)
	case PaneMemoryDiff:
		return renderDiffPanel(&m, m.width, totalHeight)
	default:
		return renderTimelinePanel(&m, m.width, totalHeight)
	}
}
