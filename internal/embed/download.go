package embed

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// EnsureModel checks if the ONNX model files exist and downloads them if missing.
// modelDir is the directory for this mode (e.g., ~/.sem/models/balanced/)
func EnsureModel(spec ModelSpec, modelDir string) error {
	// Check if model.onnx exists (flat file in model dir)
	onnxPath := filepath.Join(modelDir, "model.onnx")
	modelExists := false
	if _, err := os.Stat(onnxPath); err == nil {
		modelExists = true
	}

	if !modelExists {
		// Create model directory
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			return fmt.Errorf("create model directory: %w", err)
		}

		// Download model files
		fmt.Printf("Downloading embedding model '%s' (%s)...\n", spec.Mode, spec.Name)
		fmt.Printf("This is a one-time download (~%dMB).\n", spec.ModelSizeMB)

		// Try variants in priority order
		if err := downloadBestVariant(spec, modelDir, onnxPath); err != nil {
			return fmt.Errorf("download ONNX model: %w", err)
		}
	}

	// Download tokenizer if not yet present
	tokenizerPath := filepath.Join(modelDir, spec.TokenizerFile)
	if _, err := os.Stat(tokenizerPath); err != nil {
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			return fmt.Errorf("create model directory: %w", err)
		}
		// Try to find a variant that has the tokenizer
		tokenizerDownloaded := false
		for _, variant := range spec.Variants {
			tokenizerURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", variant.Repo, spec.TokenizerFile)
			if err := downloadFile(tokenizerPath, tokenizerURL); err == nil {
				tokenizerDownloaded = true
				break
			}
		}
		if !tokenizerDownloaded {
			fmt.Printf("Warning: could not download tokenizer\n")
			fmt.Printf("Using simple tokenization fallback.\n")
		}
	}

	if !modelExists {
		fmt.Printf("Model '%s' downloaded successfully.\n", spec.Name)
	}
	return nil
}

// downloadBestVariant tries to download model variants in priority order.
// It saves the downloaded file as "model.onnx" in the model directory.
func downloadBestVariant(spec ModelSpec, modelDir, destPath string) error {
	// Sort variants by priority (highest first)
	variants := make([]ModelVariant, len(spec.Variants))
	copy(variants, spec.Variants)
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].Priority > variants[j].Priority
	})

	var lastErr error
	for _, variant := range variants {
		url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", variant.Repo, variant.File)
		fmt.Printf("  Trying: %s/%s\n", variant.Repo, variant.File)

		if err := downloadFile(destPath, url); err != nil {
			fmt.Printf("  Failed: %v\n", err)
			lastErr = err
			continue
		}

		fmt.Printf("  Downloaded: %s/%s\n", variant.Repo, variant.File)
		return nil
	}

	return fmt.Errorf("all variants failed (last error: %w)", lastErr)
}

func downloadFile(destPath, url string) error {
	// Download to temp file first, then rename (atomic)
	tmpPath := destPath + ".tmp"

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
