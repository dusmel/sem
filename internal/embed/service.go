package embed

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"strings"
)

var tokenPattern = regexp.MustCompile(`[\pL\pN_]+`)

type Service struct {
	spec ModelSpec
}

func NewService(mode string) (*Service, error) {
	spec, err := Catalog(mode)
	if err != nil {
		return nil, err
	}

	return &Service{spec: spec}, nil
}

func (s *Service) Model() ModelSpec {
	return s.spec
}

func (s *Service) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vectors = append(vectors, s.embedText(s.spec.DocumentPrefix+text))
	}
	return vectors, nil
}

func (s *Service) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.embedText(s.spec.QueryPrefix + text), nil
}

func (s *Service) embedText(text string) []float32 {
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
