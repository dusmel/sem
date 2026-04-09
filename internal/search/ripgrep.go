package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExactMatch represents a single match from ripgrep.
type ExactMatch struct {
	FilePath   string     // Absolute path to the file
	RelPath    string     // Relative path within the source
	LineNumber int        // 1-based line number of the match
	LineText   string     // The matched line (trimmed)
	Submatches []Submatch // Positions of matches within the line
	SourceName string     // Name of the source this file belongs to
}

// Submatch represents a single match within a line.
type Submatch struct {
	Start int    // Byte offset start
	End   int    // Byte offset end
	Text  string // Matched text
}

// RipgrepResult holds all matches from a ripgrep invocation.
type RipgrepResult struct {
	Matches []ExactMatch
	Elapsed time.Duration
}

// ripgrep JSON output structs
type rgMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type rgMatchData struct {
	Path       rgPath  `json:"path"`
	Lines      rgText  `json:"lines"`
	LineNumber int     `json:"line_number"`
	Submatches []rgSub `json:"submatches"`
}

type rgPath struct {
	Text string `json:"text"`
}

type rgText struct {
	Text string `json:"text"`
}

type rgSub struct {
	Match rgText `json:"match"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// maxResultsPerFile limits matches per file to avoid overwhelming output
const maxResultsPerFile = 50

// SearchExact runs ripgrep against the given source paths and returns matches.
// If ripgrep is not found, returns an error with install instructions.
// If no matches found, returns an empty RipgrepResult (not an error).
func SearchExact(ctx context.Context, query string, sourcePaths []string, sourceMap map[string]string, maxResults int) (*RipgrepResult, error) {
	start := time.Now()

	// Check if ripgrep is available
	rgPath, ok := IsRipgrepAvailable()
	if !ok {
		return nil, fmt.Errorf("ripgrep (rg) not found. Install with: brew install ripgrep")
	}

	if len(sourcePaths) == 0 {
		return &RipgrepResult{Matches: []ExactMatch{}, Elapsed: time.Since(start)}, nil
	}

	// Build ripgrep command arguments
	args := []string{
		"--json", // JSON output for structured parsing
		"--max-count", fmt.Sprintf("%d", maxResultsPerFile),
		"-i", // Case insensitive
		query,
	}
	args = append(args, sourcePaths...)

	// Execute ripgrep
	cmd := exec.CommandContext(ctx, rgPath, args...)
	stdout, err := cmd.Output()

	// Handle exit codes
	// 0 = matches found
	// 1 = no matches (not an error)
	// 2 = error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// No matches found - this is OK
				return &RipgrepResult{Matches: []ExactMatch{}, Elapsed: time.Since(start)}, nil
			}
			// Exit code 2 or other error
			return nil, fmt.Errorf("ripgrep failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("ripgrep execution failed: %w", err)
	}

	// Parse JSON output
	matches, err := parseRipgrepOutput(stdout, sourceMap)
	if err != nil {
		return nil, fmt.Errorf("parse ripgrep output: %w", err)
	}

	// Limit total results if needed
	if len(matches) > maxResults && maxResults > 0 {
		matches = matches[:maxResults]
	}

	return &RipgrepResult{
		Matches: matches,
		Elapsed: time.Since(start),
	}, nil
}

// parseRipgrepOutput parses the JSON output from ripgrep.
func parseRipgrepOutput(output []byte, sourceMap map[string]string) ([]ExactMatch, error) {
	var matches []ExactMatch

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg rgMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Skip lines that don't parse (shouldn't happen with --json)
			continue
		}

		// We only care about "match" type messages
		if msg.Type != "match" {
			continue
		}

		var matchData rgMatchData
		if err := json.Unmarshal(msg.Data, &matchData); err != nil {
			continue
		}

		// Convert submatches
		submatches := make([]Submatch, 0, len(matchData.Submatches))
		for _, sub := range matchData.Submatches {
			submatches = append(submatches, Submatch{
				Start: sub.Start,
				End:   sub.End,
				Text:  sub.Match.Text,
			})
		}

		filePath := matchData.Path.Text
		sourceName := resolveSourceName(filePath, sourceMap)
		relPath := computeRelPath(filePath, sourceMap)

		matches = append(matches, ExactMatch{
			FilePath:   filePath,
			RelPath:    relPath,
			LineNumber: matchData.LineNumber,
			LineText:   strings.TrimSpace(matchData.Lines.Text),
			Submatches: submatches,
			SourceName: sourceName,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

// resolveSourceName finds which source a file belongs to based on its path.
func resolveSourceName(filePath string, sourceMap map[string]string) string {
	// Find the longest matching source path prefix
	var bestSource string
	var bestLen int

	for sourcePath, sourceName := range sourceMap {
		if strings.HasPrefix(filePath, sourcePath) {
			if len(sourcePath) > bestLen {
				bestLen = len(sourcePath)
				bestSource = sourceName
			}
		}
	}

	return bestSource
}

// computeRelPath computes the relative path within the source.
func computeRelPath(filePath string, sourceMap map[string]string) string {
	// Find the longest matching source path prefix
	var bestPrefix string
	var bestLen int

	for sourcePath := range sourceMap {
		if strings.HasPrefix(filePath, sourcePath) {
			if len(sourcePath) > bestLen {
				bestLen = len(sourcePath)
				bestPrefix = sourcePath
			}
		}
	}

	if bestPrefix != "" {
		rel := strings.TrimPrefix(filePath, bestPrefix)
		rel = strings.TrimPrefix(rel, "/")
		return rel
	}

	return filePath
}

// IsRipgrepAvailable checks if ripgrep is installed and returns its path.
func IsRipgrepAvailable() (string, bool) {
	path, err := exec.LookPath("rg")
	return path, err == nil
}
