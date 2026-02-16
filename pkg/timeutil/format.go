// Package timeutil provides time formatting utilities for Oculo.
//
// All timestamps in Oculo are stored as Unix nanoseconds (int64).
// This package handles conversion to human-readable formats
// for the TUI and report generation.
package timeutil

import (
	"fmt"
	"time"
)

// FromNano converts a Unix nanosecond timestamp to time.Time.
func FromNano(ns int64) time.Time {
	return time.Unix(0, ns)
}

// ToNano converts a time.Time to Unix nanoseconds.
func ToNano(t time.Time) int64 {
	return t.UnixNano()
}

// NowNano returns the current time as Unix nanoseconds.
func NowNano() int64 {
	return time.Now().UnixNano()
}

// FormatTimestamp formats a Unix nanosecond timestamp for display
// in the TUI timeline view. Format: "HH:MM:SS.mmm"
func FormatTimestamp(ns int64) string {
	t := FromNano(ns)
	return t.Format("15:04:05.000")
}

// FormatTimestampFull formats a Unix nanosecond timestamp with date.
// Format: "2006-01-02 15:04:05.000"
func FormatTimestampFull(ns int64) string {
	t := FromNano(ns)
	return t.Format("2006-01-02 15:04:05.000")
}

// FormatDuration formats a duration in milliseconds to a human-readable string.
// Examples: "1.2s", "450ms", "2m 15.3s"
func FormatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	seconds := float64(ms) / 1000.0
	if seconds < 60 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	minutes := int(seconds / 60)
	remaining := seconds - float64(minutes*60)
	return fmt.Sprintf("%dm %.1fs", minutes, remaining)
}

// RelativeTime returns a human-readable relative time string.
// Examples: "just now", "5s ago", "2m ago", "1h ago"
func RelativeTime(ns int64) string {
	diff := time.Since(FromNano(ns))

	switch {
	case diff < time.Second:
		return "just now"
	case diff < time.Minute:
		return fmt.Sprintf("%ds ago", int(diff.Seconds()))
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}
