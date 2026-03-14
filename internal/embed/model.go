package embed

import "fmt"

type ModelSpec struct {
	Mode            string `json:"mode"`
	Name            string `json:"name"`
	HuggingFaceID   string `json:"huggingface_id"`
	Dimension       int    `json:"dimension"`
	MaxTokens       int    `json:"max_tokens"`
	QueryPrefix     string `json:"query_prefix,omitempty"`
	DocumentPrefix  string `json:"document_prefix,omitempty"`
	NormalizeOutput bool   `json:"normalize_output"`
}

var catalog = map[string]ModelSpec{
	"light": {
		Mode:            "light",
		Name:            "all-MiniLM-L6-v2",
		HuggingFaceID:   "sentence-transformers/all-MiniLM-L6-v2",
		Dimension:       384,
		MaxTokens:       256,
		NormalizeOutput: true,
	},
	"balanced": {
		Mode:            "balanced",
		Name:            "bge-small-en-v1.5",
		HuggingFaceID:   "BAAI/bge-small-en-v1.5",
		Dimension:       384,
		MaxTokens:       512,
		QueryPrefix:     "Represent this sentence for searching relevant passages: ",
		NormalizeOutput: true,
	},
	"quality": {
		Mode:            "quality",
		Name:            "bge-base-en-v1.5",
		HuggingFaceID:   "BAAI/bge-base-en-v1.5",
		Dimension:       768,
		MaxTokens:       512,
		QueryPrefix:     "Represent this sentence for searching relevant passages: ",
		NormalizeOutput: true,
	},
	"nomic": {
		Mode:            "nomic",
		Name:            "nomic-embed-text-v1",
		HuggingFaceID:   "nomic-ai/nomic-embed-text-v1",
		Dimension:       768,
		MaxTokens:       512,
		NormalizeOutput: true,
	},
}

func Catalog(mode string) (ModelSpec, error) {
	spec, ok := catalog[mode]
	if !ok {
		return ModelSpec{}, fmt.Errorf("unknown embedding mode %q", mode)
	}
	return spec, nil
}
