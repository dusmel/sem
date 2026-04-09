# sem

Local-first semantic search for your repos, markdown notes, docs, and knowledge vaults.

Think of it as a search engine that understands meaning, not just keywords. Point it at your documents, let it build an index, and then ask questions in plain language.

## Quick Start

```bash
# Build
golang build -o sem ./cmd/sem

# Initialize
./sem init

# Add a source
./sem source add ~/my-notes --name notes

# Build the index
./sem index

# Search
./sem search "how do I configure authentication?"
```

## Installation

### From Source

Requirements: Go 1.21+

```bash
git clone https://github.com/yourusername/sem.git
cd sem
golang build -o sem ./cmd/sem
```

Move the binary somewhere in your PATH:

```bash
mv sem /usr/local/bin/
```

### Dependencies

- **ripgrep** (`rg`) — required for exact and hybrid search modes. Install with `brew install ripgrep` (macOS) or `apt install ripgrep` (Linux).
- **ONNX Runtime** — required for real embeddings. Install with `brew install onnxruntime` (macOS). Without it, sem falls back to hash-based embeddings (fast but less accurate).

Run `sem doctor` to check your setup.

## Usage

### Basic Search

```bash
sem search "database connection pooling"
sem search "error handling best practices"
sem search "meeting notes from last week"
```

Limit results:

```bash
sem search "authentication" --limit 20
```

### Search Modes

sem supports three search modes via `--mode`:

| Mode | What it does | When to use |
|------|-------------|-------------|
| `hybrid` | Combines semantic + exact search (default) | Most queries — best overall results |
| `semantic` | Vector similarity only | Conceptual queries, finding related ideas |
| `exact` | ripgrep text search only | Finding specific terms, function names, error messages |

```bash
sem search "login flow" --mode semantic    # Finds "authentication", "sign in", etc.
sem search "HandleLogin" --mode exact      # Finds exact text matches
sem search "login flow" --mode hybrid      # Best of both (default)
```

If ripgrep isn't installed, hybrid and exact modes fall back to semantic automatically.

### Filters

Narrow down results with these flags:

**`--source`** — search within a specific source:

```bash
sem search "config" --source notes
```

**`--language`** — filter by programming language:

```bash
sem search "error handling" --language go
sem search "data pipeline" --language python,rust
```

**`--kind`** — filter by file type:

```bash
sem search "setup instructions" --kind markdown
sem search "database pool" --kind code
```

**`--dir`** — filter by subdirectory:

```bash
sem search "api endpoint" --dir src/
sem search "deployment" --dir docs/
```

### Combining Filters

Stack filters to get precise results:

```bash
# Search Go files in src/ from the my-app source
sem search "error handling" --source my-app --language go --dir src/

# Find markdown docs about deployment
sem search "deploy pipeline" --kind markdown --source docs

# Search TypeScript files only
sem search "authentication middleware" --language typescript --kind code
```

### JSON Output for Scripting

Use `--json` to get structured output:

```bash
sem search "api design" --json
```

Output structure:

```json
{
  "query": "api design",
  "mode": "hybrid",
  "filters": {
    "source": "",
    "language": "",
    "kind": ""
  },
  "results": [
    {
      "chunk_id": "abc123...",
      "file_path": "/Users/hadad/notes/api-design.md",
      "snippet": "REST API design principles include...",
      "score": 0.892,
      "source_name": "notes",
      "metadata": {
        "file_kind": "markdown",
        "language": "markdown",
        "title": "API Design Principles",
        "start_line": 1,
        "end_line": 15
      }
    }
  ],
  "total": 1,
  "elapsed_ms": 45
}
```

Pipe it to `jq` for scripting:

```bash
# Get just the file paths
sem search "auth" --json | jq -r '.results[].file_path'

# Get top result snippet
sem search "auth" --json | jq -r '.results[0].snippet'

# Filter by score threshold
sem search "auth" --json | jq '.results[] | select(.score > 0.7)'
```

### Indexing

**Build the index:**

```bash
sem index
```

**Incremental sync** (only re-indexes changed files):

```bash
sem sync
```

`sem sync` is what you'll run most often — it detects new, modified, and deleted files since the last index and updates accordingly.

**Full rebuild:**

```bash
sem index --full
```

Use `--full` when you've changed embedding modes, chunking settings, or want a clean rebuild.

**Index a specific source:**

```bash
sem index --source notes
sem sync --source my-app
```

### Configuration

Config lives at `~/.sem/config.toml`. The defaults work well for most cases.

#### Embedding Modes

| Mode | Model | Speed | Quality | Best for |
|------|-------|-------|---------|----------|
| `light` | MiniLM | Fastest | Good | Quick searches, low-resource machines |
| `balanced` | BGE Small | Fast | Better | Daily use (default) |
| `quality` | BGE Base | Moderate | Best | When accuracy matters most |
| `nomic` | Nomic Embed | Fast | Great for code | Code-heavy repositories |

Change the mode in your config:

```toml
[embedding]
mode = "quality"
```

Or set it during init — the model downloads automatically on first use.

#### Chunking

```toml
[chunking]
max_chars = 2200       # Maximum chunk size
overlap_chars = 300    # Overlap between chunks
min_chars = 400        # Minimum chunk size (tiny chunks are skipped)
respect_headings = true # Split markdown by headings
```

Code files are split by function/class boundaries. Markdown files are split by headings when `respect_headings` is true.

#### Ignore Patterns

```toml
[ignore]
use_gitignore = true
default_patterns = [".git", "node_modules", "target", "dist", "build", "vendor"]
```

sem respects `.gitignore` files by default. Add patterns to `default_patterns` for global ignores.

### Environment Health

Run `sem doctor` to validate your setup:

```bash
sem doctor
```

It checks:
- ripgrep installation
- ONNX Runtime availability
- Cached model files
- Configuration validity
- Source path accessibility
- Bundle status

## How It Works

sem runs a 5-step pipeline:

1. **Scan** — walks your source directories, respecting `.gitignore` and ignore patterns
2. **Chunk** — splits documents into semantic units (by headings for markdown, by functions for code)
3. **Embed** — runs local ONNX models to generate vector embeddings for each chunk
4. **Store** — saves everything in portable Parquet bundles (the canonical source of truth)
5. **Search** — finds similar chunks using cosine similarity, optionally combined with ripgrep exact matching via Reciprocal Rank Fusion

The bundle is canonical — you can delete `~/.sem/backends/` and rebuild from the bundle anytime with `sem index --full`.

## Why sem?

Most search tools match exact words. That works fine if you're searching for "authentication" in a file that literally says "authentication." But what if your file says "login flow" or "sign in process"? Traditional search misses it entirely.

Semantic search understands that "authentication," "login," and "sign in" are related concepts. It finds what you mean, not just what you type.

And everything runs locally. Your notes stay on your machine. No cloud, no API keys, no sending your data anywhere. The models run on your hardware via ONNX Runtime.

I built this because I was tired of `grep`-ing through my notes and coming up empty on queries I knew the answer to somewhere in there. sem is the tool I wanted — fast, local, and actually finds what I'm looking for.

## License

MIT
