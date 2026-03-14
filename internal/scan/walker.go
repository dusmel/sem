package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"sem/internal/config"
)

var errBinaryFile = errors.New("binary file")

type FileDocument struct {
	SourceName string
	SourcePath string
	AbsPath    string
	RelPath    string
	Extension  string
	Content    string
	ByteSize   int64
	ModifiedAt time.Time
}

type matcher struct {
	includeExt      map[string]struct{}
	defaultPatterns []string
	sourcePatterns  []string
}

func ScanSource(ctx context.Context, src config.SourceConfig, defaults []string) ([]FileDocument, error) {
	m := newMatcher(src.IncludeExtensions, defaults, src.ExcludePatterns)
	files := make([]FileDocument, 0, 128)

	err := filepath.WalkDir(src.Path, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(src.Path, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		if m.skip(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		ext := normalizeExt(filepath.Ext(path))
		if _, ok := m.includeExt[ext]; !ok {
			return nil
		}

		content, err := readTextFile(path)
		if errors.Is(err, errBinaryFile) {
			return nil
		}
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		files = append(files, FileDocument{
			SourceName: src.Name,
			SourcePath: src.Path,
			AbsPath:    path,
			RelPath:    filepath.ToSlash(rel),
			Extension:  ext,
			Content:    content,
			ByteSize:   info.Size(),
			ModifiedAt: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan source %s: %w", src.Name, err)
	}

	return files, nil
}

func newMatcher(includeExt, defaults, sourcePatterns []string) matcher {
	set := make(map[string]struct{}, len(includeExt))
	for _, ext := range includeExt {
		set[normalizeExt(ext)] = struct{}{}
	}

	return matcher{
		includeExt:      set,
		defaultPatterns: append([]string(nil), defaults...),
		sourcePatterns:  append([]string(nil), sourcePatterns...),
	}
}

func (m matcher) skip(rel string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)

	for _, part := range strings.Split(rel, "/") {
		for _, pattern := range m.defaultPatterns {
			if !strings.Contains(pattern, "*") && part == pattern {
				return true
			}
		}
	}

	for _, pattern := range m.defaultPatterns {
		if !strings.Contains(pattern, "*") {
			continue
		}
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
	}

	for _, pattern := range m.sourcePatterns {
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "/**")
			if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
				return true
			}
		}
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
	}

	if isDir && strings.HasPrefix(base, ".") && base != ".github" {
		return true
	}

	return false
}

func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", path, err)
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return "", errBinaryFile
	}
	return string(data), nil
}

func normalizeExt(ext string) string {
	return strings.TrimPrefix(strings.ToLower(ext), ".")
}
