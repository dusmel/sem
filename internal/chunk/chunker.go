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
	// NEW: Chunk metadata enrichment
	FunctionName string `json:"function_name" parquet:"function_name,optional"`
	SectionLevel int    `json:"section_level" parquet:"section_level,optional"`
}

// Build chunks all documents.
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
	createdAt := time.Now().UTC()

	// Extract headings for all markdown files
	headings := extractHeadingsWithPositions(doc.Content, kind == FileKindMarkdown)
	var headingsJSON []byte
	if len(headings) > 0 {
		texts := make([]string, len(headings))
		for i, h := range headings {
			texts[i] = h.Text
		}
		headingsJSON, _ = json.Marshal(texts)
	}

	// Choose chunking strategy based on file kind
	var windows []window
	var chunkTitles []string        // Title for each chunk (from heading)
	var chunkSectionLevels []int    // Section level for each chunk (from heading level)
	var chunkFunctionNames []string // Function name for each chunk (from code boundary)

	switch kind {
	case FileKindMarkdown:
		if cfg.RespectHeadings && len(headings) > 0 {
			windows, chunkTitles, chunkSectionLevels = chunkByHeadings(doc.Content, headings, cfg)
		} else {
			windows = splitWindows(runes, cfg.MaxChars, cfg.OverlapChars)
		}
	case FileKindCode:
		var boundaries []codeBoundary
		windows, boundaries = chunkByCodeBoundaries(doc.Content, doc.Extension, cfg.MaxChars, cfg.OverlapChars)
		// Extract function names from boundaries
		chunkFunctionNames = make([]string, len(windows))
		for i := range windows {
			// Find which boundary this window belongs to
			for j, b := range boundaries {
				if j < len(windows) && i == j && b.Name != "" {
					chunkFunctionNames[i] = b.Name
					break
				}
			}
		}
	default:
		windows = splitWindows(runes, cfg.MaxChars, cfg.OverlapChars)
	}

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

		// Determine title: use heading title if available, otherwise first heading
		title := ""
		if i < len(chunkTitles) && chunkTitles[i] != "" {
			title = chunkTitles[i]
		} else if len(headings) > 0 {
			title = headings[0].Text
		}

		// Determine section level
		sectionLevel := 0
		if i < len(chunkSectionLevels) {
			sectionLevel = chunkSectionLevels[i]
		}

		// Determine function name
		functionName := ""
		if i < len(chunkFunctionNames) {
			functionName = chunkFunctionNames[i]
		}

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
			FunctionName: functionName,
			SectionLevel: sectionLevel,
		})
	}

	if len(records) == 0 {
		title := ""
		if len(headings) > 0 {
			title = headings[0].Text
		}
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

// headingInfo represents a markdown heading with position information.
type headingInfo struct {
	Text      string // Heading text content
	Level     int    // 1-6
	StartRune int    // Rune index where heading starts
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
	case "txt", "text", "rst", "canvas":
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

// extractHeadingsWithPositions extracts markdown headings with their positions.
func extractHeadingsWithPositions(content string, markdown bool) []headingInfo {
	if !markdown {
		return nil
	}

	var headings []headingInfo
	lines := strings.Split(content, "\n")

	currentRune := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			currentRune += len([]rune(line)) + 1 // +1 for newline
			continue
		}

		// Count heading level
		level := 0
		for _, r := range trimmed {
			if r == '#' {
				level++
			} else {
				break
			}
		}

		if level < 1 || level > 6 {
			currentRune += len([]rune(line)) + 1
			continue
		}

		headingText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if headingText == "" {
			currentRune += len([]rune(line)) + 1
			continue
		}

		headings = append(headings, headingInfo{
			Text:      headingText,
			Level:     level,
			StartRune: currentRune,
		})

		currentRune += len([]rune(line)) + 1
	}

	return headings
}

// chunkByHeadings splits markdown content by heading boundaries.
// Returns windows, titles for each window, and section levels for each window.
func chunkByHeadings(content string, headings []headingInfo, cfg config.ChunkingConfig) ([]window, []string, []int) {
	if len(headings) == 0 {
		runes := []rune(content)
		return splitWindows(runes, cfg.MaxChars, cfg.OverlapChars), nil, nil
	}

	runes := []rune(content)
	var windows []window
	var titles []string
	var levels []int

	for i, heading := range headings {
		startRune := heading.StartRune
		endRune := len(runes)
		if i+1 < len(headings) {
			endRune = headings[i+1].StartRune
		}

		sectionLen := endRune - startRune
		if sectionLen <= cfg.MaxChars {
			// Section fits in one chunk
			windows = append(windows, window{Start: startRune, End: endRune})
			titles = append(titles, heading.Text)
			levels = append(levels, heading.Level)
		} else {
			// Section too long, split it with character windows
			subWindows := splitWindows(runes[startRune:endRune], cfg.MaxChars, cfg.OverlapChars)
			for j, sw := range subWindows {
				windows = append(windows, window{
					Start: startRune + sw.Start,
					End:   startRune + sw.End,
				})
				// Only the first sub-window gets the heading title
				if j == 0 {
					titles = append(titles, heading.Text)
					levels = append(levels, heading.Level)
				} else {
					titles = append(titles, "")
					levels = append(levels, 0)
				}
			}
		}
	}

	return windows, titles, levels
}
