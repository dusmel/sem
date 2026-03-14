package chunk

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sem/internal/config"
	"sem/internal/scan"
)

type FileKind string

const (
	FileKindMarkdown FileKind = "markdown"
	FileKindCode     FileKind = "code"
	FileKindText     FileKind = "text"
	FileKindUnknown  FileKind = "unknown"
)

type Record struct {
	ID           string    `json:"id" parquet:"id"`
	SourceName   string    `json:"source_name" parquet:"source_name"`
	FilePath     string    `json:"file_path" parquet:"file_path"`
	RelPath      string    `json:"rel_path" parquet:"rel_path"`
	Content      string    `json:"content" parquet:"content"`
	StartLine    int       `json:"start_line" parquet:"start_line"`
	EndLine      int       `json:"end_line" parquet:"end_line"`
	ChunkIndex   int       `json:"chunk_index" parquet:"chunk_index"`
	Kind         string    `json:"kind" parquet:"kind"`
	Extension    string    `json:"extension" parquet:"extension"`
	Language     string    `json:"language" parquet:"language,optional"`
	Title        string    `json:"title" parquet:"title,optional"`
	HeadingsJSON string    `json:"headings_json" parquet:"headings_json,optional"`
	ByteSize     int64     `json:"byte_size" parquet:"byte_size"`
	ContentHash  string    `json:"content_hash" parquet:"content_hash"`
	CreatedAt    time.Time `json:"created_at" parquet:"created_at"`
}

func Build(ctx context.Context, docs []scan.FileDocument, cfg config.ChunkingConfig) ([]Record, error) {
	records := make([]Record, 0, len(docs))
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		fileRecords := buildFile(doc, cfg)
		records = append(records, fileRecords...)
	}

	return records, nil
}

func buildFile(doc scan.FileDocument, cfg config.ChunkingConfig) []Record {
	runes := []rune(doc.Content)
	if len(runes) == 0 {
		return nil
	}

	contentHash := digest(doc.Content)
	kind := classify(doc.Extension)
	language := detectLanguage(doc.Extension)
	title, headings := extractHeadings(doc.Content, kind == FileKindMarkdown)
	headingsJSON, _ := json.Marshal(headings)
	createdAt := time.Now().UTC()
	windows := splitWindows(runes, cfg.MaxChars, cfg.OverlapChars)
	if len(windows) == 0 {
		windows = append(windows, window{Start: 0, End: len(runes)})
	}

	records := make([]Record, 0, len(windows))
	for i, w := range windows {
		content := strings.TrimSpace(string(runes[w.Start:w.End]))
		if content == "" {
			continue
		}
		if len([]rune(content)) < cfg.MinChars && i != len(windows)-1 && len(windows) > 1 {
			continue
		}

		startLine := lineForRuneIndex(runes, w.Start)
		endLine := lineForRuneIndex(runes, w.End)

		records = append(records, Record{
			ID:           chunkID(doc.SourceName, doc.RelPath, i, contentHash),
			SourceName:   doc.SourceName,
			FilePath:     doc.AbsPath,
			RelPath:      doc.RelPath,
			Content:      content,
			StartLine:    startLine,
			EndLine:      endLine,
			ChunkIndex:   i,
			Kind:         string(kind),
			Extension:    doc.Extension,
			Language:     language,
			Title:        title,
			HeadingsJSON: string(headingsJSON),
			ByteSize:     doc.ByteSize,
			ContentHash:  contentHash,
			CreatedAt:    createdAt,
		})
	}

	if len(records) == 0 {
		records = append(records, Record{
			ID:           chunkID(doc.SourceName, doc.RelPath, 0, contentHash),
			SourceName:   doc.SourceName,
			FilePath:     doc.AbsPath,
			RelPath:      doc.RelPath,
			Content:      strings.TrimSpace(doc.Content),
			StartLine:    1,
			EndLine:      lineForRuneIndex(runes, len(runes)),
			ChunkIndex:   0,
			Kind:         string(kind),
			Extension:    doc.Extension,
			Language:     language,
			Title:        title,
			HeadingsJSON: string(headingsJSON),
			ByteSize:     doc.ByteSize,
			ContentHash:  contentHash,
			CreatedAt:    createdAt,
		})
	}

	return records
}

type window struct {
	Start int
	End   int
}

func splitWindows(runes []rune, maxChars, overlapChars int) []window {
	if len(runes) == 0 || maxChars <= 0 {
		return nil
	}
	if len(runes) <= maxChars {
		return []window{{Start: 0, End: len(runes)}}
	}

	step := maxChars - overlapChars
	if step <= 0 {
		step = maxChars
	}

	out := make([]window, 0, (len(runes)/step)+1)
	for start := 0; start < len(runes); start += step {
		end := start + maxChars
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, window{Start: start, End: end})
		if end == len(runes) {
			break
		}
	}
	return out
}

func lineForRuneIndex(runes []rune, idx int) int {
	if idx <= 0 {
		return 1
	}
	if idx > len(runes) {
		idx = len(runes)
	}
	line := 1
	for _, r := range runes[:idx] {
		if r == '\n' {
			line++
		}
	}
	return line
}

func chunkID(sourceName, relPath string, chunkIndex int, contentHash string) string {
	return digest(fmt.Sprintf("%s|%s|%d|%s", sourceName, relPath, chunkIndex, contentHash))
}

func digest(value string) string {
	h := sha1.Sum([]byte(value))
	return hex.EncodeToString(h[:])
}

func classify(ext string) FileKind {
	switch ext {
	case "md", "markdown":
		return FileKindMarkdown
	case "go", "rs", "ts", "tsx", "js", "jsx", "py", "java", "c", "cc", "cpp", "h", "hpp", "sh", "bash", "zsh", "json", "toml", "yaml", "yml":
		return FileKindCode
	case "txt", "text", "rst":
		return FileKindText
	default:
		return FileKindUnknown
	}
}

func detectLanguage(ext string) string {
	languages := map[string]string{
		"go":   "go",
		"rs":   "rust",
		"ts":   "typescript",
		"tsx":  "typescript",
		"js":   "javascript",
		"jsx":  "javascript",
		"py":   "python",
		"sh":   "shell",
		"bash": "shell",
		"zsh":  "shell",
		"json": "json",
		"toml": "toml",
		"yaml": "yaml",
		"yml":  "yaml",
		"md":   "markdown",
	}
	return languages[ext]
}

func extractHeadings(content string, markdown bool) (string, []string) {
	if !markdown {
		return "", nil
	}
	var title string
	headings := []string{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if heading == "" {
			continue
		}
		if title == "" {
			title = heading
		}
		headings = append(headings, heading)
	}
	return title, headings
}
