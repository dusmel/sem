package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// MatchedTerm represents a highlighted term from ripgrep submatches.
type MatchedTerm struct {
	Start int // Byte offset start in the snippet
	End   int // Byte offset end in the snippet
}

type SearchResult struct {
	ChunkID      string         `json:"chunk_id"`
	FilePath     string         `json:"file_path"`
	Snippet      string         `json:"snippet"`
	Score        float32        `json:"score"`
	SourceName   string         `json:"source_name"`
	MatchedTerms []MatchedTerm  `json:"-"` // Not serialized to JSON
	Metadata     ResultMetadata `json:"metadata"`
}

type ResultMetadata struct {
	FileKind     string `json:"file_kind"`
	Language     string `json:"language,omitempty"`
	Title        string `json:"title,omitempty"`
	StartLine    int    `json:"start_line"`
	EndLine      int    `json:"end_line"`
	FunctionName string `json:"function_name,omitempty"`
	SectionLevel int    `json:"section_level,omitempty"`
}

type SearchFilters struct {
	Source   string `json:"source,omitempty"`
	Language string `json:"language,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Dir      string `json:"dir,omitempty"`
}

type SearchResponse struct {
	Query     string         `json:"query"`
	Mode      string         `json:"mode"`
	Filters   SearchFilters  `json:"filters,omitempty"`
	Results   []SearchResult `json:"results"`
	Total     int            `json:"total"`
	ElapsedMS int64          `json:"elapsed_ms"`
}

func PrintJSON(w io.Writer, response SearchResponse) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(response)
}

func PrintHuman(w io.Writer, response SearchResponse) {
	if len(response.Results) == 0 {
		fmt.Fprintln(w, "No results found.")
		return
	}

	for i, result := range response.Results {
		fmt.Fprintf(w, "%d. %s\n", i+1, result.FilePath)
		snippet := cleanSnippet(result.Snippet)
		if len(result.MatchedTerms) > 0 {
			snippet = highlightSnippet(snippet, result.MatchedTerms)
		}
		fmt.Fprintf(w, "   %q\n", snippet)
		fmt.Fprintf(w, "   score: %.4f | source: %s | lines: %d-%d\n", result.Score, result.SourceName, result.Metadata.StartLine, result.Metadata.EndLine)
		if result.Metadata.Title != "" {
			fmt.Fprintf(w, "   title: %s\n", result.Metadata.Title)
		}
		fmt.Fprintln(w)
	}
}

func cleanSnippet(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 180 {
		return value
	}
	return value[:177] + "..."
}

// highlightSnippet wraps matched substrings in ANSI bold yellow codes.
// MatchedTerm positions are byte offsets into the snippet text.
func highlightSnippet(snippet string, terms []MatchedTerm) string {
	if len(terms) == 0 || len(snippet) == 0 {
		return snippet
	}

	type matchRange struct {
		start int
		end   int
	}

	var ranges []matchRange
	for _, t := range terms {
		if t.Start < len(snippet) && t.End <= len(snippet) && t.Start < t.End {
			ranges = append(ranges, matchRange{start: t.Start, end: t.End})
		}
	}

	if len(ranges) == 0 {
		return snippet
	}

	// Sort ranges by start position
	for i := 1; i < len(ranges); i++ {
		for j := i; j > 0 && ranges[j].start < ranges[j-1].start; j-- {
			ranges[j], ranges[j-1] = ranges[j-1], ranges[j]
		}
	}

	// Merge overlapping ranges
	merged := []matchRange{ranges[0]}
	for i := 1; i < len(ranges); i++ {
		last := &merged[len(merged)-1]
		if ranges[i].start <= last.end {
			if ranges[i].end > last.end {
				last.end = ranges[i].end
			}
		} else {
			merged = append(merged, ranges[i])
		}
	}

	// Build highlighted string
	const highlightOn = "\033[1;33m"
	const highlightOff = "\033[0m"

	var b strings.Builder
	b.Grow(len(snippet) + len(highlightOn)*len(merged) + len(highlightOff)*len(merged))
	pos := 0
	for _, r := range merged {
		if r.start > pos {
			b.WriteString(snippet[pos:r.start])
		}
		b.WriteString(highlightOn)
		b.WriteString(snippet[r.start:r.end])
		b.WriteString(highlightOff)
		pos = r.end
	}
	if pos < len(snippet) {
		b.WriteString(snippet[pos:])
	}

	return b.String()
}
