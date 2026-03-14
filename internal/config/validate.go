package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"sem/internal/errs"
)

var sourceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (c *Config) Validate() error {
	if strings.TrimSpace(c.General.DefaultBundle) == "" {
		return errs.ValidationError{Field: "general.default_bundle", Message: "must not be empty"}
	}

	if !isValidEmbeddingMode(c.Embedding.Mode) {
		return errs.ValidationError{Field: "embedding.mode", Message: "must be one of light, balanced, quality, nomic"}
	}

	if c.General.EmbeddingMode == "" {
		c.General.EmbeddingMode = c.Embedding.Mode
	}

	if c.Storage.Backend != "lancedb" {
		return errs.ValidationError{Field: "storage.backend", Message: "Stage 1 supports only lancedb"}
	}

	if !filepath.IsAbs(c.Embedding.ModelCacheDir) {
		return errs.ValidationError{Field: "embedding.model_cache_dir", Message: "must be an absolute path"}
	}

	if !filepath.IsAbs(c.Storage.BundleDir) {
		return errs.ValidationError{Field: "storage.bundle_dir", Message: "must be an absolute path"}
	}

	if !filepath.IsAbs(c.Storage.LanceDB.Path) {
		return errs.ValidationError{Field: "storage.lancedb.path", Message: "must be an absolute path"}
	}

	if c.Chunking.MinChars > c.Chunking.MaxChars {
		return errs.ValidationError{Field: "chunking.min_chars", Message: "must be less than or equal to max_chars"}
	}

	if c.Chunking.OverlapChars >= c.Chunking.MaxChars {
		return errs.ValidationError{Field: "chunking.overlap_chars", Message: "must be less than max_chars"}
	}

	seen := map[string]struct{}{}
	for _, src := range c.Sources {
		if strings.TrimSpace(src.Name) == "" {
			return errs.ValidationError{Field: "sources.name", Message: "must not be empty"}
		}
		if !sourceNamePattern.MatchString(src.Name) {
			return errs.ValidationError{Field: fmt.Sprintf("sources.%s.name", src.Name), Message: "must contain only letters, numbers, dot, underscore, or dash"}
		}
		if _, exists := seen[src.Name]; exists {
			return errs.ValidationError{Field: fmt.Sprintf("sources.%s.name", src.Name), Message: "must be unique"}
		}
		seen[src.Name] = struct{}{}
		if !filepath.IsAbs(src.Path) {
			return errs.ValidationError{Field: fmt.Sprintf("sources.%s.path", src.Name), Message: "must be an absolute path"}
		}
	}

	return nil
}

func isValidEmbeddingMode(mode string) bool {
	switch mode {
	case "light", "balanced", "quality", "nomic":
		return true
	default:
		return false
	}
}
