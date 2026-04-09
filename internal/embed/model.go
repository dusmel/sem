package embed

import (
	"fmt"
	"runtime"
)

// ModelVariant represents a downloadable variant of an embedding model.
// Variants are ordered by preference (higher priority = preferred).
type ModelVariant struct {
	Repo     string // HuggingFace repo (e.g., "sentence-transformers/all-MiniLM-L6-v2")
	File     string // Path within the repo (e.g., "onnx/model_O4.onnx")
	Priority int    // Higher = preferred
}

// ModelSpec defines an embedding model with its downloadable variants.
type ModelSpec struct {
	Mode            string         `json:"mode"`
	Name            string         `json:"name"`
	Dimension       int            `json:"dimension"`
	MaxTokens       int            `json:"max_tokens"`
	QueryPrefix     string         `json:"query_prefix,omitempty"`
	DocumentPrefix  string         `json:"document_prefix,omitempty"`
	NormalizeOutput bool           `json:"normalize_output"`
	Variants        []ModelVariant `json:"variants"`                 // Ordered by preference
	TokenizerFile   string         `json:"tokenizer_file,omitempty"` // filename: "tokenizer.json"
	ModelSizeMB     int            `json:"model_size_mb,omitempty"`  // approximate download size
}

// modelMaxTokens defines the default max tokens per mode.
// These are model-specific limits that reduce padding waste.
var modelMaxTokens = map[string]int{
	"light":    256,  // MiniLM-L6-v2
	"balanced": 512,  // BGE Small v1.5
	"quality":  512,  // BGE Base v1.5
	"nomic":    2048, // Nomic Embed v1
}

// catalog defines all embedding modes with their variant priorities.
// Variants are ordered by preference — the download logic tries them in order.
var catalog = map[string]ModelSpec{
	"light": {
		Mode:            "light",
		Name:            "all-MiniLM-L6-v2",
		Dimension:       384,
		MaxTokens:       256,
		NormalizeOutput: true,
		Variants:        miniLMVariants(),
		TokenizerFile:   "tokenizer.json",
		ModelSizeMB:     23,
	},
	"balanced": {
		Mode:            "balanced",
		Name:            "bge-small-en-v1.5",
		Dimension:       384,
		MaxTokens:       512,
		QueryPrefix:     "Represent this sentence for searching relevant passages: ",
		NormalizeOutput: true,
		Variants:        bgeSmallVariants(),
		TokenizerFile:   "tokenizer.json",
		ModelSizeMB:     33,
	},
	"quality": {
		Mode:            "quality",
		Name:            "bge-base-en-v1.5",
		Dimension:       768,
		MaxTokens:       512,
		QueryPrefix:     "Represent this sentence for searching relevant passages: ",
		NormalizeOutput: true,
		Variants:        bgeBaseVariants(),
		TokenizerFile:   "tokenizer.json",
		ModelSizeMB:     218,
	},
	"nomic": {
		Mode:            "nomic",
		Name:            "nomic-embed-text-v1",
		Dimension:       768,
		MaxTokens:       2048,
		NormalizeOutput: true,
		Variants: []ModelVariant{
			{
				Repo:     "nomic-ai/nomic-embed-text-v1",
				File:     "onnx/model.onnx",
				Priority: 10,
			},
		},
		TokenizerFile: "tokenizer.json",
		ModelSizeMB:   550,
	},
}

// miniLMVariants returns the variant list for MiniLM (light mode).
// On ARM64, INT8 quantized beats graph-optimized, so it's preferred.
func miniLMVariants() []ModelVariant {
	if runtime.GOARCH == "arm64" {
		return []ModelVariant{
			{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model_qint8_arm64.onnx", Priority: 100},
			{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model_O4.onnx", Priority: 90},
			{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model_O3.onnx", Priority: 80},
			{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model_O2.onnx", Priority: 70},
			{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model.onnx", Priority: 10},
		}
	}
	return []ModelVariant{
		{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model_O4.onnx", Priority: 90},
		{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model_O3.onnx", Priority: 80},
		{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model_O2.onnx", Priority: 70},
		{Repo: "sentence-transformers/all-MiniLM-L6-v2", File: "onnx/model.onnx", Priority: 10},
	}
}

// bgeSmallVariants returns the variant list for BGE Small (balanced mode).
// Qdrant's INT8 quantized variant is preferred (5-9x speedup).
func bgeSmallVariants() []ModelVariant {
	return []ModelVariant{
		{Repo: "Qdrant/bge-small-en-v1.5-onnx-Q", File: "model_qint8.onnx", Priority: 100},
		{Repo: "BAAI/bge-small-en-v1.5", File: "onnx/model.onnx", Priority: 10},
	}
}

// bgeBaseVariants returns the variant list for BGE Base (quality mode).
// Qdrant's graph-optimized (FP16) variant is preferred (~1.5-2x speedup).
func bgeBaseVariants() []ModelVariant {
	return []ModelVariant{
		{Repo: "Qdrant/bge-base-en-v1.5-onnx-Q", File: "model_optimized.onnx", Priority: 100},
		{Repo: "BAAI/bge-base-en-v1.5", File: "onnx/model.onnx", Priority: 10},
	}
}

func Catalog(mode string) (ModelSpec, error) {
	spec, ok := catalog[mode]
	if !ok {
		return ModelSpec{}, fmt.Errorf("unknown embedding mode %q", mode)
	}
	return spec, nil
}

// MaxTokensForMode returns the default max tokens for a given mode.
// This is the model-specific limit that reduces padding waste.
func MaxTokensForMode(mode string) int {
	if tokens, ok := modelMaxTokens[mode]; ok {
		return tokens
	}
	return 512 // safe default
}
