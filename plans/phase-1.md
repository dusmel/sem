# Phase 1 Reference Document

> Stage 1: Core Local MVP Implementation

## Summary

Phase 1 implemented the foundational MVP of `sem` - a local-first semantic search CLI. The implementation includes:

- **Configuration system** with Viper (TOML-based)
- **Source registry** for managing indexed directories
- **File scanning** with ignore rules (gitignore-style patterns)
- **Text chunking** with support for markdown, code, and plain text
- **Hash-based embeddings** (placeholder for real ONNX models)
- **Parquet bundle storage** (canonical data) + JSON cache (runtime)
- **CLI commands**: `init`, `source add/list/remove`, `index`, `search`

The MVP demonstrates the full pipeline from file scanning to semantic search, using deterministic hash-based vectors as a placeholder for real ONNX embeddings.

---

## Files Created

### Entry Point

| File | Description |
|------|-------------|
| [`cmd/sem/main.go`](../cmd/sem/main.go) | Application entry point, loads config and executes CLI |

### Internal Packages

| File | Description |
|------|-------------|
| [`internal/app/app.go`](../internal/app/app.go) | Application dependency container |
| [`internal/app/paths.go`](../internal/app/paths.go) | Path resolution for config, bundles, and cache |
| [`internal/config/config.go`](../internal/config/config.go) | Configuration structs and defaults |
| [`internal/config/loader.go`](../internal/config/loader.go) | Viper-based config loading with validation |
| [`internal/config/validate.go`](../internal/config/validate.go) | Configuration validation logic |
| [`internal/source/registry.go`](../internal/source/source.go) | Source registry management (add/list/remove) |
| [`internal/scan/walker.go`](../internal/scan/walker.go) | Directory walking with ignore pattern matching |
| [`internal/chunk/chunker.go`](../internal/chunk/chunker.go) | Text chunking with overlap, heading extraction |
| [`internal/embed/model.go`](../internal/embed/model.go) | Embedding model specifications (4 modes) |
| [`internal/embed/service.go`](../internal/embed/service.go) | Hash-based embedding service (Stage 1 placeholder) |
| [`internal/storage/bundle.go`](../internal/storage/bundle.go) | Parquet bundle I/O (chunks, embeddings, manifest) |
| [`internal/storage/lancedb.go`](../internal/storage/lancedb.go) | JSON-based vector cache with cosine search |
| [`internal/indexer/indexer.go`](../internal/indexer/indexer.go) | Indexing pipeline orchestrator |
| [`internal/output/format.go`](../internal/output/format.go) | Human-readable and JSON output formatting |
| [`internal/errs/kinds.go`](../internal/errs/kinds.go) | Typed error definitions |

### CLI Commands

| File | Description |
|------|-------------|
| [`internal/cli/root.go`](../internal/cli/root.go) | Root Cobra command and flag setup |
| [`internal/cli/init.go`](../internal/cli/init.go) | `sem init` - Initialize config and directories |
| [`internal/cli/source.go`](../internal/cli/source.go) | `sem source add/list/remove` - Source management |
| [`internal/cli/index.go`](../internal/cli/index.go) | `sem index` - Build the semantic index |
| [`internal/cli/search.go`](../internal/cli/search.go) | `sem search` - Query the index |

### Documentation

| File | Description |
|------|-------------|
| [`README.md`](../README.md) | Project overview and quick start |
| [`docs/usage.md`](../docs/usage.md) | Comprehensive CLI usage guide |
| [`docs/stage1-spec.md`](../docs/stage1-spec.md) | Stage 1 specification |
| [`docs/stage1-spec-go.md`](../docs/stage1-spec-go.md) | Go implementation details |
| [`docs/maintainer-guide.md`](../docs/maintainer-guide.md) | Maintainer documentation |

---

## Commands Implemented

### `sem init`

Initializes the sem environment in `~/.sem/`.

```bash
sem init [--force]
```

**Behavior:**
- Creates `~/.sem/` directory structure
- Generates default `config.toml`
- Creates bundles and backends directories
- `--force` overwrites existing config

**Output:**
```
Initialized sem at /home/user/.sem
Config: /home/user/.sem/config.toml
Next step: sem source add <path>
```

---

### `sem source add`

Adds a source directory to the configuration.

```bash
sem source add <path> [--name <name>]
```

**Behavior:**
- Resolves path to absolute
- Uses directory name if `--name` not provided
- Validates name uniqueness and format
- Persists to config file

**Output:**
```
Added source my-vault -> /home/user/Documents/my-vault
Next step: sem index
```

---

### `sem source list`

Lists all configured sources.

```bash
sem source list
```

**Output:**
```
NAME            ENABLED PATH
my-vault        true    /home/user/Documents/my-vault
project-docs    true    /home/user/projects/docs
```

---

### `sem source remove`

Removes a source from configuration.

```bash
sem source remove <name>
```

**Output:**
```
Removed source my-vault
Run sem index to rebuild the bundle without this source.
```

---

### `sem index`

Builds the semantic index from configured sources.

```bash
sem index [--source <name>]
```

**Behavior:**
1. Scans enabled sources for supported files
2. Applies ignore patterns (`.git`, `node_modules`, etc.)
3. Chunks content with overlap
4. Generates embeddings (hash-based in Stage 1)
5. Writes to Parquet bundle
6. Rebuilds JSON vector cache

**Output:**
```
Indexed 2 sources, 142 files, 1847 chunks in 2.345s
Embedding mode: balanced (bge-small-en-v1.5)
```

---

### `sem search`

Searches indexed content semantically.

```bash
sem search <query> [--json] [--limit N] [--source <name>]
```

**Behavior:**
1. Generates embedding for query
2. Computes cosine similarity against all vectors
3. Returns top N results sorted by score

**Human Output:**
```
Query: authentication setup
Mode: balanced (bge-small-en-v1.5)
Found 10 results in 45ms

━━━ 1. docs/security.md (score: 0.892)
Source: project-docs
...authentication configuration is handled through...
────────────────────────────────────────
```

**JSON Output:**
```json
{
  "query": "authentication setup",
  "mode": "balanced",
  "results": [...],
  "total": 10,
  "elapsed_ms": 45
}
```

---

## Architecture Decisions

### 1. Hash-Based Embeddings (MVP Placeholder)

**Decision:** Use deterministic hash-based vectors instead of real ONNX embeddings.

**Rationale:**
- Allows testing the full pipeline without model dependencies
- Deterministic results for testing and debugging
- Simple implementation (~50 lines of code)
- Easy to replace with real embeddings later

**Implementation:**
- FNV-1a hash of tokens mapped to vector dimensions
- Bigram hashing for context
- L2 normalization for cosine similarity

**File:** [`internal/embed/service.go`](../internal/embed/service.go:49-68)

```go
func (s *Service) embedText(text string) []float32 {
    vec := make([]float32, s.spec.Dimension)
    tokens := tokenize(text)
    for i, token := range tokens {
        applyHashedToken(vec, token, 1.0)
        if i > 0 {
            applyHashedToken(vec, tokens[i-1]+"_"+token, 0.5)
        }
    }
    if s.spec.NormalizeOutput {
        normalize(vec)
    }
    return vec
}
```

---

### 2. Parquet for Canonical Storage

**Decision:** Use Parquet files as the canonical source of truth.

**Rationale:**
- Columnar format efficient for embeddings
- Portable across languages and systems
- Supports compression
- Easy to backup and version

**Structure:**
```
~/.sem/bundles/default/
├── chunks.parquet      # Text chunks with metadata
├── embeddings.parquet  # Vector embeddings
├── manifest.json       # Bundle metadata
└── model.json          # Embedding model spec
```

**File:** [`internal/storage/bundle.go`](../internal/storage/bundle.go:65-82)

---

### 3. JSON Cache for Vector Search

**Decision:** Use a JSON file as the vector cache instead of LanceDB.

**Rationale:**
- Stage 1 simplicity - no native dependencies
- Easy to debug and inspect
- Sufficient for MVP scale (<100k vectors)
- Rebuildable from Parquet bundle

**Implementation:**
- Load all vectors into memory
- Compute cosine similarity in Go
- Sort and return top N results

**File:** [`internal/storage/lancedb.go`](../internal/storage/lancedb.go:45-77)

---

### 4. Viper for Configuration

**Decision:** Use Viper with TOML format for configuration.

**Rationale:**
- Mature, well-documented library
- Supports environment variables
- TOML is human-readable and editable
- Easy to validate and extend

**Config Location:** `~/.sem/config.toml`

**File:** [`internal/config/loader.go`](../internal/config/loader.go)

---

### 5. Cobra for CLI

**Decision:** Use Cobra for CLI structure.

**Rationale:**
- De facto standard for Go CLIs
- Built-in help generation
- Flag parsing and validation
- Subcommand support

**File:** [`internal/cli/root.go`](../internal/cli/root.go)

---

## Known Limitations

### Not Yet Implemented

| Feature | Status | Notes |
|---------|--------|-------|
| **Real ONNX Embeddings** | ❌ | Using hash-based placeholder |
| **Incremental Sync** | ❌ | Full rebuild on each `index` |
| **File Watching** | ❌ | No automatic re-indexing |
| **LanceDB Integration** | ❌ | Using JSON cache instead |
| **Model Download** | ❌ | No automatic model fetching |
| **Multiple Bundles** | ⚠️ | Config supports it, CLI doesn't expose |
| **Source-Specific Ignore** | ⚠️ | Config supports it, not fully tested |
| **Progress Reporting** | ❌ | No progress bars or ETA |
| **Concurrent Embedding** | ❌ | Sequential processing |

### Hash-Based Embedding Limitations

The hash-based embeddings in Stage 1 have significant limitations:

1. **No semantic understanding** - "authentication" and "login" are not related
2. **Token-based only** - No contextual embeddings
3. **Same text = same vector** - Good for testing, bad for real search
4. **Quality varies** - Works better with exact term matches

**Workaround:** Test with queries that use exact terms from your documents.

---

## Testing Notes

### Build and Run

```bash
# Build
golang build -o sem ./cmd/sem

# Initialize
./sem init

# Add a test source
./sem source add ~/Documents/notes --name notes

# Index
./sem index

# Search
./sem search "test query"
./sem search "test query" --json
```

### Run Tests

```bash
# Run all tests
golang test ./...

# Run with verbose output
golang test -v ./...

# Run specific package
golang test ./internal/chunk/...
```

### Test Data

Create test files in your source directory:

```bash
mkdir -p ~/Documents/notes

cat > ~/Documents/notes/test.md << 'EOF'
# Authentication Guide

This document explains how to set up authentication.

## OAuth Setup

Configure OAuth by creating an application in the developer console.

## API Keys

For server-to-server communication, use API keys.
EOF

./sem index
./sem search "how to configure oauth"
```

### Verify Bundle

```bash
# Check bundle structure
ls -la ~/.sem/bundles/default/

# View manifest
cat ~/.sem/bundles/default/manifest.json

# View model spec
cat ~/.sem/bundles/default/model.json
```

### Clean Slate

```bash
# Remove all sem data
rm -rf ~/.sem

# Reinitialize
./sem init
```

---

## Commit History

Phase 1 was implemented in 7 commits:

| Commit | Message |
|--------|---------|
| `b130b1b` | Initialize Go module and project structure |
| `2044132` | Add core internal packages |
| `1db55f4` | Add scan and chunk packages |
| `aa66cee` | Add embed and storage packages |
| `3f7b4ee` | Add indexer and output packages |
| `ad65cf9` | Add CLI commands with Cobra |
| `8857cd0` | Add documentation for Stage 1 |

### Commit Details

#### `b130b1b` - Initialize Go module and project structure
- Created `go.mod` with module name `sem`
- Added `AGENTS.md` with project rules
- Created `.gitignore` for Go projects

#### `2044132` - Add core internal packages
- `internal/app/` - Application paths and container
- `internal/config/` - Configuration structs and loader
- `internal/source/` - Source registry
- `internal/errs/` - Typed errors

#### `1db55f4` - Add scan and chunk packages
- `internal/scan/` - Directory walking with ignore patterns
- `internal/chunk/` - Text chunking with overlap

#### `aa66cee` - Add embed and storage packages
- `internal/embed/` - Model specs and hash-based service
- `internal/storage/` - Parquet bundle and JSON cache

#### `3f7b4ee` - Add indexer and output packages
- `internal/indexer/` - Indexing pipeline orchestrator
- `internal/output/` - Human and JSON formatters

#### `ad65cf9` - Add CLI commands with Cobra
- `internal/cli/` - All CLI commands (init, source, index, search)
- `cmd/sem/main.go` - Entry point

#### `8857cd0` - Add documentation for Stage 1
- `README.md` - Project overview
- `docs/usage.md` - Comprehensive usage guide
- `docs/stage1-spec.md` - Specification
- `docs/stage1-spec-go.md` - Go implementation notes
- `docs/maintainer-guide.md` - Maintainer docs

---

## Next Steps (Phase 2)

1. **Real ONNX Embeddings**
   - Integrate `klauspost/go-onnxruntime`
   - Download models from HuggingFace
   - Support all 4 embedding modes

2. **Incremental Sync**
   - Track file modification times
   - Only process changed files
   - Maintain change log

3. **LanceDB Integration**
   - Replace JSON cache with LanceDB
   - Use native vector indexing
   - Support larger datasets

4. **Progress Reporting**
   - Add progress bars for indexing
   - Show ETA and throughput
   - Report errors gracefully

---

## References

- [Stage 1 Specification](../docs/stage1-spec.md)
- [Go Implementation Spec](../docs/stage1-spec-go.md)
- [Usage Guide](../docs/usage.md)
- [Maintainer Guide](../docs/maintainer-guide.md)
