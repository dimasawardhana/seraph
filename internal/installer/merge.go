package installer

import (
	"fmt"
	"strings"
)

func mergeBlock(existing, block string) (string, error) {
	start := strings.Index(existing, StartMarker)
	end := strings.Index(existing, EndMarker)

	switch {
	case start < 0 && end < 0:
		if existing == "" {
			return block, nil
		}
		if !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		if !strings.HasSuffix(existing, "\n\n") {
			existing += "\n"
		}
		return existing + block, nil

	case start >= 0 && end > start:
		stop := end + len(EndMarker)
		if stop < len(existing) && existing[stop] == '\n' {
			stop++
		}
		return existing[:start] + block + existing[stop:], nil

	default:
		return "", fmt.Errorf("%s is missing %s or %s; refusing to guess where the block ends",
			describeMarkers(start, end), StartMarker, EndMarker)
	}
}

func describeMarkers(start, end int) string {
	switch {
	case start >= 0 && end < 0:
		return "the block's start marker is present but its end marker is not"
	case start < 0 && end >= 0:
		return "the block's end marker is present but its start marker is not"
	default:
		return "the block's markers are out of order"
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

func lineDiff(before, after string) (removed, added []string) {
	b, a := splitLines(before), splitLines(after)

	prefix := 0
	for prefix < len(b) && prefix < len(a) && b[prefix] == a[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(b)-prefix && suffix < len(a)-prefix &&
		b[len(b)-1-suffix] == a[len(a)-1-suffix] {
		suffix++
	}
	return b[prefix : len(b)-suffix], a[prefix : len(a)-suffix]
}
