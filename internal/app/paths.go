package app

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	HomeDir     string
	BaseDir     string
	ConfigPath  string
	BundlesDir  string
	BackendsDir string
	LanceDBDir  string
	ModelsDir   string
}

func Resolve() (Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".sem")

	return Paths{
		HomeDir:     homeDir,
		BaseDir:     baseDir,
		ConfigPath:  filepath.Join(baseDir, "config.toml"),
		BundlesDir:  filepath.Join(baseDir, "bundles"),
		BackendsDir: filepath.Join(baseDir, "backends"),
		LanceDBDir:  filepath.Join(baseDir, "backends", "lancedb"),
		ModelsDir:   filepath.Join(baseDir, "models"),
	}, nil
}

func (p Paths) BundleDir(name string) string {
	return filepath.Join(p.BundlesDir, name)
}

func (p Paths) EnsureLayout(bundle string) error {
	dirs := []string{
		p.BaseDir,
		p.BundlesDir,
		p.BundleDir(bundle),
		p.BackendsDir,
		p.LanceDBDir,
		p.ModelsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return nil
}
