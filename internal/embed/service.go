package embed

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var tokenPattern = regexp.MustCompile(`[\pL\pN_]+`)

type Service struct {
	spec      ModelSpec
	backend   *onnxBackend // nil if ONNX not available
	modelDir  string
	usingONNX bool
}

// NewService creates a new embedding service for the given mode.
// It attempts to use ONNX Runtime if available, falling back to hash-based embeddings.
func NewService(mode string) (*Service, error) {
	spec, err := Catalog(mode)
	if err != nil {
		return nil, err
	}
	return &Service{spec: spec}, nil
}

// NewServiceWithModelDir creates a new embedding service with a specific model directory.
// It attempts to initialize ONNX Runtime, falling back to hash-based if unavailable.
func NewServiceWithModelDir(mode string, modelCacheDir string) (*Service, error) {
	spec, err := Catalog(mode)
	if err != nil {
		return nil, err
	}

	modelDir := filepath.Join(modelCacheDir, mode)
	svc := &Service{
		spec:     spec,
		modelDir: modelDir,
	}

	// Try to initialize ONNX backend
	if isOnnxRuntimeAvailable() {
		// Ensure model files are downloaded
		if err := EnsureModel(spec, modelDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not download ONNX model: %v\n", err)
			fmt.Fprintf(os.Stderr, "Falling back to hash-based embeddings.\n")
			return svc, nil
		}

		backend, err := newOnnxBackend(spec, modelDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: ONNX backend failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Falling back to hash-based embeddings.\n")
			return svc, nil
		}

		svc.backend = backend
		svc.usingONNX = true
	}

	return svc, nil
}

func (s *Service) Model() ModelSpec {
	return s.spec
}

func (s *Service) UsingONNX() bool {
	return s.usingONNX
}

func (s *Service) Close() error {
	if s.backend != nil {
		return s.backend.close()
	}
	return nil
}

func (s *Service) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	// Try ONNX backend first
	if s.backend != nil {
		// Apply document prefix
		prefixed := make([]string, len(texts))
		for i, text := range texts {
			prefixed[i] = s.spec.DocumentPrefix + text
		}

		// Process in batches
		batchSize := 32
		var results [][]float32
		for i := 0; i < len(prefixed); i += batchSize {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			end := i + batchSize
			if end > len(prefixed) {
				end = len(prefixed)
			}

			batch, err := s.backend.embedTexts(prefixed[i:end])
			if err != nil {
				// ONNX failed, fall back to hash-based for this batch
				hashBatch := make([][]float32, end-i)
				for j, text := range prefixed[i:end] {
					hashBatch[j] = s.hashEmbed(text)
				}
				results = append(results, hashBatch...)
				continue
			}
			results = append(results, batch...)
		}
		return results, nil
	}

	// Hash-based fallback
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vectors = append(vectors, s.hashEmbed(s.spec.DocumentPrefix+text))
	}
	return vectors, nil
}

func (s *Service) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefixed := s.spec.QueryPrefix + text

	if s.backend != nil {
		results, err := s.backend.embedTexts([]string{prefixed})
		if err != nil {
			// Fall back to hash-based
			return s.hashEmbed(prefixed), nil
		}
		if len(results) > 0 {
			return results[0], nil
		}
	}

	return s.hashEmbed(prefixed), nil
}

// hashEmbed generates a hash-based embedding vector (fallback when ONNX is not available).
func (s *Service) hashEmbed(text string) []float32 {
	vec := make([]float32, s.spec.Dimension)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return vec
	}

	for i, token := range tokens {
		applyHashedToken(vec, token, 1.0)
		if i > 0 {
			applyHashedToken(vec, tokens[i-1]+"_"+token, 0.5)
		}
	}

	if s.spec.NormalizeOutput {
		normalize(vec)
	}

	return vec
}

func tokenize(text string) []string {
	return tokenPattern.FindAllString(strings.ToLower(text), -1)
}

func applyHashedToken(vec []float32, token string, weight float32) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(token))
	sum := h.Sum64()
	idx := int(sum % uint64(len(vec)))
	sign := float32(-1)
	if (sum>>8)&1 == 1 {
		sign = 1
	}
	vec[idx] += weight * sign

	idx2 := int((sum >> 16) % uint64(len(vec)))
	vec[idx2] += weight * 0.5
}

func normalize(vec []float32) {
	var norm float64
	for _, value := range vec {
		norm += float64(value * value)
	}
	if norm == 0 {
		return
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= scale
	}
}

func EnsureMode(mode string) error {
	if _, err := Catalog(mode); err != nil {
		return fmt.Errorf("resolve embedding model: %w", err)
	}
	return nil
}
