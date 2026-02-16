// Package jsonutil provides JSON parsing and manipulation utilities for Oculo.
//
// These helpers are used throughout the codebase for handling
// metadata blobs, tool call arguments/results, and memory values.
package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// PrettyJSON formats a JSON string with indentation for display.
// Returns the original string if it's not valid JSON.
func PrettyJSON(s string) string {
	var obj interface{}
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return s
	}
	pretty, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return s
	}
	return string(pretty)
}

// CompactJSON minifies a JSON string by removing whitespace.
func CompactJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

// SafeUnmarshal attempts to unmarshal JSON into a map.
// Returns an empty map on error instead of failing.
func SafeUnmarshal(s string) map[string]interface{} {
	result := make(map[string]interface{})
	if s == "" {
		return result
	}
	json.Unmarshal([]byte(s), &result)
	return result
}

// MustMarshal marshals a value to JSON, panicking on error.
// Use only for values known to be marshalable (e.g., maps, slices).
func MustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("jsonutil.MustMarshal: %v", err))
	}
	return string(b)
}

// DiffJSON computes the differences between two JSON strings.
// Returns a list of changes with their paths and values.
type JSONDiff struct {
	Path     string `json:"path"`
	Type     string `json:"type"` // "add", "update", "delete"
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value,omitempty"`
}

// ComputeJSONDiff compares two JSON objects and returns the differences.
// This is used for generating memory mutation diffs when the memory
// state is stored as JSON.
func ComputeJSONDiff(oldJSON, newJSON string) ([]JSONDiff, error) {
	var oldMap, newMap map[string]interface{}

	if oldJSON != "" {
		if err := json.Unmarshal([]byte(oldJSON), &oldMap); err != nil {
			return nil, fmt.Errorf("parsing old JSON: %w", err)
		}
	} else {
		oldMap = make(map[string]interface{})
	}

	if newJSON != "" {
		if err := json.Unmarshal([]byte(newJSON), &newMap); err != nil {
			return nil, fmt.Errorf("parsing new JSON: %w", err)
		}
	} else {
		newMap = make(map[string]interface{})
	}

	var diffs []JSONDiff
	diffs = diffMaps("", oldMap, newMap, diffs)
	return diffs, nil
}

func diffMaps(prefix string, oldMap, newMap map[string]interface{}, diffs []JSONDiff) []JSONDiff {
	// Collect all keys
	allKeys := make(map[string]bool)
	for k := range oldMap {
		allKeys[k] = true
	}
	for k := range newMap {
		allKeys[k] = true
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		oldVal, oldExists := oldMap[k]
		newVal, newExists := newMap[k]

		switch {
		case !oldExists && newExists:
			diffs = append(diffs, JSONDiff{
				Path:     path,
				Type:     "add",
				NewValue: toJSONStr(newVal),
			})
		case oldExists && !newExists:
			diffs = append(diffs, JSONDiff{
				Path:     path,
				Type:     "delete",
				OldValue: toJSONStr(oldVal),
			})
		case oldExists && newExists:
			oldStr := toJSONStr(oldVal)
			newStr := toJSONStr(newVal)
			if oldStr != newStr {
				// Check if both are maps for recursive diff
				oldChild, oldIsMap := oldVal.(map[string]interface{})
				newChild, newIsMap := newVal.(map[string]interface{})
				if oldIsMap && newIsMap {
					diffs = diffMaps(path, oldChild, newChild, diffs)
				} else {
					diffs = append(diffs, JSONDiff{
						Path:     path,
						Type:     "update",
						OldValue: oldStr,
						NewValue: newStr,
					})
				}
			}
		}
	}

	return diffs
}

func toJSONStr(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TruncateString truncates a string to maxLen characters, adding "..."
// if truncation occurred. Used for display in the TUI.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
