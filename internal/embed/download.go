package embed

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

		// Download ONNX model
		onnxURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", spec.HuggingFaceID, spec.ONNXFile)
		if err := downloadFile(onnxPath, onnxURL); err != nil {
			return fmt.Errorf("download ONNX model: %w", err)
		}
	}

	// Download quantized model variant if available and not yet downloaded
	if spec.ONNXQuantizedFile != "" {
		quantizedPath := filepath.Join(modelDir, "model_quantized.onnx")
		if _, err := os.Stat(quantizedPath); err != nil {
			if err := os.MkdirAll(modelDir, 0755); err != nil {
				return fmt.Errorf("create model directory: %w", err)
			}
			if !modelExists {
				fmt.Printf("Downloading quantized model for faster inference...\n")
			}
			quantizedURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", spec.HuggingFaceID, spec.ONNXQuantizedFile)
			if err := downloadFile(quantizedPath, quantizedURL); err != nil {
				// Quantized model download failure is not fatal
				fmt.Printf("Note: quantized model not available, using standard model\n")
			}
		}
	}

	// Download tokenizer if not yet present
	tokenizerPath := filepath.Join(modelDir, spec.TokenizerFile)
	if _, err := os.Stat(tokenizerPath); err != nil {
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			return fmt.Errorf("create model directory: %w", err)
		}
		tokenizerURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/tokenizer.json", spec.HuggingFaceID)
		if err := downloadFile(tokenizerPath, tokenizerURL); err != nil {
			fmt.Printf("Warning: could not download tokenizer: %v\n", err)
			fmt.Printf("Using simple tokenization fallback.\n")
		}
	}

	if !modelExists {
		fmt.Printf("Model '%s' downloaded successfully.\n", spec.Name)
	}
	return nil
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
