package tui

import "github.com/charmbracelet/lipgloss"

// ────────────────────────────────────────────────────────────
// Color Palette — GitHub Dark aesthetic
// ────────────────────────────────────────────────────────────
//
// All colors are defined here. No ad-hoc color literals anywhere.
// Designed for readability in dark terminals (iTerm2, Windows
// Terminal, Ghostty, Alacritty) and comfortable for long
// debugging sessions.

var (
	// Base
	colorBg        = lipgloss.Color("#0d1117")
	colorBgPanel   = lipgloss.Color("#161b22")
	colorBgSurface = lipgloss.Color("#1c2128")

	// Text
	colorText      = lipgloss.Color("#e6edf3")
	colorTextDim   = lipgloss.Color("#8b949e")
	colorTextMuted = lipgloss.Color("#484f58")

	// Accents
	colorBlue   = lipgloss.Color("#58a6ff")
	colorGreen  = lipgloss.Color("#3fb950")
	colorRed    = lipgloss.Color("#f85149")
	colorYellow = lipgloss.Color("#d29922")
	colorPurple = lipgloss.Color("#bc8cff")
	colorCyan   = lipgloss.Color("#76e3ea")

	// Structural
	colorDivider   = lipgloss.Color("#30363d")
	colorHighlight = lipgloss.Color("#1f6feb")
)

// ────────────────────────────────────────────────────────────
// Component Styles
// ────────────────────────────────────────────────────────────

// Header bar
var (
	headerBarStyle = lipgloss.NewStyle().
			Background(colorBgSurface).
			Foreground(colorText).
			Padding(0, 1)

	headerBrandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorBlue)

	headerSepStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted)

	headerMetaStyle = lipgloss.NewStyle().
			Foreground(colorTextDim)
)

// Panel chrome
var (
	panelStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.Border{
			Top:    "─",
			Bottom: "",
			Left:   "",
			Right:  "",
		}).
		BorderForeground(colorDivider)

	panelActiveStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Border(lipgloss.Border{
			Top:    "─",
			Bottom: "",
			Left:   "",
			Right:  "",
		}).
		BorderForeground(colorBlue)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	panelTitleDimStyle = lipgloss.NewStyle().
				Foreground(colorTextMuted).
				Bold(true)
)

// Timeline tree
var (
	spanNormalStyle = lipgloss.NewStyle().
			Foreground(colorText)

	spanSelectedStyle = lipgloss.NewStyle().
				Background(colorHighlight).
				Foreground(colorText).
				Bold(true)

	spanLLMStyle = lipgloss.NewStyle().
			Foreground(colorPurple)

	spanToolStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	spanMemoryStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	spanPlanningStyle = lipgloss.NewStyle().
				Foreground(colorCyan)

	spanRetrievalStyle = lipgloss.NewStyle().
				Foreground(colorBlue)

	treeBranchStyle = lipgloss.NewStyle().
			Foreground(colorDivider)

	treeTimestampStyle = lipgloss.NewStyle().
				Foreground(colorTextMuted)

	treeDurationStyle = lipgloss.NewStyle().
				Foreground(colorTextDim)
)

// Detail pane
var (
	detailLabelStyle = lipgloss.NewStyle().
				Foreground(colorBlue)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(colorText)

	detailSectionStyle = lipgloss.NewStyle().
				Foreground(colorDivider)

	tokenBarPromptStyle = lipgloss.NewStyle().
				Foreground(colorBlue)

	tokenBarCompletionStyle = lipgloss.NewStyle().
				Foreground(colorPurple)

	tokenBarEmptyStyle = lipgloss.NewStyle().
				Foreground(colorTextMuted)
)

// Memory diff
var (
	diffAddStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	diffDelStyle = lipgloss.NewStyle().
			Foreground(colorRed)

	diffModStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	diffContextStyle = lipgloss.NewStyle().
				Foreground(colorTextMuted)

	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)
)

// Footer / status bar
var (
	statusStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorBgSurface).
			Padding(0, 1)

	statusAccentStyle = lipgloss.NewStyle().
				Foreground(colorBlue).
				Background(colorBgSurface).
				Bold(true).
				Padding(0, 1)

	hintKeyStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true)

	hintDescStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted)
)

// Trace list
var (
	traceItemStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(0, 1)

	traceSelectedStyle = lipgloss.NewStyle().
				Background(colorHighlight).
				Foreground(colorText).
				Bold(true).
				Padding(0, 1)

	traceStatusOk = lipgloss.NewStyle().
			Foreground(colorGreen)

	traceStatusFail = lipgloss.NewStyle().
			Foreground(colorRed)

	traceStatusRunning = lipgloss.NewStyle().
				Foreground(colorYellow)

	traceDimStyle = lipgloss.NewStyle().
			Foreground(colorTextDim)

	emptyStateStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted).
			Padding(2, 4)
)

// Search bar
var (
	searchBarStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorBgSurface).
			Padding(0, 1)

	searchCursorStyle = lipgloss.NewStyle().
				Background(colorBlue).
				Foreground(colorBg)
)
