# sem

Local-first semantic search for your repos, markdown notes, docs, and knowledge vaults.

Think of it as a search engine that understands meaning, not just keywords. Point it at your documents, let it build an index, and then ask questions in plain language.

## Quick Start

```bash
# Build
golang build -o bin/sem ./cmd/sem

# Initialize
./sem init

# Add a source (your notes, repo, docs, etc.)
./sem source add ~/my-vault --name vault

# Build the index
./sem index

# Search!
./sem search "how do I configure authentication?"
```

## Installation

### From Source

Requirements:
- Go 1.21 or later

```bash
git clone https://github.com/yourusername/sem.git
cd sem
golang build -o bin/sem ./cmd/sem
```

Move the binary somewhere in your PATH:

```bash
mv sem /usr/local/bin/
```

## Usage

### Initialize

```bash
sem init
```

This creates `~/.sem/` with default configuration and downloads the embedding model.

### Add Sources

Sources are directories you want to search. Add as many as you like:

```bash
sem source add ~/projects/my-app --name my-app
sem source add ~/notes --name notes
sem source add ~/docs --name docs
```

List configured sources:

```bash
sem source list
```

Remove a source:

```bash
sem source remove my-app
```

### Index

Build the semantic index from your sources:

```bash
sem index
```

This walks through all enabled sources, chunks the content, generates embeddings, and stores everything in a portable bundle.

Index a specific source only:

```bash
sem index --source notes
```

### Search

Search your indexed content:

```bash
sem search "database connection pooling"
sem search "error handling best practices"
sem search "meeting notes from last week"
```

Get JSON output for scripting:

```bash
sem search "api design" --json
```

Limit results:

```bash
sem search "authentication" --limit 20
```

Search within a specific source:

```bash
sem search "config" --source notes
```

## Configuration

Config lives at `~/.sem/config.toml`. The defaults work well for most cases, but you can tweak:

- **Embedding mode**: `light`, `balanced`, `quality`, or `nomic`
- **Chunk size**: How text gets split for indexing
- **Ignore patterns**: Files/directories to skip

### Embedding Modes

| Mode | Model | Speed | Quality |
|------|-------|-------|---------|
| `light` | MiniLM | Fastest | Good |
| `balanced` | BGE Small | Fast | Better |
| `quality` | BGE Base | Moderate | Best |
| `nomic` | Nomic Embed | Fast | Great for code |

Default is `balanced`. Change in config:

```toml
[general]
embedding_mode = "quality"
```

## How It Works

1. **Scan** - Walks your source directories, respecting ignore patterns
2. **Chunk** - Splits documents into semantic chunks (not just fixed-size blocks)
3. **Embed** - Runs local ONNX models to generate vector embeddings
4. **Store** - Saves everything in a portable Parquet bundle
5. **Search** - Finds similar chunks using cosine similarity

The bundle is canonical—you can delete `~/.sem/backends/` and rebuild from the bundle anytime.

## Documentation

- [Stage 1 Specification](docs/stage1-spec.md) - Technical details and architecture
- [Go Implementation Spec](docs/stage1-spec-go.md) - Implementation notes

## Why sem?

Most search tools match exact words. That works for finding "authentication" in a file about authentication. But what if your file says "login flow" or "sign in process"? Traditional search misses it.

Semantic search understands that "authentication" and "login" are related concepts. It finds what you mean, not just what you type.

Plus, everything runs locally. Your notes stay on your machine. No cloud, no API calls, no tracking.

## License

MIT
