package chunk

import (
	"regexp"
	"strings"
)

// codeBoundary represents a detected boundary in code.
type codeBoundary struct {
	Line int    // 1-based line number
	Type string // "function", "class", "method", "type"
	Name string // Name of the function/class/etc.
}

// languagePatterns maps file extensions to regex patterns for detecting boundaries.
var languagePatterns = map[string][]struct {
	Pattern *regexp.Regexp
	Type    string
}{
	"go": {
		{regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`), "function"},
		{regexp.MustCompile(`^type\s+(\w+)\s+(?:struct|interface|func|map|chan|\[)`), "type"},
	},
	"py": {
		{regexp.MustCompile(`^(?:async\s+)?def\s+(\w+)\s*\([^)]*\)\s*:`), "function"},
		{regexp.MustCompile(`^class\s+(\w+)\s*(?:\([^)]*\))?\s*:`), "class"},
	},
	"js": {
		{regexp.MustCompile(`^(?:async\s+)?function\s+(\w+)\s*\(`), "function"},
		{regexp.MustCompile(`^(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?(?:function|\([^)]*\)\s*=>)`), "function"},
		{regexp.MustCompile(`^class\s+(\w+)(?:\s+extends\s+\w+)?\s*\{`), "class"},
	},
	"ts": {
		{regexp.MustCompile(`^(?:async\s+)?function\s+(\w+)\s*\(`), "function"},
		{regexp.MustCompile(`^(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?(?:function|\([^)]*\)\s*=>)`), "function"},
		{regexp.MustCompile(`^class\s+(\w+)(?:\s+extends\s+\w+)?\s*\{`), "class"},
		{regexp.MustCompile(`^(?:export\s+)?(?:abstract\s+)?class\s+(\w+)`), "class"},
	},
	"rs": {
		{regexp.MustCompile(`^(?:pub\s+)?(?:async\s+)?fn\s+(\w+)\s*\(`), "function"},
		{regexp.MustCompile(`^(?:pub\s+)?(?:struct|enum|trait|impl)\s+(?:<[^>]+>\s+)?(\w+)`), "type"},
	},
}

// findCodeBoundaries detects function/class/method boundaries in code content.
func findCodeBoundaries(content string, extension string) []codeBoundary {
	patterns, ok := languagePatterns[extension]
	if !ok {
		return nil // No patterns for this language
	}

	var boundaries []codeBoundary
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, p := range patterns {
			matches := p.Pattern.FindStringSubmatch(trimmed)
			if matches != nil {
				name := matches[1]
				boundaries = append(boundaries, codeBoundary{
					Line: lineNum + 1,
					Type: p.Type,
					Name: name,
				})
				break // Only one match per line
			}
		}
	}

	return boundaries
}

// chunkByCodeBoundaries splits content by code boundaries (functions, classes, etc.).
// Falls back to character-window splitting if no boundaries found or section too large.
func chunkByCodeBoundaries(content string, extension string, maxChars int, overlapChars int) ([]window, []codeBoundary) {
	boundaries := findCodeBoundaries(content, extension)

	if len(boundaries) == 0 {
		// No boundaries found, fall back to character windows
		return splitWindows([]rune(content), maxChars, overlapChars), nil
	}

	runes := []rune(content)
	lines := strings.Split(content, "\n")

	// Build a map: line number → rune index
	lineToRune := make([]int, len(lines)+1)
	currentRune := 0
	for i, line := range lines {
		lineToRune[i] = currentRune
		currentRune += len([]rune(line)) + 1 // +1 for newline
	}
	lineToRune[len(lines)] = len(runes)

	var windows []window
	var matchedBoundaries []codeBoundary

	// Handle content before first boundary
	if boundaries[0].Line > 1 {
		firstBoundaryRune := lineToRune[boundaries[0].Line-1]
		if firstBoundaryRune > 0 {
			preWindows := splitWindows(runes[:firstBoundaryRune], maxChars, overlapChars)
			windows = append(windows, preWindows...)
			// No boundary info for pre-content
			for range preWindows {
				matchedBoundaries = append(matchedBoundaries, codeBoundary{})
			}
		}
	}

	for i, bound := range boundaries {
		startRune := lineToRune[bound.Line-1]

		// Find end: next boundary or end of file
		var endRune int
		if i+1 < len(boundaries) {
			endRune = lineToRune[boundaries[i+1].Line-1]
		} else {
			endRune = len(runes)
		}

		chunkLen := endRune - startRune

		if chunkLen <= maxChars {
			// Chunk fits within limit
			windows = append(windows, window{Start: startRune, End: endRune})
			matchedBoundaries = append(matchedBoundaries, bound)
		} else {
			// Chunk too large, split with character windows
			sectionRunes := runes[startRune:endRune]
			subWindows := splitWindows(sectionRunes, maxChars, overlapChars)
			for j, sw := range subWindows {
				windows = append(windows, window{
					Start: startRune + sw.Start,
					End:   startRune + sw.End,
				})
				// Only the first sub-window gets the boundary info
				if j == 0 {
					matchedBoundaries = append(matchedBoundaries, bound)
				} else {
					matchedBoundaries = append(matchedBoundaries, codeBoundary{})
				}
			}
		}
	}

	if len(windows) == 0 {
		windows = append(windows, window{Start: 0, End: len(runes)})
		matchedBoundaries = append(matchedBoundaries, codeBoundary{})
	}

	return windows, matchedBoundaries
}
