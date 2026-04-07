package embed

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tokenizer implements a minimal WordPiece tokenizer that reads
// HuggingFace tokenizer.json format. It handles the common BERT-style
// tokenization: lowercase → split on whitespace/punctuation → greedy
// WordPiece subword matching with vocab lookup.
//
// This is intentionally "good enough" — it skips edge cases like
// Chinese character handling and accent stripping. Those can be added
// later if needed. The key guarantee is that all returned token IDs
// are valid vocab indices (never out-of-bounds).
type Tokenizer struct {
	vocab    map[string]int64 // token string → ID
	clsID    int64            // [CLS] token ID
	sepID    int64            // [SEP] token ID
	unkID    int64            // [UNK] token ID
	padID    int64            // [PAD] token ID
	prefix   string           // continuing subword prefix (e.g., "##")
	maxChars int              // max input chars per word
}

// tokenizerJSON represents the relevant fields from HuggingFace tokenizer.json.
type tokenizerJSON struct {
	Model       modelJSON        `json:"model"`
	Normalizer  *normalizerJSON  `json:"normalizer"`
	AddedTokens []addedTokenJSON `json:"added_tokens"`
}

type modelJSON struct {
	Type                    string           `json:"type"`
	UnkToken                string           `json:"unk_token"`
	ContinuingSubwordPrefix string           `json:"continuing_subword_prefix"`
	MaxInputCharsPerWord    int              `json:"max_input_chars_per_word"`
	Vocab                   map[string]int64 `json:"vocab"`
}

type normalizerJSON struct {
	Type         string `json:"type"`
	Lowercase    bool   `json:"lowercase"`
	CleanText    bool   `json:"clean_text"`
	StripAccents *bool  `json:"strip_accents"` // null means "follow lowercase"
}

type addedTokenJSON struct {
	ID      int64  `json:"id"`
	Content string `json:"content"`
	Special bool   `json:"special"`
}

// LoadTokenizer reads a HuggingFace tokenizer.json file and returns a Tokenizer.
func LoadTokenizer(path string) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tokenizer file: %w", err)
	}

	var tj tokenizerJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, fmt.Errorf("parse tokenizer JSON: %w", err)
	}

	if tj.Model.Type != "WordPiece" {
		return nil, fmt.Errorf("unsupported tokenizer type %q (only WordPiece is supported)", tj.Model.Type)
	}

	t := &Tokenizer{
		vocab:    tj.Model.Vocab,
		prefix:   tj.Model.ContinuingSubwordPrefix,
		maxChars: tj.Model.MaxInputCharsPerWord,
	}

	if t.maxChars == 0 {
		t.maxChars = 100
	}
	if t.prefix == "" {
		t.prefix = "##"
	}

	// Resolve special token IDs from vocab
	t.clsID = vocabID(t.vocab, "[CLS]", 101)
	t.sepID = vocabID(t.vocab, "[SEP]", 102)
	t.unkID = vocabID(t.vocab, "[UNK]", 100)
	t.padID = vocabID(t.vocab, "[PAD]", 0)

	return t, nil
}

func vocabID(vocab map[string]int64, token string, fallback int64) int64 {
	if id, ok := vocab[token]; ok {
		return id
	}
	return fallback
}

// Encode tokenizes text and returns token IDs with [CLS] and [SEP] added.
func (t *Tokenizer) Encode(text string) []int64 {
	// 1. Normalize: lowercase + basic cleanup
	text = normalizeText(text)

	// 2. Pre-tokenize: split on whitespace and punctuation
	words := preTokenize(text)

	// 3. WordPiece encode each word
	tokens := make([]int64, 0, len(words)+2)
	tokens = append(tokens, t.clsID)

	for _, word := range words {
		if len(word) > t.maxChars {
			tokens = append(tokens, t.unkID)
			continue
		}
		subwords := t.wordPiece(word)
		tokens = append(tokens, subwords...)
	}

	tokens = append(tokens, t.sepID)
	return tokens
}

// normalizeText does basic text normalization: lowercase and clean whitespace.
func normalizeText(text string) string {
	// Lowercase
	text = strings.ToLower(text)

	// Clean up whitespace: replace tabs/newlines with space, collapse runs
	var b strings.Builder
	b.Grow(len(text))
	prevSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// preTokenize splits text into words following BERT pre-tokenization rules:
// split on whitespace, then split each word on punctuation boundaries.
func preTokenize(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		} else if isPunctuation(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, string(r))
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

// isPunctuation returns true for ASCII punctuation and Unicode punctuation categories.
func isPunctuation(r rune) bool {
	// ASCII punctuation ranges (matching BERT's definition)
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) ||
		(r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r)
}

// wordPiece applies the WordPiece algorithm to a single word.
// It greedily matches the longest prefix in the vocabulary.
// Continuation subwords are prefixed with the continuing_subword_prefix (e.g., "##").
func (t *Tokenizer) wordPiece(word string) []int64 {
	if id, ok := t.vocab[word]; ok {
		return []int64{id}
	}

	var tokens []int64
	start := 0

	for start < len(word) {
		end := len(word)
		var found bool

		for end > start {
			subword := word[start:end]

			// Add continuation prefix for non-initial subwords
			if start > 0 {
				subword = t.prefix + subword
			}

			if id, ok := t.vocab[subword]; ok {
				tokens = append(tokens, id)
				found = true
				break
			}
			end -= utf8.RuneLen(rune(word[end-1]))
			if end <= start {
				break
			}
		}

		if !found {
			// No subword match — whole word is unknown
			return []int64{t.unkID}
		}
		start = end
	}

	return tokens
}
