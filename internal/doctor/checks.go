package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sem/internal/config"
	"sem/internal/embed"
	"sem/internal/storage"
)

// Status represents the result of a health check.
type Status int

const (
	Pass Status = iota
	Warn
	Fail
)

func (s Status) Symbol() string {
	switch s {
	case Pass:
		return "✓"
	case Warn:
		return "⚠"
	case Fail:
		return "✗"
	default:
		return "?"
	}
}

// Check represents a single health check result.
type Check struct {
	Name    string
	Status  Status
	Message string
	Hint    string // Optional hint for fixing issues
}

// RunAll executes all health checks and returns results.
func RunAll(cfg config.Config, configPath string, modelsDir string, bundleDir string) []Check {
	var checks []Check
	checks = append(checks, checkRipgrep())
	checks = append(checks, checkONNXRuntime(modelsDir))
	checks = append(checks, checkModels(modelsDir)...)
	checks = append(checks, checkConfig(configPath))
	checks = append(checks, checkSources(cfg.Sources))
	checks = append(checks, checkBundle(bundleDir))
	return checks
}

// checkRipgrep verifies that ripgrep (rg) is installed.
func checkRipgrep() Check {
	path, err := exec.LookPath("rg")
	if err != nil {
		return Check{
			Name:    "ripgrep",
			Status:  Fail,
			Message: "ripgrep not found",
			Hint:    "Install with: brew install ripgrep",
		}
	}

	// Get version
	version := "unknown"
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err == nil {
		// Output: "ripgrep 14.1.0 ..."
		parts := strings.Fields(string(out))
		if len(parts) >= 2 {
			version = parts[1]
		}
	}

	return Check{
		Name:    "ripgrep",
		Status:  Pass,
		Message: fmt.Sprintf("ripgrep %s installed at %s", version, path),
	}
}

// checkONNXRuntime verifies that the ONNX Runtime shared library is loadable.
func checkONNXRuntime(modelsDir string) Check {
	// Try to initialize a minimal embedding service to test ONNX
	svc, err := embed.NewServiceWithModelDir("light", modelsDir)
	if err != nil {
		return Check{
			Name:    "ONNX Runtime",
			Status:  Fail,
			Message: fmt.Sprintf("failed to initialize: %v", err),
			Hint:    "Ensure ONNX Runtime is properly installed",
		}
	}
	defer svc.Close()

	if svc.UsingONNX() {
		return Check{
			Name:    "ONNX Runtime",
			Status:  Pass,
			Message: "ONNX Runtime loaded successfully",
		}
	}

	return Check{
		Name:    "ONNX Runtime",
		Status:  Warn,
		Message: "ONNX Runtime not available (using hash-based fallback)",
		Hint:    "Install ONNX Runtime shared library for real embeddings",
	}
}

// checkModels verifies that model files exist for each configured embedding mode.
func checkModels(modelsDir string) []Check {
	modes := []string{"light", "balanced", "quality"}
	var checks []Check

	for _, mode := range modes {
		spec, err := embed.Catalog(mode)
		if err != nil {
			checks = append(checks, Check{
				Name:    fmt.Sprintf("model '%s'", mode),
				Status:  Fail,
				Message: fmt.Sprintf("unknown mode: %s", mode),
			})
			continue
		}

		modelDir := filepath.Join(modelsDir, mode)
		info, err := os.Stat(modelDir)
		if err != nil || !info.IsDir() {
			checks = append(checks, Check{
				Name:    fmt.Sprintf("model '%s'", mode),
				Status:  Fail,
				Message: fmt.Sprintf("model '%s' not cached", mode),
				Hint:    fmt.Sprintf("Run: sem index (will download on first use)"),
			})
			continue
		}

		// Calculate directory size
		size := dirSize(modelDir)
		sizeStr := formatSize(size)

		checks = append(checks, Check{
			Name:    fmt.Sprintf("model '%s'", mode),
			Status:  Pass,
			Message: fmt.Sprintf("model '%s' cached (~%s) — %s", mode, sizeStr, spec.Name),
		})
	}

	return checks
}

// checkConfig verifies that the config file exists and is valid.
func checkConfig(configPath string) Check {
	if _, err := os.Stat(configPath); err != nil {
		return Check{
			Name:    "config",
			Status:  Fail,
			Message: fmt.Sprintf("config not found at %s", configPath),
			Hint:    "Run: sem init",
		}
	}

	_, err := config.Load(configPath)
	if err != nil {
		return Check{
			Name:    "config",
			Status:  Fail,
			Message: fmt.Sprintf("config invalid at %s: %v", configPath, err),
			Hint:    "Run: sem init to regenerate",
		}
	}

	return Check{
		Name:    "config",
		Status:  Pass,
		Message: fmt.Sprintf("config valid at %s", configPath),
	}
}

// checkSources verifies that configured source paths are accessible.
func checkSources(sources []config.SourceConfig) Check {
	if len(sources) == 0 {
		return Check{
			Name:    "sources",
			Status:  Warn,
			Message: "no sources configured",
			Hint:    "Run: sem source add <path> --name <name>",
		}
	}

	enabled := 0
	inaccessible := 0
	for _, src := range sources {
		if src.Enabled {
			enabled++
			if _, err := os.Stat(src.Path); err != nil {
				inaccessible++
			}
		}
	}

	if enabled == 0 {
		return Check{
			Name:    "sources",
			Status:  Warn,
			Message: fmt.Sprintf("%d source(s) configured, 0 enabled", len(sources)),
			Hint:    "Check: sem source list",
		}
	}

	msg := fmt.Sprintf("%d source(s) configured, %d enabled", len(sources), enabled)
	if inaccessible > 0 {
		msg += fmt.Sprintf(", %d inaccessible", inaccessible)
		return Check{
			Name:    "sources",
			Status:  Warn,
			Message: msg,
			Hint:    "Verify source paths exist and are readable",
		}
	}

	return Check{
		Name:    "sources",
		Status:  Pass,
		Message: msg,
	}
}

// checkBundle verifies that the bundle directory exists and has data.
func checkBundle(bundleDir string) Check {
	bundle := storage.NewBundle(bundleDir)
	manifest, err := bundle.LoadManifest()
	if err != nil {
		return Check{
			Name:    "bundle",
			Status:  Fail,
			Message: "bundle not found",
			Hint:    "Run: sem index",
		}
	}

	// Calculate bundle directory size
	size := dirSize(bundleDir)
	sizeStr := formatSize(size)

	msg := fmt.Sprintf("bundle exists — %d chunks, %s", manifest.ChunkCount, sizeStr)
	return Check{
		Name:    "bundle",
		Status:  Pass,
		Message: msg,
	}
}

// dirSize calculates the total size of files in a directory recursively.
func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// formatSize returns a human-readable size string.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.0fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.0fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
