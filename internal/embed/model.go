package embed

import "fmt"

type ModelSpec struct {
	Mode              string `json:"mode"`
	Name              string `json:"name"`
	HuggingFaceID     string `json:"huggingface_id"`
	Dimension         int    `json:"dimension"`
	MaxTokens         int    `json:"max_tokens"`
	QueryPrefix       string `json:"query_prefix,omitempty"`
	DocumentPrefix    string `json:"document_prefix,omitempty"`
	NormalizeOutput   bool   `json:"normalize_output"`
	ONNXFile          string `json:"onnx_file,omitempty"`           // filename: "model.onnx"
	ONNXQuantizedFile string `json:"onnx_quantized_file,omitempty"` // e.g., "onnx/model_qint8_arm64.onnx"
	TokenizerFile     string `json:"tokenizer_file,omitempty"`      // filename: "tokenizer.json"
	ModelSizeMB       int    `json:"model_size_mb,omitempty"`       // approximate download size
}

var catalog = map[string]ModelSpec{
	"light": {
		Mode:              "light",
		Name:              "all-MiniLM-L6-v2",
		HuggingFaceID:     "sentence-transformers/all-MiniLM-L6-v2",
		Dimension:         384,
		MaxTokens:         256,
		NormalizeOutput:   true,
		ONNXFile:          "onnx/model.onnx",
		ONNXQuantizedFile: "onnx/model_qint8_arm64.onnx",
		TokenizerFile:     "tokenizer.json",
		ModelSizeMB:       90,
	},
	"balanced": {
		Mode:            "balanced",
		Name:            "bge-small-en-v1.5",
		HuggingFaceID:   "BAAI/bge-small-en-v1.5",
		Dimension:       384,
		MaxTokens:       512,
		QueryPrefix:     "Represent this sentence for searching relevant passages: ",
		NormalizeOutput: true,
		ONNXFile:        "onnx/model.onnx",
		TokenizerFile:   "tokenizer.json",
		ModelSizeMB:     130,
	},
	"quality": {
		Mode:            "quality",
		Name:            "bge-base-en-v1.5",
		HuggingFaceID:   "BAAI/bge-base-en-v1.5",
		Dimension:       768,
		MaxTokens:       512,
		QueryPrefix:     "Represent this sentence for searching relevant passages: ",
		NormalizeOutput: true,
		ONNXFile:        "onnx/model.onnx",
		TokenizerFile:   "tokenizer.json",
		ModelSizeMB:     430,
	},
	"nomic": {
		Mode:            "nomic",
		Name:            "nomic-embed-text-v1",
		HuggingFaceID:   "nomic-ai/nomic-embed-text-v1",
		Dimension:       768,
		MaxTokens:       512,
		NormalizeOutput: true,
		ONNXFile:        "onnx/model.onnx",
		TokenizerFile:   "tokenizer.json",
		ModelSizeMB:     550,
	},
}

func Catalog(mode string) (ModelSpec, error) {
	spec, ok := catalog[mode]
	if !ok {
		return ModelSpec{}, fmt.Errorf("unknown embedding mode %q", mode)
	}
	return spec, nil
}
