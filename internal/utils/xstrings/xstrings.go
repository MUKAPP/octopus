package xstrings

import "strings"

// SplitTrimCompact splits strings by sep, trims whitespace, and drops empty items.
// Commonly used for parsing comma-separated configs like "a, b, ,c,".
func SplitTrimCompact(sep string, parts ...string) []string {
	out := make([]string, 0)
	for _, p := range parts {
		if p == "" {
			continue
		}
		for _, item := range strings.Split(p, sep) {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
	}
	return out
}

// SplitCompact splits non-empty inputs by sep without trimming or filtering split items.
func SplitCompact(sep string, parts ...string) []string {
	out := make([]string, 0)
	for _, part := range parts {
		if part != "" {
			out = append(out, strings.Split(part, sep)...)
		}
	}
	return out
}

// TrimCompact trims whitespace and drops empty items in a string slice.
func TrimCompact(items []string) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}
