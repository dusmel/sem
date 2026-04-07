# Phase 2 Reference Document

> Stage 2: Practical Daily Use Implementation

## Summary

Phase 2 transforms `sem` from a proof-of-concept into a practical daily tool. The key improvements:

- **Incremental sync** — only re-process changed/new/deleted files instead of full rebuilds
- **.gitignore support** — scanner respects `.gitignore` files found in source directories
- **Real ONNX embeddings** — actual neural model inference via ONNX Runtime with WordPiece tokenization
- **Quantized ARM64 models** — INT8 quantized models for 2-4x faster inference on Apple Silicon
- **Batched inference** — multiple texts processed in a single ONNX Run() call
- **Fixed `--source` indexing** — single-source index merges into existing bundle instead of overwriting
- **New commands** — `sem sync` (incremental updates) and `sem status` (index health)

---

## Files Created

| File | Description |
|------|-------------|
| [`internal/indexer/state.go`](../internal/indexer/state.go) | File state tracking: BundleState, FileEntry, Load/Save/Diff |
| [`internal/embed/onnx.go`](../internal/embed/onnx.go) | ONNX Runtime backend: DynamicAdvancedSession, batched inference, quantized model support |
| [`internal/embed/tokenizer.go`](../internal/embed/tokenizer.go) | Minimal WordPiece tokenizer reading HuggingFace tokenizer.json |
| [`internal/embed/download.go`](../internal/embed/download.go) | Model download from HuggingFace (standard + quantized variants) |
| [`internal/cli/sync.go`](../internal/cli/sync.go) | `sem sync` command |
| [`internal/cli/status.go`](../internal/cli/status.go) | `sem status` command |

## Files Modified

| File | Changes |
|------|---------|
| [`internal/scan/walker.go`](../internal/scan/walker.go) | Replaced hand-rolled matcher with `go-gitignore` + ignore stack |
| [`internal/embed/model.go`](../internal/embed/model.go) | Added ONNXFile, ONNXQuantizedFile, TokenizerFile, ModelSizeMB fields |
| [`internal/embed/service.go`](../internal/embed/service.go) | ONNX-first with hash-based fallback, Close(), batching |
| [`internal/storage/bundle.go`](../internal/storage/bundle.go) | Added RemoveChunks(), Merge(), WriteMetadata() |
| [`internal/storage/lancedb.go`](../internal/storage/lancedb.go) | Added MergeRecords() |
| [`internal/indexer/indexer.go`](../internal/indexer/indexer.go) | Full rewrite: incremental sync, --full/--source support |
| [`internal/cli/root.go`](../internal/cli/root.go) | Registered sync and status commands |
| [`internal/cli/index.go`](../internal/cli/index.go) | Wired --full flag, incremental output |
| [`internal/cli/search.go`](../internal/cli/search.go) | Uses NewServiceWithModelDir, Close() |
| [`internal/config/config.go`](../internal/config/config.go) | Added UseGitignore to IgnoreConfig |
| [`internal/errs/kinds.go`](../internal/errs/kinds.go) | Added ErrModelMismatch, ErrConfigChanged |
| [`go.mod`](../go.mod) | Added sabhiram/go-gitignore, yalue/onnxruntime_go dependencies |

---

## Commands

### `sem index` (updated)

Now performs incremental sync by default. Only processes changed/new/deleted files.

```bash
sem index              # Incremental sync (default)
sem index --full       # Force full rebuild
sem index --source X   # Re-index single source (merge, not overwrite)
```

**Incremental output:**
```
Synced 2 sources in 45ms
Files: 3 new, 1 changed, 0 deleted
Chunks: 1847 total
Embedding mode: light (all-MiniLM-L6-v2)
```

**Full rebuild output:**
```
Synced 2 sources in 42.58s
Files: 662 new, 0 changed, 0 deleted
Chunks: 1443 total
Embedding mode: light (all-MiniLM-L6-v2)
```

### `sem sync`

Shorthand for incremental sync. Equivalent to `sem index` without `--full`.

```bash
sem sync              # Incremental sync all sources
sem sync --source X   # Incremental sync single source
```

**Output:**
```
Synced 2 sources: 3 new, 1 changed, 0 deleted files
Chunks: 1847 total
Embedding mode: light (all-MiniLM-L6-v2)
Duration: 45ms
```

### `sem status`

Shows index health and staleness.

```bash
sem status
```

**Output:**
```
Bundle: default
Indexed: 2026-04-07 10:30:00
Model: light (all-MiniLM-L6-v2)
Sources: 2
  vault               142 files
  project-docs          38 files
Total: 180 files, 2259 chunks, 2259 embeddings

State: indexed
```

---

## Architecture: Incremental Sync

### File State Tracking

A `state.json` file in the bundle directory tracks per-file metadata:

```json
{
  "version": "1",
  "embedding_mode": "light",
  "chunking_hash": "abc123...",
  "files": {
    "vault|notes/auth.md": {
      "content_hash": "sha1-of-content",
      "modified_at": "2026-04-07T10:30:00Z",
      "byte_size": 4200,
      "chunk_ids": ["abc", "def"]
    }
  }
}
```

### Diff Algorithm

1. Load previous `state.json`
2. Scan all source directories
3. For each file: compare mod time → content hash → classify as new/changed/unchanged
4. For files in state but not on disk: deleted
5. Process only new + changed files
6. Remove deleted/changed file chunks from bundle
7. Merge new chunks into bundle
8. Save updated state, rebuild vector cache

### Auto Full-Rebuild Triggers

- No previous state (first-time index)
- Embedding mode changed (e.g., light → quality)
- Chunking config changed (different max_chars/overlap_chars)
- `--full` flag explicitly set

---

## Architecture: .gitignore Support

Uses `sabhiram/go-gitignore` for full gitignore semantics.

### Ignore Stack

The scanner maintains a stack of ignore matchers:
- At each directory, check for `.gitignore` file
- If found, parse and push onto stack
- When checking a path, test against all matchers in the stack
- Precedence: `.gitignore` → source exclude patterns → config default patterns

### Config

```toml
[ignore]
default_patterns = [".git", "node_modules", ...]
use_gitignore = true    # New in Phase 2
```

---

## Architecture: ONNX Embeddings

### Detection and Fallback

1. Check if ONNX Runtime shared library is available (common paths + env var)
2. If available: download model files → create ONNX session → use neural inference
3. If not available: fall back to hash-based embeddings with a warning

### Model Download

Models are downloaded from HuggingFace on first use to `~/.sem/models/<mode>/`:
- `model.onnx` — standard ONNX model file
- `model_quantized.onnx` — INT8 quantized model (ARM64 only, when available)
- `tokenizer.json` — HuggingFace WordPiece tokenizer

Downloads are atomic (temp file + rename). Quantized model download failure is non-fatal.

### Quantized ARM64 Models

On ARM64 (Apple Silicon, AWS Graviton), `sem` automatically prefers the quantized model:
- `model_qint8_arm64.onnx` — INT8 quantized, ~4x smaller, ~2-4x faster inference
- Currently available for `light` mode (all-MiniLM-L6-v2)
- Other modes fall back to standard model

### WordPiece Tokenizer

A minimal WordPiece tokenizer (`internal/embed/tokenizer.go`) reads HuggingFace `tokenizer.json`:
- Loads vocabulary map (30K+ entries)
- Applies BERT-style normalization (lowercase, whitespace cleanup)
- Pre-tokenizes on whitespace and punctuation boundaries
- Greedy longest-match WordPiece subword tokenization
- Adds [CLS] and [SEP] special tokens
- Falls back to hash-based tokenization if tokenizer.json is unavailable

This is intentionally "good enough" — edge cases like Chinese character handling and accent stripping are deferred to Phase 3.

### Batched Inference

Texts are processed in batches for efficiency:
1. Service-level batching: groups of 32 texts
2. ONNX-level batching: groups of 8 texts per Run() call
3. Within each batch, texts are padded to the longest sequence length (not maxTokens)
4. DynamicAdvancedSession allows variable shapes per batch
5. Output tensors are auto-allocated by ONNX Runtime

### Session Options

- Intra-op threads set to `runtime.NumCPU()` for maximum parallelism
- Graph optimization level set to `ORT_ENABLE_ALL`
- DynamicAdvancedSession for variable-length inputs

### Performance

| Mode | Model | 173 chunks | 1443 chunks | Notes |
|------|-------|-----------|-------------|-------|
| light (quantized) | MiniLM-L6-v2 INT8 | ~4.5s | ~43s | ARM64 only |
| light (standard) | MiniLM-L6-v2 FP32 | ~23s | — | Any CPU |
| balanced | BGE Small FP32 | ~40s | — | Any CPU |

---

## Known Limitations

| Feature | Status | Notes |
|---------|--------|-------|
| **ONNX inference** | ✅ Working | Real neural embeddings via ONNX Runtime |
| **Tokenizer** | ✅ Working | Minimal WordPiece tokenizer, good enough for English |
| **Quantized models** | ✅ Working | ARM64 INT8 for light mode; other modes lack quantized variants on HuggingFace |
| **Nested .gitignore** | ✅ | Supported via ignore stack |
| **Mod-time fast path** | ✅ | Skip hash computation when mod time unchanged |
| **Tokenizer edge cases** | ⚠️ | No accent stripping, no Chinese char handling — deferred to Phase 3 |
| **Bundle ONNX Runtime** | ❌ | Users must install libonnxruntime separately — deferred to Stage 8 (packaging) |
| **Concurrent embedding** | ❌ | Sequential batching only |
| **Real LanceDB** | ❌ | Still using JSON cache |
| **Progress bars** | ❌ | No progress reporting yet |

---

## Testing

```bash
# Build
go build -o sem ./cmd/sem

# Full workflow
./sem init
./sem source add ~/Documents/notes --name notes
./sem index                    # Full rebuild (first time)
./sem sync                     # Incremental (no changes = fast)
# ... modify a file ...
./sem sync                     # Incremental (only changed files)
./sem search "query"
./sem status

# Test --source merge
./sem source add ~/code --name repos
./sem index                    # Index both sources
./sem index --source notes     # Re-index only notes, repos preserved

# Test --full
./sem index --full             # Force complete rebuild
```

---

## Next Steps (Phase 3)

1. **Hybrid search** — ripgrep exact search + semantic search merged
2. **Better code chunking** — AST-aware chunking for code files
3. **Source/type filters** — filter search results by repo, language, file type
4. **Improved tokenizer** — accent stripping, Chinese character handling, BPE support for non-BERT models
5. **Real LanceDB** — replace JSON cache with actual LanceDB Go bindings
6. **Progress bars** — show indexing progress with ETA
