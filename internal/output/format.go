package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type SearchResult struct {
	ChunkID    string         `json:"chunk_id"`
	FilePath   string         `json:"file_path"`
	Snippet    string         `json:"snippet"`
	Score      float32        `json:"score"`
	SourceName string         `json:"source_name"`
	Metadata   ResultMetadata `json:"metadata"`
}

type ResultMetadata struct {
	FileKind  string `json:"file_kind"`
	Language  string `json:"language,omitempty"`
	Title     string `json:"title,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type SearchResponse struct {
	Query     string         `json:"query"`
	Mode      string         `json:"mode"`
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
		fmt.Fprintf(w, "   %q\n", cleanSnippet(result.Snippet))
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
