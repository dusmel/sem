# Phase 3 Reference Document

> Stage 3: Good Search Quality Implementation

## Summary

Phase 3 transforms `sem` from a basic semantic search tool into a production-quality search system. The key improvements:

- **Hybrid search** — combines semantic similarity with exact text matching via ripgrep, merged with Reciprocal Rank Fusion (RRF, k=60). Default search mode is `hybrid`.
- **Language-aware code chunking** — regex-based function/class boundary detection for Go, Python, JS/TS, and Rust. Markdown respects heading boundaries.
- **Search filters** — `--source`, `--language`, `--kind`, and `--dir` flags for targeted searches.
- **Embedding optimizations** — ONNX O1–O4 graph-optimized variants, dynamic `max_tokens` per model, quantized BGE Small (5–9× speedup), Qdrant BGE Base optimized (~1.5–2× speedup).
- **UX improvements** — progress bars, `--verbose` debug logging, ANSI snippet highlighting, accent stripping, `sem doctor` environment validation.
- **E2E tests** — end-to-end workflow tests covering init → source → index → sync → search.

---

## Files Created

| File | Description |
|------|-------------|
| [`internal/search/ripgrep.go`](../internal/search/ripgrep.go) | Ripgrep invocation, JSON output parsing, `ExactMatch` structs |
| [`internal/search/hybrid.go`](../internal/search/hybrid.go) | Hybrid search orchestration, RRF ranking (k=60) |
| [`internal/chunk/code.go`](../internal/chunk/code.go) | Regex-based code boundary detection (Go, Python, JS/TS, Rust) |
| [`internal/cli/doctor.go`](../internal/cli/doctor.go) | `sem doctor` command |
| [`internal/doctor/checks.go`](../internal/doctor/checks.go) | Environment checks: ripgrep, ONNX Runtime, models, config, sources |
| [`internal/progress/bar.go`](../internal/progress/bar.go) | Progress bar wrapper (`schollz/progressbar/v3`), TTY/verbose-aware |
| [`internal/log/logger.go`](../internal/log/logger.go) | Verbose debug logger |
| [`internal/e2e/e2e_test.go`](../internal/e2e/e2e_test.go) | End-to-end workflow tests |
| [`internal/chunk/chunker_test.go`](../internal/chunk/chunker_test.go) | Unit tests for chunking logic |
| [`internal/embed/tokenizer_test.go`](../internal/embed/tokenizer_test.go) | Unit tests for tokenizer (accent stripping, WordPiece) |
| [`internal/output/format_test.go`](../internal/output/format_test.go) | Unit tests for output formatting |
| [`plans/phase-3.spec.md`](../plans/phase-3.spec.md) | Full technical specification |

## Files Modified

| File | Changes |
|------|---------|
| [`internal/cli/search.go`](../internal/cli/search.go) | Added `--mode`, `--source`, `--language`, `--kind`, `--dir` flags; wired hybrid search, filters, verbose logging |
| [`internal/cli/index.go`](../internal/cli/index.go) | Wired progress bars, verbose output, improved incremental sync reporting |
| [`internal/cli/sync.go`](../internal/cli/sync.go) | Progress bar integration, verbose output |
| [`internal/cli/root.go`](../internal/cli/root.go) | Registered `doctor` command |
| [`internal/chunk/chunker.go`](../internal/chunk/chunker.go) | Language-aware code chunking, heading-aware markdown chunking, `FunctionName`/`SectionLevel` metadata |
| [`internal/embed/model.go`](../internal/embed/model.go) | ONNX O1–O4 variant support, dynamic `max_tokens` per mode, quantized BGE Small/Base model specs |
| [`internal/embed/download.go`](../internal/embed/download.go) | Multi-variant download with priority ordering (optimized → quantized → standard) |
| [`internal/embed/service.go`](../internal/embed/service.go) | Dynamic `max_tokens`, variant selection, `Close()` lifecycle |
| [`internal/embed/tokenizer.go`](../internal/embed/tokenizer.go) | Accent stripping via NFD normalization + combining mark removal |
| [`internal/indexer/indexer.go`](../internal/indexer/indexer.go) | Progress bar integration, atomic state writes, improved incremental sync |
| [`internal/output/format.go`](../internal/output/format.go) | ANSI snippet highlighting, improved human-readable output |
| [`internal/config/config.go`](../internal/config/config.go) | Removed redundant `embedding_mode` from `GeneralConfig` |
| [`internal/config/loader.go`](../internal/config/loader.go) | Config validation, deprecation warnings |
| [`internal/config/validate.go`](../internal/config/validate.go) | Search mode validation |
| [`go.mod`](../go.mod) | Added `schollz/progressbar/v3`, `golang.org/x/text` dependencies |

---

## Commands

### `sem search` (updated)

Now supports hybrid search by default with multiple filter flags.

```bash
sem search "query"                          # Hybrid search (default)
sem search "query" --mode semantic          # Semantic-only
sem search "query" --mode exact             # Ripgrep-only
sem search "query" --source vault           # Filter by source
sem search "query" --language go            # Filter by language
sem search "query" --kind code              # Filter by file kind
sem search "query" --dir src/               # Filter by subdirectory
sem search "query" --verbose                # Debug logging
sem search "query" --json                   # Machine-readable output
```

**Hybrid output:**
```
1. internal/search/hybrid.go (score: 0.892)
   func RRF(semantic []Result, exact []ExactMatch) []HybridResult {
       // Reciprocal Rank Fusion merges both result sets
   }
   Source: sem-repo | Lines: 42-58

2. docs/search.md (score: 0.751)
   Hybrid search combines semantic and exact matching using RRF.
   Source: vault | Lines: 12-15
```

### `sem doctor`

Validates the environment and reports issues with actionable hints.

```bash
sem doctor
```

**Output:**
```
✓ ripgrep 14.1.0 installed at /opt/homebrew/bin/rg
✓ ONNX Runtime 1.17.0
✓ Model 'light' cached (~23MB)
✓ Config valid at /Users/hadad/.sem/config.toml
✓ 2 sources accessible
```

---

## Architecture: Hybrid Search

### Search Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `hybrid` | Semantic + exact merged with RRF | Default, best overall results |
| `semantic` | Vector similarity only | Conceptual queries, fuzzy matching |
| `exact` | Ripgrep text search only | Literal string matching, known terms |

### Reciprocal Rank Fusion (RRF)

RRF formula: `score(d) = Σ 1 / (k + rank_i(d))` where k=60.

- No training or tuning required
- Works across different score scales (cosine similarity vs. boolean match)
- Simple, proven in production systems (Elasticsearch, Pinecone)

### Ripgrep Integration

- Shells out to `rg --json` for structured output
- Parses `begin`/`match`/`end`/`summary` event types
- Extracts file path, line number, matched text, context lines
- Exit code 1 (no matches) is handled gracefully, not as error
- Fetch limit increased to `max(limit*10, 100)` to survive post-search filters

---

## Architecture: Language-Aware Chunking

### Code Chunking

Regex-based boundary detection for 5 languages:

| Language | Patterns |
|----------|----------|
| Go | `func Name(...)`, `type Name struct/interface/...` |
| Python | `def name(...)`, `async def`, `class Name` |
| JavaScript | `function`, `const/let/var = function/arrow`, `class` |
| TypeScript | Same as JS + `export/abstract class` |
| Rust | `fn`, `pub fn`, `struct/enum/trait/impl` |

Falls back to character-window splitting when no boundaries are found or sections exceed `maxChars`.

### Markdown Chunking

When `respect_headings` is enabled:
- Chunks align with heading boundaries (H1–H6)
- Sections exceeding `maxChars` are sub-split with character windows
- Each chunk carries `Title` and `SectionLevel` metadata

### Chunk Metadata

```go
type ChunkMetadata struct {
    FileKind     string // markdown, code, text
    Language     string // go, python, javascript, typescript, rust
    Title        string // Markdown heading text
    FunctionName string // Code function/method name
    SectionLevel int    // Markdown heading level (1-6)
    StartLine    int
    EndLine      int
}
```

---

## Architecture: Embedding Optimizations

### ONNX O1–O4 Variants

Models are downloaded with priority ordering:
1. Quantized ARM64 (INT8) — highest priority on Apple Silicon
2. O4 optimized (graph optimizations) — ~1.5–2× speedup
3. O3/O2/O1 optimized — fallback chain
4. Standard `model.onnx` — last resort

### Dynamic max_tokens

Each model uses its native token limit instead of a config default:

| Mode | Model | max_tokens |
|------|-------|-----------|
| light | MiniLM-L6-v2 | 256 |
| balanced | BGE Small v1.5 | 512 |
| quality | BGE Base v1.5 | 512 |
| nomic | Nomic Embed v1 | 2048 |

~50% reduction in token padding waste for MiniLM.

### Quantized Models

| Mode | Source | Speedup |
|------|--------|---------|
| light | `model_qint8_arm64.onnx` (official) | ~4× |
| balanced | `Qdrant/bge-small-en-v1.5-onnx-Q` | 5–9× |
| quality | `Qdrant/bge-base-en-v1.5-onnx-Q` (`model_optimized.onnx`, FP16) | ~1.5–2× |

---

## Architecture: Filters

| Filter | Stage | Implementation |
|--------|-------|----------------|
| `--source` | Pre-search | Ripgrep path filtering + vector cache pre-filter |
| `--language` | Post-search | Filter by chunk `Metadata.Language` |
| `--kind` | Post-search | Filter by chunk `Metadata.FileKind` |
| `--dir` | Post-search | Filter by file path prefix |

Exact search kind detection: ripgrep results get language/kind detected from file extensions in the CLI layer before filtering.

---

## Known Limitations

| Feature | Status | Notes |
|---------|--------|-------|
| **Hybrid search** | ✅ Working | RRF with k=60, ripgrep integration |
| **Code chunking** | ✅ Working | 5 languages via regex, fallback to char-window |
| **Markdown chunking** | ✅ Working | Heading-aware with sub-splitting |
| **Search filters** | ✅ Working | `--source`, `--language`, `--kind`, `--dir` |
| **ONNX optimizations** | ✅ Working | O1–O4 variants, dynamic max_tokens |
| **Quantized models** | ✅ Working | Light (INT8), balanced (INT8), quality (FP16 optimized) |
| **Progress bars** | ✅ Working | TTY/verbose-aware |
| **`sem doctor`** | ✅ Working | Environment validation |
| **Accent stripping** | ✅ Working | NFD normalization + combining mark removal |
| **Snippet highlighting** | ✅ Working | ANSI color codes in human output |
| **E2E tests** | ✅ Working | Full workflow coverage |
| **Nomic BPE tokenizer** | ❌ Deferred | Needs BPE tokenizer implementation |
| **`--file-match` flag** | ❌ Deferred | Filename-only search |
| **ClassName/Imports metadata** | ❌ Deferred | Deeper code analysis |
| **BGE Base INT8 export** | ❌ Dropped | Qdrant optimized model is sufficient |
| **Real LanceDB** | ❌ Deferred | Still using JSON cache |
| **TUI** | ❌ Deferred | Phase 4 |

---

## Testing

```bash
# Build
/usr/local/go/bin/go build -o sem ./cmd/sem

# Unit tests
/usr/local/go/bin/go test ./...

# Full workflow
./sem init
./sem source add ~/Documents/notes --name notes
./sem index
./sem sync
./sem search "query"
./sem search "query" --mode semantic
./sem search "query" --mode exact
./sem search "query" --language go --kind code
./sem search "query" --json
./sem doctor

# E2E tests (require ripgrep + model cached)
/usr/local/go/bin/go test ./internal/e2e/...
```

---

## Next Steps (Phase 4)

1. **Real LanceDB** — replace JSON cache with actual vector database
2. **TUI** — interactive search with live results
3. **Nomic BPE tokenizer** — full BPE support for Nomic Embed mode
4. **AST-based chunking** — for languages where regex fails
5. **Concurrent embedding** — worker pool for parallel batch processing
6. **CoreML execution provider** — Apple Silicon Neural Engine acceleration
