package chunk

import (
	"strings"
	"testing"

	"sem/internal/config"
	"sem/internal/scan"
)

func TestExtractHeadingsWithPositions(t *testing.T) {
	content := `# Title

Some content here.

## Section 1

More content.

### Subsection 1.1

Even more.

## Section 2

Final content.
`
	headings := extractHeadingsWithPositions(content, true)

	if len(headings) != 4 {
		t.Errorf("Expected 4 headings, got %d", len(headings))
	}

	expected := []struct {
		text  string
		level int
	}{
		{"Title", 1},
		{"Section 1", 2},
		{"Subsection 1.1", 3},
		{"Section 2", 2},
	}

	for i, exp := range expected {
		if i >= len(headings) {
			break
		}
		if headings[i].Text != exp.text {
			t.Errorf("Heading %d: expected text %q, got %q", i, exp.text, headings[i].Text)
		}
		if headings[i].Level != exp.level {
			t.Errorf("Heading %d: expected level %d, got %d", i, exp.level, headings[i].Level)
		}
		if headings[i].StartRune < 0 {
			t.Errorf("Heading %d: StartRune should be >= 0, got %d", i, headings[i].StartRune)
		}
	}
}

func TestExtractHeadingsWithPositionsNotMarkdown(t *testing.T) {
	headings := extractHeadingsWithPositions("# Not a heading", false)
	if len(headings) != 0 {
		t.Errorf("Expected 0 headings for non-markdown, got %d", len(headings))
	}
}

func TestChunkByHeadings(t *testing.T) {
	content := `# Title
Content under title.

## Section 1
Content under section 1.
More content.

## Section 2
Content under section 2.
`

	headings := extractHeadingsWithPositions(content, true)
	cfg := config.ChunkingConfig{
		MaxChars:     1000,
		OverlapChars: 100,
	}

	windows, titles, levels := chunkByHeadings(content, headings, cfg)

	if len(windows) != 3 {
		t.Errorf("Expected 3 windows (one per heading), got %d", len(windows))
	}

	expectedTitles := []string{"Title", "Section 1", "Section 2"}
	expectedLevels := []int{1, 2, 2}

	for i, expTitle := range expectedTitles {
		if i >= len(titles) {
			t.Errorf("Missing title at index %d", i)
			continue
		}
		if titles[i] != expTitle {
			t.Errorf("Window %d: expected title %q, got %q", i, expTitle, titles[i])
		}
		if levels[i] != expectedLevels[i] {
			t.Errorf("Window %d: expected level %d, got %d", i, expectedLevels[i], levels[i])
		}
	}
}

func TestChunkByHeadingsWithLargeSection(t *testing.T) {
	// Create a large section that exceeds maxChars
	largeContent := strings.Repeat("word ", 500) // > 2000 chars
	content := `# Title
` + largeContent + `

## Section 2
Small content.
`

	headings := extractHeadingsWithPositions(content, true)
	cfg := config.ChunkingConfig{
		MaxChars:     1000,
		OverlapChars: 100,
	}

	windows, titles, _ := chunkByHeadings(content, headings, cfg)

	// First section should be split into multiple windows
	if len(windows) < 2 {
		t.Errorf("Expected multiple windows for large section, got %d", len(windows))
	}

	// First window should have the title
	if titles[0] != "Title" {
		t.Errorf("First window should have title 'Title', got %q", titles[0])
	}
}

func TestFindCodeBoundariesGo(t *testing.T) {
	content := `package main

import "fmt"

func main() {
	fmt.Println("Hello")
}

func helper() int {
	return 42
}

type MyStruct struct {
	Value int
}
`

	boundaries := findCodeBoundaries(content, "go")

	if len(boundaries) != 3 {
		t.Errorf("Expected 3 boundaries (2 functions + 1 type), got %d", len(boundaries))
	}

	expected := []struct {
		name string
		typ  string
	}{
		{"main", "function"},
		{"helper", "function"},
		{"MyStruct", "type"},
	}

	for i, exp := range expected {
		if i >= len(boundaries) {
			break
		}
		if boundaries[i].Name != exp.name {
			t.Errorf("Boundary %d: expected name %q, got %q", i, exp.name, boundaries[i].Name)
		}
		if boundaries[i].Type != exp.typ {
			t.Errorf("Boundary %d: expected type %q, got %q", i, exp.typ, boundaries[i].Type)
		}
	}
}

func TestFindCodeBoundariesPython(t *testing.T) {
	content := `def main():
    print("Hello")

class MyClass:
    def method(self):
        pass

async def async_func():
    await something()
`

	boundaries := findCodeBoundaries(content, "py")

	// Should detect: main, MyClass, method, async_func (method inside class is also detected)
	if len(boundaries) < 3 {
		t.Errorf("Expected at least 3 boundaries, got %d", len(boundaries))
	}

	// Check that expected names are present
	hasMain := false
	hasMyClass := false
	hasAsyncFunc := false
	for _, b := range boundaries {
		switch b.Name {
		case "main":
			hasMain = true
		case "MyClass":
			hasMyClass = true
		case "async_func":
			hasAsyncFunc = true
		}
	}

	if !hasMain {
		t.Error("Expected to find 'main' function")
	}
	if !hasMyClass {
		t.Error("Expected to find 'MyClass' class")
	}
	if !hasAsyncFunc {
		t.Error("Expected to find 'async_func' function")
	}
}

func TestFindCodeBoundariesUnsupportedLanguage(t *testing.T) {
	boundaries := findCodeBoundaries("some content", "unknown")
	if boundaries != nil {
		t.Errorf("Expected nil for unsupported language, got %v", boundaries)
	}
}

func TestChunkByCodeBoundaries(t *testing.T) {
	content := `package main

func main() {
	println("Hello")
}

func helper() {
	println("Helper")
}
`

	cfg := config.ChunkingConfig{
		MaxChars:     1000,
		OverlapChars: 100,
	}

	windows, boundaries := chunkByCodeBoundaries(content, "go", cfg.MaxChars, cfg.OverlapChars)

	if len(windows) == 0 {
		t.Error("Expected at least one window")
	}

	// Should have at least 2 function boundaries (main and helper)
	if len(boundaries) < 2 {
		t.Errorf("Expected at least 2 boundaries, got %d", len(boundaries))
	}

	// Check that function names are captured
	hasMain := false
	hasHelper := false
	for _, b := range boundaries {
		if b.Name == "main" {
			hasMain = true
		}
		if b.Name == "helper" {
			hasHelper = true
		}
	}
	if !hasMain {
		t.Error("Expected to find 'main' function in boundaries")
	}
	if !hasHelper {
		t.Error("Expected to find 'helper' function in boundaries")
	}
}

func TestChunkByCodeBoundariesFallback(t *testing.T) {
	// Content with no detectable boundaries should fall back to character windows
	content := `some random content
without any functions
or class definitions
`

	cfg := config.ChunkingConfig{
		MaxChars:     50,
		OverlapChars: 10,
	}

	windows, boundaries := chunkByCodeBoundaries(content, "go", cfg.MaxChars, cfg.OverlapChars)

	if boundaries != nil {
		t.Error("Expected nil boundaries for fallback")
	}

	// Should still produce windows
	if len(windows) == 0 {
		t.Error("Expected windows from fallback")
	}
}

func TestBuildFileMarkdownWithHeadings(t *testing.T) {
	doc := scan.FileDocument{
		SourceName: "test",
		AbsPath:    "/test/file.md",
		RelPath:    "file.md",
		Extension:  "md",
		Content: `# Title

Content here.

## Section 1

More content.
`,
		ByteSize: 50,
	}

	cfg := config.ChunkingConfig{
		MaxChars:        1000,
		OverlapChars:    100,
		MinChars:        10,
		RespectHeadings: true,
	}

	records := buildFile(doc, cfg)

	if len(records) == 0 {
		t.Fatal("Expected at least one record")
	}

	// Should have title set from first heading
	if records[0].Title != "Title" {
		t.Errorf("Expected title 'Title', got %q", records[0].Title)
	}

	// Should have Kind set to markdown
	if records[0].Kind != string(FileKindMarkdown) {
		t.Errorf("Expected kind 'markdown', got %q", records[0].Kind)
	}
}

func TestBuildFileCodeWithBoundaries(t *testing.T) {
	doc := scan.FileDocument{
		SourceName: "test",
		AbsPath:    "/test/file.go",
		RelPath:    "file.go",
		Extension:  "go",
		Content: `package main

func main() {
	println("Hello")
}

func helper() int {
	return 42
}
`,
		ByteSize: 80,
	}

	cfg := config.ChunkingConfig{
		MaxChars:     1000,
		OverlapChars: 100,
		MinChars:     10,
	}

	records := buildFile(doc, cfg)

	if len(records) == 0 {
		t.Fatal("Expected at least one record")
	}

	// Check that FunctionName is set for chunks with detected boundaries
	foundFunction := false
	for _, r := range records {
		if r.FunctionName != "" {
			foundFunction = true
			break
		}
	}

	if !foundFunction {
		t.Error("Expected at least one record with FunctionName set")
	}

	// Should have Kind set to code
	if records[0].Kind != string(FileKindCode) {
		t.Errorf("Expected kind 'code', got %q", records[0].Kind)
	}
}

func TestBuildFileMetadataEnrichment(t *testing.T) {
	// Test markdown with section levels
	doc := scan.FileDocument{
		SourceName: "test",
		AbsPath:    "/test/file.md",
		RelPath:    "file.md",
		Extension:  "md",
		Content: `# H1 Title

## H2 Section

Content.
`,
		ByteSize: 40,
	}

	cfg := config.ChunkingConfig{
		MaxChars:        1000,
		OverlapChars:    100,
		MinChars:        10,
		RespectHeadings: true,
	}

	records := buildFile(doc, cfg)

	if len(records) == 0 {
		t.Fatal("Expected at least one record")
	}

	// Check that at least one record has SectionLevel set
	foundLevel := false
	for _, r := range records {
		if r.SectionLevel > 0 {
			foundLevel = true
			break
		}
	}

	if !foundLevel {
		t.Error("Expected at least one record with SectionLevel set")
	}
}
