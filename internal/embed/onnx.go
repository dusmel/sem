package embed

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

// onnxBackend wraps the ONNX Runtime session for embedding inference.
type onnxBackend struct {
	session      *ort.DynamicAdvancedSession
	tokenizer    *Tokenizer // real WordPiece tokenizer (nil if unavailable)
	spec         ModelSpec
	modelDir     string
	libPath      string
	envReady     bool
	needsTypeIDs bool // true if model requires token_type_ids input
}

// findOnnxRuntimeLibrary finds the ONNX Runtime shared library.
// Returns empty string if not found.
func findOnnxRuntimeLibrary() string {
	// Check env var first
	if path := os.Getenv("ONNXRUNTIME_SO_PATH"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Check common locations
	commonPaths := []string{
		"/opt/homebrew/lib/libonnxruntime.dylib",       // macOS ARM (Homebrew)
		"/usr/local/lib/libonnxruntime.dylib",          // macOS Intel (Homebrew)
		"/usr/local/lib/libonnxruntime.so",             // Linux
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",  // Ubuntu
		"/usr/lib/aarch64-linux-gnu/libonnxruntime.so", // Ubuntu ARM
	}
	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// isOnnxRuntimeAvailable checks if the ONNX Runtime shared library can be found.
func isOnnxRuntimeAvailable() bool {
	return findOnnxRuntimeLibrary() != ""
}

// isARM64 returns true if running on ARM64 architecture.
func isARM64() bool {
	return runtime.GOARCH == "arm64"
}

// newOnnxBackend creates a new ONNX backend for the given model spec.
func newOnnxBackend(spec ModelSpec, modelDir string) (*onnxBackend, error) {
	libPath := findOnnxRuntimeLibrary()
	if libPath == "" {
		return nil, fmt.Errorf("ONNX Runtime shared library not found. Install with: brew install onnxruntime (macOS) or apt install libonnxruntime-dev (Linux)")
	}

	// Determine which model file to use
	// Prefer quantized model on ARM64 if available
	onnxPath := filepath.Join(modelDir, "model.onnx")
	quantizedPath := filepath.Join(modelDir, "model_quantized.onnx")

	if isARM64() {
		if _, err := os.Stat(quantizedPath); err == nil {
			onnxPath = quantizedPath
		}
	}

	if _, err := os.Stat(onnxPath); err != nil {
		return nil, fmt.Errorf("ONNX model not found at %s: %w", onnxPath, err)
	}

	// Set the shared library path and initialize ONNX Runtime
	ort.SetSharedLibraryPath(libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("initialize ONNX Runtime: %w", err)
	}

	backend := &onnxBackend{
		spec:     spec,
		modelDir: modelDir,
		libPath:  libPath,
		envReady: true,
	}

	// Load tokenizer (non-fatal if unavailable — falls back to simpleTokenize)
	tokenizerPath := filepath.Join(modelDir, "tokenizer.json")
	if tok, err := LoadTokenizer(tokenizerPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load tokenizer from %s: %v\n", tokenizerPath, err)
		fmt.Fprintf(os.Stderr, "Using simple hash-based tokenization fallback.\n")
	} else {
		backend.tokenizer = tok
	}

	// Create the session — try with token_type_ids first (BERT-based models),
	// fall back to 2-input if the model doesn't need it.
	if err := backend.createSessionWithTokenTypes(onnxPath); err != nil {
		if err2 := backend.createSessionBasic(onnxPath); err2 != nil {
			ort.DestroyEnvironment()
			return nil, fmt.Errorf("create ONNX session (tried with and without token_type_ids): 3-input: %w, 2-input: %v", err, err2)
		}
	}

	return backend, nil
}

func (b *onnxBackend) createSessionWithTokenTypes(modelPath string) error {
	opts, err := b.sessionOptions()
	if err != nil {
		return err
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		opts)
	opts.Destroy()
	if err != nil {
		return fmt.Errorf("create 3-input ONNX session: %w", err)
	}
	b.session = session
	b.needsTypeIDs = true
	return nil
}

func (b *onnxBackend) createSessionBasic(modelPath string) error {
	opts, err := b.sessionOptions()
	if err != nil {
		return err
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"last_hidden_state"},
		opts)
	opts.Destroy()
	if err != nil {
		return fmt.Errorf("create 2-input ONNX session: %w", err)
	}
	b.session = session
	b.needsTypeIDs = false
	return nil
}

// sessionOptions creates ONNX session options tuned for CPU inference.
func (b *onnxBackend) sessionOptions() (*ort.SessionOptions, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("create session options: %w", err)
	}
	// Use all available CPU cores for intra-op parallelism
	if err := opts.SetIntraOpNumThreads(runtime.NumCPU()); err != nil {
		// Non-fatal
		fmt.Fprintf(os.Stderr, "Warning: could not set intra-op threads: %v\n", err)
	}
	// Enable all graph optimizations
	if err := opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll); err != nil {
		// Non-fatal
		fmt.Fprintf(os.Stderr, "Warning: could not set graph optimization level: %v\n", err)
	}
	return opts, nil
}

// close releases the ONNX session and environment resources.
func (b *onnxBackend) close() error {
	if b.session != nil {
		b.session.Destroy()
		b.session = nil
	}
	if b.envReady {
		ort.DestroyEnvironment()
		b.envReady = false
	}
	return nil
}

// embedTexts runs ONNX inference on a batch of texts using batched processing.
// Texts are tokenized, padded to the same length within each batch, and
// processed together in a single ONNX Run() call for efficiency.
func (b *onnxBackend) embedTexts(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Tokenize all texts upfront
	maxTokens := b.spec.MaxTokens
	allTokens := make([][]int64, len(texts))
	for i, text := range texts {
		var tokens []int64
		if b.tokenizer != nil {
			tokens = b.tokenizer.Encode(text)
		} else {
			tokens = simpleTokenize(text)
		}
		if len(tokens) > maxTokens {
			tokens = tokens[:maxTokens]
		}
		allTokens[i] = tokens
	}

	// Process in batches of up to 8
	const batchSize = 8
	var results [][]float32

	for i := 0; i < len(allTokens); i += batchSize {
		end := i + batchSize
		if end > len(allTokens) {
			end = len(allTokens)
		}
		batchTokens := allTokens[i:end]

		batchResults, err := b.embedBatch(batchTokens)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", i, end, err)
		}
		results = append(results, batchResults...)
	}

	return results, nil
}

// embedBatch runs ONNX inference on a batch of pre-tokenized texts.
// All texts are padded to the length of the longest text in the batch,
// then processed in a single Run() call.
func (b *onnxBackend) embedBatch(tokenLists [][]int64) ([][]float32, error) {
	batchSize := len(tokenLists)
	dim := b.spec.Dimension

	// Find max sequence length in this batch
	maxSeqLen := 0
	for _, tokens := range tokenLists {
		if len(tokens) > maxSeqLen {
			maxSeqLen = len(tokens)
		}
	}
	if maxSeqLen == 0 {
		// All empty — return zero vectors
		results := make([][]float32, batchSize)
		for i := range results {
			results[i] = make([]float32, dim)
		}
		return results, nil
	}

	// Build padded input arrays: [batchSize, maxSeqLen]
	inputIDs := make([]int64, batchSize*maxSeqLen)
	maskData := make([]int64, batchSize*maxSeqLen)

	for i, tokens := range tokenLists {
		offset := i * maxSeqLen
		for j, tok := range tokens {
			inputIDs[offset+j] = tok
			maskData[offset+j] = 1
		}
		// Remaining positions are already 0 (padding)
	}

	// Create tensors
	batchShape := ort.NewShape(int64(batchSize), int64(maxSeqLen))

	inputTensor, err := ort.NewTensor(batchShape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("create input tensor: %w", err)
	}

	maskTensor, err := ort.NewTensor(batchShape, maskData)
	if err != nil {
		inputTensor.Destroy()
		return nil, fmt.Errorf("create mask tensor: %w", err)
	}

	var inputs []ort.Value
	if b.needsTypeIDs {
		// token_type_ids: all zeros
		typeData := make([]int64, batchSize*maxSeqLen)
		typeTensor, err := ort.NewTensor(batchShape, typeData)
		if err != nil {
			inputTensor.Destroy()
			maskTensor.Destroy()
			return nil, fmt.Errorf("create type tensor: %w", err)
		}
		inputs = []ort.Value{inputTensor, maskTensor, typeTensor}
	} else {
		inputs = []ort.Value{inputTensor, maskTensor}
	}

	// Output is nil — auto-allocated by ONNX Runtime
	outputs := []ort.Value{nil}

	// Run inference
	if err := b.session.Run(inputs, outputs); err != nil {
		for _, inp := range inputs {
			inp.Destroy()
		}
		return nil, fmt.Errorf("run ONNX inference: %w", err)
	}

	// Clean up input tensors
	for _, inp := range inputs {
		inp.Destroy()
	}

	// Extract output
	outputTensor := outputs[0].(*ort.Tensor[float32])
	outputData := outputTensor.GetData()
	// Output shape: [batchSize, maxSeqLen, dim]

	// Mean pooling for each item in the batch
	results := make([][]float32, batchSize)
	for i := range results {
		vec := make([]float32, dim)
		var tokenCount float32
		for t := 0; t < maxSeqLen; t++ {
			if maskData[i*maxSeqLen+t] == 0 {
				continue
			}
			tokenCount++
			outOffset := (i*maxSeqLen + t) * dim
			for d := 0; d < dim; d++ {
				vec[d] += outputData[outOffset+d]
			}
		}

		if tokenCount > 0 {
			for d := 0; d < dim; d++ {
				vec[d] /= tokenCount
			}
		}

		if b.spec.NormalizeOutput {
			normalizeVector(vec)
		}
		results[i] = vec
	}

	// Clean up output tensor
	outputTensor.Destroy()

	return results, nil
}

// simpleTokenize converts text to a list of token IDs using word-level hashing.
// This is a placeholder tokenizer — a real BPE tokenizer will be added later.
func simpleTokenize(text string) []int64 {
	words := strings.Fields(strings.ToLower(text))
	tokens := make([]int64, 0, len(words))
	for _, word := range words {
		h := fnvHash(word)
		tokens = append(tokens, int64(h%30000)+1000)
	}
	return tokens
}

func fnvHash(s string) uint64 {
	h := uint64(2166136261)
	for _, c := range s {
		h ^= uint64(c)
		h *= 16777619
	}
	return h
}

// normalizeVector performs L2 normalization in-place.
func normalizeVector(vec []float32) {
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm == 0 {
		return
	}
	s := float32(1.0 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= s
	}
}
