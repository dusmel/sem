package embed

import "testing"

func TestStripAccents(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"café", "café", "cafe"},
		{"naïve", "naïve", "naive"},
		{"résumé", "résumé", "resume"},
		{"Zürich", "Zürich", "Zurich"},
		{"no accents", "hello world", "hello world"},
		{"empty", "", ""},
		{"mixed", "café résumé naïve", "cafe resume naive"},
		{"already plain", "cafe", "cafe"},
		{"nordic", "Ångström", "Angstrom"},
		{"spanish", "señor", "senor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAccents(tt.input)
			if got != tt.want {
				t.Errorf("stripAccents(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeTextWithAccents(t *testing.T) {
	// Verify that normalizeText (which calls stripAccents) produces
	// the same tokens for accented and non-accented text.
	tests := []struct {
		a, b string
	}{
		{"café", "cafe"},
		{"résumé", "resume"},
		{"naïve approach", "naive approach"},
	}

	for _, tt := range tests {
		gotA := normalizeText(tt.a)
		gotB := normalizeText(tt.b)
		if gotA != gotB {
			t.Errorf("normalizeText(%q) = %q, normalizeText(%q) = %q — expected equal",
				tt.a, gotA, tt.b, gotB)
		}
	}
}

func TestTokenizeWithAccents(t *testing.T) {
	// Verify that the hash-based tokenizer produces the same tokens
	// for accented and non-accented text.
	tokensA := tokenize("café")
	tokensB := tokenize("cafe")

	if len(tokensA) != len(tokensB) {
		t.Fatalf("tokenize(\"café\") = %v, tokenize(\"cafe\") = %v — expected same length",
			tokensA, tokensB)
	}

	for i := range tokensA {
		if tokensA[i] != tokensB[i] {
			t.Errorf("tokenize(\"café\")[%d] = %q, tokenize(\"cafe\")[%d] = %q",
				i, tokensA[i], i, tokensB[i])
		}
	}
}
