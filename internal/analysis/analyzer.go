// Package analysis provides lightweight, deterministic anomaly detection
// for AI agent traces. All analysis uses mathematical and statistical
// methods — no LLMs are involved.
//
// Key capabilities:
//   - Token hotspot detection via Z-score analysis
//   - Memory growth trend analysis via linear regression
//   - Cost attribution across LLM calls
//   - Prompt clustering via similarity metrics
package analysis

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Mr-Dark-debug/oculo/internal/database"
	"github.com/Mr-Dark-debug/oculo/pkg/timeutil"
)

// Analyzer performs semantic analysis on trace data without LLMs.
type Analyzer struct {
	store database.Store
}

// NewAnalyzer creates a new analysis engine backed by the given store.
func NewAnalyzer(store database.Store) *Analyzer {
	return &Analyzer{store: store}
}

// ============================================================
// Token Hotspot Detection
// ============================================================

// TokenHotspot identifies a span with abnormally high token consumption.
type TokenHotspot struct {
	SpanID           string  `json:"span_id"`
	OperationName    string  `json:"operation_name"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	ZScore           float64 `json:"z_score"`
	Severity         string  `json:"severity"` // "low", "medium", "high"
}

// DetectTokenHotspots calculates the Z-score of token usage across all spans
// in a trace, identifying outliers that consume disproportionate tokens.
//
// A Z-score > 2.0 is considered a hotspot ("medium" severity).
// A Z-score > 3.0 is a significant hotspot ("high" severity).
//
// This answers: "Which LLM calls are consuming the most tokens?"
func (a *Analyzer) DetectTokenHotspots(traceID string) ([]TokenHotspot, error) {
	spans, err := a.store.QueryTimeline(traceID)
	if err != nil {
		return nil, fmt.Errorf("querying timeline for hotspot analysis: %w", err)
	}

	// Filter to LLM spans only
	var llmSpans []*database.Span
	for _, s := range spans {
		if s.OperationType == "LLM" {
			llmSpans = append(llmSpans, s)
		}
	}

	if len(llmSpans) < 2 {
		// Not enough data for meaningful Z-score analysis
		return nil, nil
	}

	// Calculate total tokens per span
	totals := make([]float64, len(llmSpans))
	var sum, sumSq float64
	for i, s := range llmSpans {
		total := float64(s.PromptTokens + s.CompletionTokens)
		totals[i] = total
		sum += total
		sumSq += total * total
	}

	n := float64(len(llmSpans))
	mean := sum / n
	variance := (sumSq / n) - (mean * mean)
	stddev := math.Sqrt(variance)

	if stddev == 0 {
		// All spans have the same token count — no hotspots
		return nil, nil
	}

	var hotspots []TokenHotspot
	for i, s := range llmSpans {
		zScore := (totals[i] - mean) / stddev

		if zScore > 1.5 {
			severity := "low"
			if zScore > 3.0 {
				severity = "high"
			} else if zScore > 2.0 {
				severity = "medium"
			}

			hotspots = append(hotspots, TokenHotspot{
				SpanID:           s.SpanID,
				OperationName:    s.OperationName,
				PromptTokens:     s.PromptTokens,
				CompletionTokens: s.CompletionTokens,
				TotalTokens:      s.PromptTokens + s.CompletionTokens,
				ZScore:           math.Round(zScore*100) / 100,
				Severity:         severity,
			})
		}
	}

	// Sort by Z-score descending
	sort.Slice(hotspots, func(i, j int) bool {
		return hotspots[i].ZScore > hotspots[j].ZScore
	})

	return hotspots, nil
}

// ============================================================
// Memory Growth Analysis
// ============================================================

// MemoryGrowthReport contains the results of memory growth analysis.
type MemoryGrowthReport struct {
	TraceID         string           `json:"trace_id"`
	TotalKeys       int              `json:"total_keys"`
	TotalEvents     int              `json:"total_events"`
	GrowthRate      float64          `json:"growth_rate"`       // Keys per second
	Slope           float64          `json:"slope"`             // Linear regression slope
	Intercept       float64          `json:"intercept"`         // Linear regression intercept
	RSquared        float64          `json:"r_squared"`         // Goodness of fit
	Prediction30Min int              `json:"prediction_30_min"` // Predicted key count in 30 minutes
	IsUnbounded     bool             `json:"is_unbounded"`      // True if growth appears unbounded
	KeyGrowth       []KeyGrowthEntry `json:"key_growth"`
}

// KeyGrowthEntry tracks when a specific key was added to memory.
type KeyGrowthEntry struct {
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
	Timestamp string `json:"timestamp"`
	Operation string `json:"operation"`
}

// dataPoint represents a single time-series observation for regression analysis.
type dataPoint struct {
	timestamp float64 // Seconds since first event
	keyCount  float64
}

// AnalyzeMemoryGrowth performs linear regression on cumulative memory size
// over time to predict whether the agent's memory will grow unbounded.
//
// This answers: "Is this agent accumulating too much state?"
func (a *Analyzer) AnalyzeMemoryGrowth(traceID string) (*MemoryGrowthReport, error) {
	spans, err := a.store.QueryTimeline(traceID)
	if err != nil {
		return nil, fmt.Errorf("querying timeline for memory analysis: %w", err)
	}

	// Collect all memory events across all spans
	var allEvents []*database.MemoryEvent
	keySet := make(map[string]bool)
	for _, s := range spans {
		events, err := a.store.GetMemoryDiffs(s.SpanID)
		if err != nil {
			continue
		}
		allEvents = append(allEvents, events...)
	}

	if len(allEvents) < 2 {
		return &MemoryGrowthReport{
			TraceID:     traceID,
			TotalKeys:   0,
			TotalEvents: len(allEvents),
		}, nil
	}

	// Sort by timestamp
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Timestamp < allEvents[j].Timestamp
	})

	baseTime := allEvents[0].Timestamp
	var points []dataPoint
	var keyGrowth []KeyGrowthEntry

	for _, ev := range allEvents {
		switch ev.Operation {
		case "ADD":
			keySet[ev.Namespace+"."+ev.Key] = true
		case "DELETE":
			delete(keySet, ev.Namespace+"."+ev.Key)
		}

		t := float64(ev.Timestamp-baseTime) / 1e9 // Convert to seconds
		points = append(points, dataPoint{
			timestamp: t,
			keyCount:  float64(len(keySet)),
		})

		keyGrowth = append(keyGrowth, KeyGrowthEntry{
			Key:       ev.Key,
			Namespace: ev.Namespace,
			Timestamp: timeutil.FormatTimestamp(ev.Timestamp),
			Operation: ev.Operation,
		})
	}

	// Linear regression: y = mx + b
	slope, intercept, rSquared := linearRegression(points)

	// Predict key count in 30 minutes
	lastTime := points[len(points)-1].timestamp
	prediction30Min := slope*(lastTime+1800) + intercept

	// Determine if growth is unbounded
	isUnbounded := slope > 0.1 && rSquared > 0.7

	report := &MemoryGrowthReport{
		TraceID:         traceID,
		TotalKeys:       len(keySet),
		TotalEvents:     len(allEvents),
		GrowthRate:      math.Round(slope*100) / 100,
		Slope:           math.Round(slope*1000) / 1000,
		Intercept:       math.Round(intercept*100) / 100,
		RSquared:        math.Round(rSquared*1000) / 1000,
		Prediction30Min: int(math.Max(0, prediction30Min)),
		IsUnbounded:     isUnbounded,
		KeyGrowth:       keyGrowth,
	}

	return report, nil
}

// linearRegression computes ordinary least squares regression.
// Returns slope (m), intercept (b), and R-squared goodness of fit.
func linearRegression(points []dataPoint) (slope, intercept, rSquared float64) {
	n := float64(len(points))
	if n < 2 {
		return 0, 0, 0
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for _, p := range points {
		sumX += p.timestamp
		sumY += p.keyCount
		sumXY += p.timestamp * p.keyCount
		sumX2 += p.timestamp * p.timestamp
		sumY2 += p.keyCount * p.keyCount
	}

	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return 0, sumY / n, 0
	}

	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n

	// R-squared
	meanY := sumY / n
	var ssRes, ssTot float64
	for _, p := range points {
		predicted := slope*p.timestamp + intercept
		ssRes += (p.keyCount - predicted) * (p.keyCount - predicted)
		ssTot += (p.keyCount - meanY) * (p.keyCount - meanY)
	}

	if ssTot == 0 {
		rSquared = 1.0
	} else {
		rSquared = 1 - ssRes/ssTot
	}

	return slope, intercept, rSquared
}

// ============================================================
// Cost Attribution
// ============================================================

// CostEntry attributes token cost to a specific operation.
type CostEntry struct {
	SpanID           string  `json:"span_id"`
	OperationName    string  `json:"operation_name"`
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	EstimatedCost    float64 `json:"estimated_cost_usd"`
	Percentage       float64 `json:"percentage"`
}

// CostReport summarizes token costs across a trace.
type CostReport struct {
	TraceID               string      `json:"trace_id"`
	TotalPromptTokens     int         `json:"total_prompt_tokens"`
	TotalCompletionTokens int         `json:"total_completion_tokens"`
	TotalEstimatedCost    float64     `json:"total_estimated_cost_usd"`
	Entries               []CostEntry `json:"entries"`
}

// Model pricing (approximate, per 1K tokens)
var modelPricing = map[string][2]float64{
	"gpt-4":           {0.03, 0.06},
	"gpt-4-turbo":     {0.01, 0.03},
	"gpt-4o":          {0.005, 0.015},
	"gpt-4o-mini":     {0.00015, 0.0006},
	"gpt-3.5-turbo":   {0.0005, 0.0015},
	"claude-3-opus":   {0.015, 0.075},
	"claude-3-sonnet": {0.003, 0.015},
	"claude-3-haiku":  {0.00025, 0.00125},
}

// AttributeCosts calculates estimated costs for each LLM call in a trace.
func (a *Analyzer) AttributeCosts(traceID string) (*CostReport, error) {
	spans, err := a.store.QueryTimeline(traceID)
	if err != nil {
		return nil, fmt.Errorf("querying timeline for cost analysis: %w", err)
	}

	report := &CostReport{TraceID: traceID}

	for _, s := range spans {
		if s.OperationType != "LLM" {
			continue
		}

		model := "unknown"
		if s.Model != nil {
			model = *s.Model
		}

		pricing, ok := modelPricing[model]
		if !ok {
			pricing = [2]float64{0.01, 0.03} // Default estimate
		}

		promptCost := float64(s.PromptTokens) / 1000.0 * pricing[0]
		completionCost := float64(s.CompletionTokens) / 1000.0 * pricing[1]
		totalCost := promptCost + completionCost

		report.TotalPromptTokens += s.PromptTokens
		report.TotalCompletionTokens += s.CompletionTokens
		report.TotalEstimatedCost += totalCost

		report.Entries = append(report.Entries, CostEntry{
			SpanID:           s.SpanID,
			OperationName:    s.OperationName,
			Model:            model,
			PromptTokens:     s.PromptTokens,
			CompletionTokens: s.CompletionTokens,
			EstimatedCost:    math.Round(totalCost*10000) / 10000,
		})
	}

	// Calculate percentages
	for i := range report.Entries {
		if report.TotalEstimatedCost > 0 {
			report.Entries[i].Percentage = math.Round(
				report.Entries[i].EstimatedCost/report.TotalEstimatedCost*10000) / 100
		}
	}

	return report, nil
}

// ============================================================
// Full Analysis Report
// ============================================================

// AnalysisReport is the complete output of `oculo analyze`.
type AnalysisReport struct {
	TraceID         string               `json:"trace_id"`
	GeneratedAt     string               `json:"generated_at"`
	Stats           *database.TraceStats `json:"stats"`
	TokenHotspots   []TokenHotspot       `json:"token_hotspots"`
	MemoryGrowth    *MemoryGrowthReport  `json:"memory_growth"`
	CostAttribution *CostReport          `json:"cost_attribution"`
	Warnings        []string             `json:"warnings"`
}

// FullAnalysis runs all analysis passes and generates a comprehensive report.
func (a *Analyzer) FullAnalysis(traceID string) (*AnalysisReport, error) {
	report := &AnalysisReport{
		TraceID:     traceID,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	// Gather statistics
	stats, err := a.store.GetTraceStats(traceID)
	if err != nil {
		return nil, fmt.Errorf("gathering trace stats: %w", err)
	}
	report.Stats = stats

	// Token hotspots
	hotspots, err := a.DetectTokenHotspots(traceID)
	if err != nil {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Token hotspot analysis failed: %v", err))
	} else {
		report.TokenHotspots = hotspots
	}

	// Memory growth
	memGrowth, err := a.AnalyzeMemoryGrowth(traceID)
	if err != nil {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Memory growth analysis failed: %v", err))
	} else {
		report.MemoryGrowth = memGrowth
	}

	// Cost attribution
	costReport, err := a.AttributeCosts(traceID)
	if err != nil {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Cost attribution failed: %v", err))
	} else {
		report.CostAttribution = costReport
	}

	// Generate warnings based on analysis
	if memGrowth != nil && memGrowth.IsUnbounded {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("⚠ UNBOUNDED MEMORY GROWTH detected (slope=%.3f keys/sec, R²=%.3f). "+
				"Agent may accumulate excessive state.", memGrowth.Slope, memGrowth.RSquared))
	}

	for _, h := range hotspots {
		if h.Severity == "high" {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("⚠ TOKEN HOTSPOT: %s consumed %d tokens (Z-score: %.2f). "+
					"Consider prompt optimization.", h.OperationName, h.TotalTokens, h.ZScore))
		}
	}

	return report, nil
}

// FormatReport generates a human-readable markdown report.
func (a *Analyzer) FormatReport(report *AnalysisReport) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Oculo Analysis Report\n\n"))
	b.WriteString(fmt.Sprintf("**Trace ID:** `%s`\n", report.TraceID))
	b.WriteString(fmt.Sprintf("**Generated:** %s\n\n", report.GeneratedAt))

	// Stats
	if report.Stats != nil {
		b.WriteString("## Execution Summary\n\n")
		b.WriteString(fmt.Sprintf("| Metric | Value |\n"))
		b.WriteString(fmt.Sprintf("|--------|-------|\n"))
		b.WriteString(fmt.Sprintf("| Total Spans | %d |\n", report.Stats.TotalSpans))
		b.WriteString(fmt.Sprintf("| LLM Calls | %d |\n", report.Stats.LLMCalls))
		b.WriteString(fmt.Sprintf("| Tool Calls | %d |\n", report.Stats.ToolCalls))
		b.WriteString(fmt.Sprintf("| Memory Operations | %d |\n", report.Stats.MemoryOps))
		b.WriteString(fmt.Sprintf("| Total Prompt Tokens | %d |\n", report.Stats.TotalPromptTokens))
		b.WriteString(fmt.Sprintf("| Total Completion Tokens | %d |\n", report.Stats.TotalCompletionTokens))
		b.WriteString(fmt.Sprintf("| Total Duration | %s |\n\n", timeutil.FormatDuration(report.Stats.TotalDurationMs)))
	}

	// Token Hotspots
	if len(report.TokenHotspots) > 0 {
		b.WriteString("## Token Hotspots\n\n")
		b.WriteString("| Operation | Tokens | Z-Score | Severity |\n")
		b.WriteString("|-----------|--------|---------|----------|\n")
		for _, h := range report.TokenHotspots {
			b.WriteString(fmt.Sprintf("| %s | %d | %.2f | %s |\n",
				h.OperationName, h.TotalTokens, h.ZScore, h.Severity))
		}
		b.WriteString("\n")
	}

	// Memory Growth
	if report.MemoryGrowth != nil {
		mg := report.MemoryGrowth
		b.WriteString("## Memory Growth Analysis\n\n")
		b.WriteString(fmt.Sprintf("- **Current Keys:** %d\n", mg.TotalKeys))
		b.WriteString(fmt.Sprintf("- **Total Events:** %d\n", mg.TotalEvents))
		b.WriteString(fmt.Sprintf("- **Growth Rate:** %.2f keys/sec\n", mg.GrowthRate))
		b.WriteString(fmt.Sprintf("- **R² Fit:** %.3f\n", mg.RSquared))
		b.WriteString(fmt.Sprintf("- **30-min Prediction:** %d keys\n", mg.Prediction30Min))
		if mg.IsUnbounded {
			b.WriteString("- **⚠ WARNING:** Unbounded growth detected!\n")
		}
		b.WriteString("\n")
	}

	// Cost Attribution
	if report.CostAttribution != nil {
		ca := report.CostAttribution
		b.WriteString("## Cost Attribution\n\n")
		b.WriteString(fmt.Sprintf("**Total Estimated Cost:** $%.4f\n\n", ca.TotalEstimatedCost))
		if len(ca.Entries) > 0 {
			b.WriteString("| Operation | Model | Tokens | Cost | % |\n")
			b.WriteString("|-----------|-------|--------|------|---|\n")
			for _, e := range ca.Entries {
				b.WriteString(fmt.Sprintf("| %s | %s | %d | $%.4f | %.1f%% |\n",
					e.OperationName, e.Model,
					e.PromptTokens+e.CompletionTokens,
					e.EstimatedCost, e.Percentage))
			}
		}
		b.WriteString("\n")
	}

	// Warnings
	if len(report.Warnings) > 0 {
		b.WriteString("## Warnings\n\n")
		for _, w := range report.Warnings {
			b.WriteString(fmt.Sprintf("- %s\n", w))
		}
	}

	return b.String()
}
