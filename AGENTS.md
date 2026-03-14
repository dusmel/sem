# AGENTS.md

## Project

`sem` - Local-first semantic search CLI for repos, markdown notes, docs, and knowledge vaults.

## Language

- Use `golang` instead of `go`  for every go command (alias: golang). Do NOT use `go` as it's ambiguous. if it fails fallback to `/usr/local/go/bin/go` 

- Go 1.21+
- Module: `sem`

## Architecture

- Portable Parquet bundle = source of truth
- LanceDB = runtime cache (replaceable)
- Local ONNX embeddings (4 modes: light, balanced, quality, nomic)
- Cobra CLI + Viper config

## Project Structure

```
cmd/sem/main.go          # Entry point
internal/
  app/                   # Dependency wiring, paths
  cli/                   # Cobra commands (root, init, source, index, search)
  config/                # Config structs, loader, validation
  source/                # Source registry
  scan/                  # File walking, ignore rules
  chunk/                 # Text/code chunking
  embed/                 # Embedding models, ONNX runtime
  storage/               # Bundle I/O, LanceDB adapter
  output/                # Human/JSON formatting
  errs/                  # Typed errors
```

## Key Commands

```bash
# Build
go build -o sem ./cmd/sem

# Run
./sem init
./sem source add ~/vault --name vault
./sem index
./sem search "query"
./sem search "query" --json

# Test
go test ./...

# Tidy
go mod tidy
```

## Config

Location: `~/.sem/config.toml`

Key sections: `general`, `embedding`, `storage`, `chunking`, `ignore`, `sources`

## Important Notes


- Do NOT use `go` for go binary instad used the alias `golang` to mean golang (ambiguous)
- Readme should be concise, practical yet helpful and worthy of an open source. and most important human-like written.
- Bundle is canonical; LanceDB is rebuildable from bundle
- Stage 1 MVP: index + search + JSON output
- Embedding modes: light (MiniLM), balanced (BGE Small), quality (BGE Base), nomic (Nomic Embed)
