package output

import "testing"

func TestHighlightSnippet(t *testing.T) {
	tests := []struct {
		name     string
		snippet  string
		terms    []MatchedTerm
		expected string
	}{
		{
			name:     "single match",
			snippet:  "this is an error message",
			terms:    []MatchedTerm{{Start: 11, End: 16}},
			expected: "this is an \033[1;33merror\033[0m message",
		},
		{
			name:     "multiple matches",
			snippet:  "error in error handler",
			terms:    []MatchedTerm{{Start: 0, End: 5}, {Start: 9, End: 14}},
			expected: "\033[1;33merror\033[0m in \033[1;33merror\033[0m handler",
		},
		{
			name:     "no terms",
			snippet:  "plain text",
			terms:    nil,
			expected: "plain text",
		},
		{
			name:     "empty snippet",
			snippet:  "",
			terms:    []MatchedTerm{{Start: 0, End: 5}},
			expected: "",
		},
		{
			name:     "overlapping ranges",
			snippet:  "hello world",
			terms:    []MatchedTerm{{Start: 0, End: 5}, {Start: 3, End: 8}},
			expected: "\033[1;33mhello wo\033[0mrld",
		},
		{
			name:     "out of bounds ignored",
			snippet:  "short",
			terms:    []MatchedTerm{{Start: 0, End: 100}},
			expected: "short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlightSnippet(tt.snippet, tt.terms)
			if got != tt.expected {
				t.Errorf("highlightSnippet(%q, %v) = %q, want %q",
					tt.snippet, tt.terms, got, tt.expected)
			}
		})
	}
}
