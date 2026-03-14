# sem Usage Guide

A comprehensive guide to using `sem` - a local-first semantic search CLI for repositories, markdown notes, documentation, and knowledge vaults.

## Table of Contents

- [Installation](#installation)
- [CLI Commands](#cli-commands)
  - [sem init](#sem-init)
  - [sem source](#sem-source)
  - [sem index](#sem-index)
  - [sem search](#sem-search)
- [Configuration](#configuration)
  - [Config File Location](#config-file-location)
  - [Configuration Sections](#configuration-sections)
- [Embedding Modes](#embedding-modes)
- [Storage Layout](#storage-layout)
- [Common Workflows](#common-workflows)
- [Environment Variables](#environment-variables)

---

## Installation

Build from source:

```bash
golang build -o sem ./cmd/sem
```

Move the binary to your PATH:

```bash
sudo mv sem /usr/local/bin/
```

---

## CLI Commands

### sem init

Initialize sem in `~/.sem`. This creates the configuration file, directory structure, and downloads the embedding model.

**Usage:**

```bash
sem init [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Overwrite existing configuration and default metadata |

**Examples:**

```bash
# First-time initialization
sem init

# Re-initialize (overwrite existing config)
sem init --force
```

**Expected Output:**

```
Initialized sem at /home/user/.sem
Config: /home/user/.sem/config.toml
Next step: sem source add <path>
```

**What it does:**

1. Creates the `~/.sem` directory structure
2. Generates the default configuration file at `~/.sem/config.toml`
3. Initializes the default bundle directory
4. Prepares for embedding model download (occurs during first index)

---

### sem source

Manage indexed sources. Sources are directories containing files you want to make searchable.

#### sem source add

Add a source directory to the configuration.

**Usage:**

```bash
sem source add <path> [flags]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `<path>` | Yes | Absolute or relative path to the source directory |

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | (directory name) | Explicit name for the source |

**Examples:**

```bash
# Add a notes vault (uses directory name as source name)
sem source add ~/Documents/obsidian-vault

# Add with a custom name
sem source add ~/projects/my-app --name myapp

# Add a documentation directory
sem source add ./docs --name project-docs
```

**Expected Output:**

```
Added source obsidian-vault -> /home/user/Documents/obsidian-vault
Next step: sem index
```

**Notes:**

- If `--name` is not provided, the directory name is used
- Source names must be unique and contain only letters, numbers, dots, underscores, or dashes
- Paths are stored as absolute paths in the configuration

#### sem source list

List all configured sources.

**Usage:**

```bash
sem source list
```

**Examples:**

```bash
sem source list
```

**Expected Output:**

```
NAME            ENABLED PATH
obsidian-vault true    /home/user/Documents/obsidian-vault
myapp           true    /home/user/projects/my-app
project-docs    true    /home/user/projects/docs
```

#### sem source remove

Remove a configured source.

**Usage:**

```bash
sem source remove <name>
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `<name>` | Yes | Name of the source to remove |

**Examples:**

```bash
sem source remove myapp
```

**Expected Output:**

```
Removed source myapp
Run sem index to rebuild the bundle without this source.
```

**Notes:**

- Removing a source does not delete it from the index immediately
- Run `sem index` after removal to rebuild the bundle

---

### sem index

Build the semantic index from all configured sources.

**Usage:**

```bash
sem index [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--source` | `""` | Restrict indexing to a single source |
| `--full` | `false` | Full rebuild (Stage 1 always performs full rebuild) |

**Examples:**

```bash
# Index all configured sources
sem index

# Index only a specific source
sem index --source obsidian-vault
```

**Expected Output:**

```
Indexed 3 sources, 142 files, 1847 chunks in 12.345s
Embedding mode: balanced (bge-small-en-v1.5)
```

**What it does:**

1. Scans all enabled sources for supported files
2. Chunks text content into semantic units
3. Generates embeddings using the configured model
4. Stores chunks and embeddings in the bundle
5. Rebuilds the LanceDB cache for fast searching

---

### sem search

Search indexed content semantically.

**Usage:**

```bash
sem search <query> [flags]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `<query>` | Yes | Natural language search query |

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output results as JSON |
| `--limit` | `10` | Maximum number of results |
| `--source` | `""` | Restrict search to a specific source |

**Examples:**

```bash
# Basic search
sem search "how to configure authentication"

# Get more results
sem search "database connection pooling" --limit 20

# Search within a specific source
sem search "API endpoints" --source myapp

# JSON output for scripting
sem search "error handling patterns" --json

# Combine flags
sem search "react hooks" --source project-docs --limit 5 --json
```

**Human-Readable Output:**

```
Query: how to configure authentication
Mode: balanced (bge-small-en-v1.5)
Found 10 results in 45ms

━━━ 1. auth/config.go (score: 0.892)
Source: myapp
...authentication configuration is handled through the Config struct...
────────────────────────────────────────

━━━ 2. docs/security.md (score: 0.867)
Source: project-docs
# Authentication Setup
To configure authentication, first create an OAuth application...
────────────────────────────────────────
```

**JSON Output:**

```json
{
  "query": "how to configure authentication",
  "mode": "balanced",
  "results": [
    {
      "chunk_id": "abc123",
      "file_path": "/home/user/projects/myapp/auth/config.go",
      "snippet": "authentication configuration is handled through...",
      "score": 0.892,
      "source_name": "myapp",
      "metadata": {
        "file_kind": "code",
        "language": "go",
        "title": "config.go",
        "start_line": 15,
        "end_line": 42
      }
    }
  ],
  "total": 10,
  "elapsed_ms": 45
}
```

---

## Configuration

### Config File Location

The configuration file is located at:

```
~/.sem/config.toml
```

### Configuration Sections

#### `[general]`

General application settings.

```toml
[general]
default_bundle = "default"    # Name of the default bundle
embedding_mode = "balanced"   # Default embedding mode
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_bundle` | string | `"default"` | Name of the bundle to use |
| `embedding_mode` | string | `"balanced"` | Embedding mode (light, balanced, quality, nomic) |

#### `[embedding]`

Embedding model configuration.

```toml
[embedding]
mode = "balanced"                              # Embedding mode
model_cache_dir = "/home/user/.sem/models"     # Where to cache models
batch_size = 32                                # Embedding batch size
max_tokens = 512                               # Maximum tokens per chunk
normalize = true                               # Normalize output vectors
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `"balanced"` | Embedding mode: `light`, `balanced`, `quality`, or `nomic` |
| `model_cache_dir` | string | `~/.sem/models` | Absolute path to model cache directory |
| `batch_size` | int | `32` | Number of texts to embed per batch |
| `max_tokens` | int | `512` | Maximum tokens per text chunk |
| `normalize` | bool | `true` | Whether to normalize output vectors |

#### `[storage]`

Storage backend configuration.

```toml
[storage]
bundle_dir = "/home/user/.sem/bundles"         # Bundle storage directory
backend = "lancedb"                            # Vector backend (Stage 1: lancedb only)

[storage.lancedb]
path = "/home/user/.sem/backends/lancedb"     # LanceDB storage path
table = "chunks"                               # Table name
metric = "cosine"                              # Distance metric
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `bundle_dir` | string | `~/.sem/bundles` | Absolute path to bundle storage |
| `backend` | string | `"lancedb"` | Vector search backend (Stage 1 supports only `lancedb`) |

**`[storage.lancedb]` sub-section:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | `~/.sem/backends/lancedb` | Absolute path to LanceDB data |
| `table` | string | `"chunks"` | Table name for vectors |
| `metric` | string | `"cosine"` | Distance metric for similarity |

#### `[chunking]`

Text chunking configuration.

```toml
[chunking]
max_chars = 2200          # Maximum characters per chunk
overlap_chars = 300       # Character overlap between chunks
min_chars = 400           # Minimum characters per chunk
respect_headings = true   # Split on markdown headings
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_chars` | int | `2200` | Maximum characters per chunk |
| `overlap_chars` | int | `300` | Overlap between consecutive chunks |
| `min_chars` | int | `400` | Minimum characters (smaller chunks are merged) |
| `respect_headings` | bool | `true` | Whether to split on markdown headings |

#### `[ignore]`

Default ignore patterns for all sources.

```toml
[ignore]
default_patterns = [
    ".git",
    "node_modules",
    "target",
    "dist",
    "build",
    "vendor",
    "__pycache__",
    ".idea",
    ".vscode",
    ".obsidian",
    "*.min.js",
    "*.min.css"
]
```

| Field | Type | Description |
|-------|------|-------------|
| `default_patterns` | []string | Glob patterns for files/directories to ignore |

#### `[[sources]]`

Array of source configurations.

```toml
[[sources]]
name = "obsidian-vault"
path = "/home/user/Documents/obsidian-vault"
enabled = true
include_extensions = [".md", ".txt"]
exclude_patterns = ["*.tmp", "drafts/*"]

[[sources]]
name = "myapp"
path = "/home/user/projects/my-app"
enabled = true
include_extensions = [".go", ".md", ".yaml"]
exclude_patterns = ["*_test.go", "vendor/*"]
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique source identifier |
| `path` | string | Yes | Absolute path to source directory |
| `enabled` | bool | No | Whether to index this source (default: `true`) |
| `include_extensions` | []string | No | File extensions to include (empty = all supported) |
| `exclude_patterns` | []string | No | Additional patterns to exclude |

---

## Embedding Modes

Sem supports four embedding modes, each with different trade-offs between speed, quality, and resource usage.

### Mode Comparison

| Mode | Model | Dimensions | Max Tokens | Speed | Quality | Best For |
|------|-------|------------|------------|-------|---------|----------|
| `light` | all-MiniLM-L6-v2 | 384 | 256 | Fastest | Good | Testing, quick prototypes |
| `balanced` | bge-small-en-v1.5 | 384 | 512 | Fast | Better | General use, notes |
| `quality` | bge-base-en-v1.5 | 768 | 512 | Medium | Best | Production, accuracy-critical |
| `nomic` | nomic-embed-text-v1 | 768 | 512 | Medium | Excellent | Code, technical docs |

### Detailed Descriptions

#### light - MiniLM

```toml
[embedding]
mode = "light"
```

- **Model**: `sentence-transformers/all-MiniLM-L6-v2`
- **Dimensions**: 384
- **Max Tokens**: 256
- **Size**: ~80MB

**Best for:**
- Quick testing and prototyping
- Resource-constrained environments
- When speed is more important than accuracy

**Trade-offs:**
- ✅ Fastest embedding generation
- ✅ Smallest memory footprint
- ✅ Quick model download
- ❌ Lower accuracy for complex queries
- ❌ Shorter context window (256 tokens)

#### balanced - BGE Small (Default)

```toml
[embedding]
mode = "balanced"
```

- **Model**: `BAAI/bge-small-en-v1.5`
- **Dimensions**: 384
- **Max Tokens**: 512
- **Size**: ~130MB

**Best for:**
- General-purpose semantic search
- Personal knowledge bases
- Good balance of speed and quality

**Trade-offs:**
- ✅ Good accuracy for most use cases
- ✅ Reasonable speed
- ✅ Larger context window than light
- ❌ Not as accurate as quality mode
- ❌ May struggle with highly technical content

**Note**: Uses query prefix `"Represent this sentence for searching relevant passages: "` for optimal search performance.

#### quality - BGE Base

```toml
[embedding]
mode = "quality"
```

- **Model**: `BAAI/bge-base-en-v1.5`
- **Dimensions**: 768
- **Max Tokens**: 512
- **Size**: ~400MB

**Best for:**
- Production systems
- Accuracy-critical applications
- Complex semantic queries

**Trade-offs:**
- ✅ Best accuracy among BGE models
- ✅ Larger embedding dimension (768)
- ✅ Handles nuanced queries well
- ❌ Slower than light/balanced
- ❌ Larger model size

**Note**: Uses query prefix `"Represent this sentence for searching relevant passages: "` for optimal search performance.

#### nomic - Nomic Embed

```toml
[embedding]
mode = "nomic"
```

- **Model**: `nomic-ai/nomic-embed-text-v1`
- **Dimensions**: 768
- **Max Tokens**: 512
- **Size**: ~275MB

**Best for:**
- Code repositories
- Technical documentation
- Mixed code and prose content

**Trade-offs:**
- ✅ Excellent for code understanding
- ✅ Good at technical terminology
- ✅ Context length up to 8192 tokens (in full model)
- ❌ May be overkill for simple notes
- ❌ Slightly slower than balanced

### Choosing a Mode

| Use Case | Recommended Mode |
|----------|------------------|
| Quick testing | `light` |
| Personal notes vault | `balanced` |
| Code repository search | `nomic` |
| Production knowledge base | `quality` |
| Mixed content (code + docs) | `nomic` |
| Resource-limited server | `light` |

---

## Storage Layout

Sem stores all data in `~/.sem/` by default.

### Directory Structure

```
~/.sem/
├── config.toml              # Main configuration file
├── bundles/                 # Parquet bundles (canonical data)
│   └── default/             # Default bundle
│       ├── chunks.parquet   # Text chunks with metadata
│       ├── embeddings.parquet  # Vector embeddings
│       ├── manifest.json    # Bundle metadata
│       └── model.json       # Embedding model spec
├── backends/                # Vector search backends
│   └── lancedb/             # LanceDB cache
│       └── cache.json       # Vector cache for fast search
└── models/                  # Cached embedding models
    └── ...                  # ONNX model files
```

### Bundle Structure

The bundle is the **canonical source of truth**. It contains:

1. **`chunks.parquet`**: Text chunks with metadata
   - Chunk ID, content, file path
   - Source name, file kind, language
   - Line numbers, title

2. **`embeddings.parquet`**: Vector embeddings
   - Chunk ID mapping
   - Embedding vectors (float32 arrays)

3. **`manifest.json`**: Bundle metadata
   - Version, bundle name
   - Embedding model specification
   - Index timestamp
   - Source, file, and chunk counts

4. **`model.json`**: Embedding model specification
   - Mode, name, HuggingFace ID
   - Dimension, max tokens
   - Query/document prefixes

### LanceDB Cache

The LanceDB cache (`backends/lancedb/cache.json`) is a **rebuildable runtime cache**:

- Optimized for fast similarity search
- Can be deleted and rebuilt from the bundle
- Stage 1 uses a JSON-based cache for simplicity

### Backup and Restore

**To backup:**

```bash
# Backup the entire sem directory
tar -czf sem-backup.tar.gz -C ~ .sem

# Or just the bundles (canonical data)
tar -czf sem-bundles-backup.tar.gz -C ~/.sem bundles
```

**To restore:**

```bash
# Restore from backup
tar -xzf sem-backup.tar.gz -C ~

# If only bundles were backed up, restore and reinitialize
tar -xzf sem-bundles-backup.tar.gz -C ~/.sem
```

### Rebuilding from Bundle

Since the bundle is canonical, you can rebuild the LanceDB cache:

```bash
# The cache is automatically rebuilt during indexing
sem index
```

---

## Common Workflows

### Setting Up a New Vault

```bash
# 1. Initialize sem
sem init

# 2. Add your knowledge sources
sem source add ~/Documents/obsidian-vault --name notes
sem source add ~/projects/documentation --name docs

# 3. Build the index
sem index

# 4. Start searching
sem search "project setup instructions"
```

### Indexing Multiple Sources

```bash
# Add multiple sources
sem source add ~/notes --name personal-notes
sem source add ~/work/docs --name work-docs
sem source add ~/code/myapp --name myapp-code

# Index all at once
sem index

# Or index individually (useful for large sources)
sem index --source personal-notes
sem index --source work-docs
sem index --source myapp-code
```

### Searching Effectively

```bash
# Natural language queries work best
sem search "how do I handle authentication errors"
sem search "database migration best practices"
sem search "API rate limiting implementation"

# Use --source to narrow results
sem search "configuration" --source work-docs

# Get more context with higher limits
sem search "error handling" --limit 20

# Use JSON for scripting
sem search "TODO items" --json | jq '.results[].file_path'
```

### Updating the Index After Changes

```bash
# After adding/modifying files in your sources
sem index

# Stage 1 always performs a full rebuild
# Future versions will support incremental updates
```

### Switching Embedding Modes

```bash
# 1. Edit the config file
# Change [embedding].mode to your desired mode
vim ~/.sem/config.toml

# Or use sed
sed -i 's/^mode = .*/mode = "quality"/' ~/.sem/config.toml

# 2. Rebuild the index with the new model
sem index

# The new embeddings will use the selected mode
```

### Integrating with Scripts

```bash
# Find all files matching a concept
sem search "security vulnerability" --json | \
  jq -r '.results[].file_path' | \
  sort | uniq

# Count matches per source
sem search "API endpoint" --json | \
  jq -r '.results[].source_name' | \
  sort | uniq -c

# Get file paths with scores above threshold
sem search "database query" --json | \
  jq -r '.results[] | select(.score > 0.8) | "\(.file_path) (\(.score))"'
```

---

## Environment Variables

Sem supports configuration via environment variables with the `SEM_` prefix:

| Environment Variable | Config Path | Example |
|---------------------|-------------|---------|
| `SEM_GENERAL_DEFAULT_BUNDLE` | `general.default_bundle` | `default` |
| `SEM_GENERAL_EMBEDDING_MODE` | `general.embedding_mode` | `balanced` |
| `SEM_EMBEDDING_MODE` | `embedding.mode` | `quality` |
| `SEM_EMBEDDING_BATCH_SIZE` | `embedding.batch_size` | `64` |
| `SEM_STORAGE_BACKEND` | `storage.backend` | `lancedb` |

**Example:**

```bash
# Override embedding mode via environment
export SEM_EMBEDDING_MODE=quality
sem index

# Use a different bundle
export SEM_GENERAL_DEFAULT_BUNDLE=work
sem search "project timeline"
```

---

## Troubleshooting

### "not initialized" error

Run `sem init` first:

```bash
sem init
```

### "no sources configured" error

Add at least one source:

```bash
sem source add /path/to/your/content
```

### "index not found" error

Build the index before searching:

```bash
sem index
```

### Slow indexing

- Use `light` mode for faster indexing
- Reduce `chunking.max_chars` for smaller chunks
- Use `--source` to index specific sources

### Poor search results

- Try `quality` or `nomic` mode for better accuracy
- Ensure your query is descriptive
- Check that relevant files are being indexed with `sem source list`

---

## Getting Help

```bash
# General help
sem --help

# Command-specific help
sem init --help
sem source --help
sem source add --help
sem index --help
sem search --help
```
