package source

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sem/internal/config"
	"sem/internal/errs"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9._ -]+$`)

var defaultExtensions = []string{"md", "txt", "go", "rs", "ts", "js", "jsx", "tsx", "py", "toml", "yaml", "yml", "json"}

func Add(cfg *config.Config, sourcePath, sourceName string) (config.SourceConfig, error) {
	absPath, err := filepath.Abs(filepath.Clean(sourcePath))
	if err != nil {
		return config.SourceConfig{}, fmt.Errorf("resolve source path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return config.SourceConfig{}, fmt.Errorf("stat source path: %w", err)
	}
	if !info.IsDir() {
		return config.SourceConfig{}, fmt.Errorf("source path must be a directory: %s", absPath)
	}

	if strings.TrimSpace(sourceName) == "" {
		sourceName = filepath.Base(absPath)
	}
	if !validName.MatchString(sourceName) {
		return config.SourceConfig{}, fmt.Errorf("invalid source name %q", sourceName)
	}

	for _, src := range cfg.Sources {
		if src.Name == sourceName {
			return config.SourceConfig{}, errs.ErrSourceExists
		}
	}

	added := config.SourceConfig{
		Name:              sourceName,
		Path:              absPath,
		Enabled:           true,
		IncludeExtensions: append([]string(nil), defaultExtensions...),
		ExcludePatterns:   []string{},
	}

	cfg.Sources = append(cfg.Sources, added)
	sort.Slice(cfg.Sources, func(i, j int) bool {
		return cfg.Sources[i].Name < cfg.Sources[j].Name
	})

	return added, nil
}

func Remove(cfg *config.Config, name string) error {
	for i, src := range cfg.Sources {
		if src.Name != name {
			continue
		}

		cfg.Sources = append(cfg.Sources[:i], cfg.Sources[i+1:]...)
		return nil
	}

	return errs.ErrSourceNotFound
}
