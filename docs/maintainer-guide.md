# Maintainer Guide for `sem`

This guide is for engineers who want to maintain or extend the `sem` project. It assumes basic programming knowledge but not deep familiarity with Go.

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Package Responsibilities](#2-package-responsibilities)
3. [How to Make Common Changes](#3-how-to-make-common-changes)
4. [Key Interfaces and Patterns](#4-key-interfaces-and-patterns)
5. [Build, Test, and Development Workflow](#5-build-test-and-development-workflow)

---

## 1. Architecture Overview

### High-Level Data Flow

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Source    │───▶│    Scan     │───▶│    Chunk    │───▶│    Embed    │───▶│   Store     │
│ (directory) │    │ (walk files)│    │(split text) │    │  (vectors)  │    │ (parquet)   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
                                                                │
                                                                ▼
                                                        ┌─────────────┐
                                                        │   Search    │
                                                        │ (query vec) │
                                                        └─────────────┘
```

### Component Interaction

```
                    ┌─────────────────────────────────────────┐
                    │              CLI Layer                   │
                    │  (internal/cli: root, init, source,     │
                    │   index, search commands)               │
                    └─────────────────┬───────────────────────┘
                                      │
                                      ▼
                    ┌─────────────────────────────────────────┐
                    │              App Layer                   │
                    │  (internal/app: paths, config loading)  │
                    └─────────────────┬───────────────────────┘
                                      │
          ┌───────────────────────────┼───────────────────────────┐
          │                           │                           │
          ▼                           ▼                           ▼
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│     Source      │       │     Indexer     │       │    Storage      │
│   (registry)    │       │  (orchestrator) │       │ (bundle+cache)  │
└─────────────────┘       └─────────────────┘       └─────────────────┘
          │                           │                           │
          ▼                           ▼                           ▼
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│      Scan       │       │      Chunk      │       │     Embed       │
│   (file walk)   │       │  (text split)   │       │   (vectors)     │
└─────────────────┘       └─────────────────┘       └─────────────────┘
```

### Key Design Decisions

#### Why is the Bundle Canonical?

The **Parquet bundle** (`~/.sem/bundles/<name>/`) is the source of truth because:

1. **Portability**: Parquet files can be copied, backed up, or synced to cloud storage
2. **Rebuildability**: The vector cache (LanceDB) can be completely rebuilt from the bundle
3. **Versioning**: The bundle includes a manifest with model specs, making it self-describing
4. **Efficiency**: Parquet is a columnar format optimized for analytics and compression

The bundle contains:
- `chunks.parquet` - Text chunks with metadata
- `embeddings.parquet` - Vector embeddings
- `manifest.json` - Index metadata (sources, counts, timestamps)
- `model.json` - Embedding model specification

#### Why LanceDB as a Cache?

LanceDB is used as a **runtime cache** for fast vector similarity search:

1. **Performance**: Optimized for vector operations
2. **Rebuildability**: Can be recreated from the Parquet bundle
3. **Local-first**: No external server required
4. **Future-proof**: Can be swapped for other backends

In Stage 1, we use a JSON-based cache for simplicity. The architecture supports upgrading to real LanceDB or other vector stores.

---

## 2. Package Responsibilities

| Package | File(s) | Responsibility |
|---------|---------|----------------|
| `cmd/sem` | [`main.go`](../cmd/sem/main.go) | Entry point, creates app and executes CLI |
| `internal/app` | [`app.go`](../internal/app/app.go), [`paths.go`](../internal/app/paths.go) | Dependency wiring, path resolution |
| `internal/cli` | [`root.go`](../internal/cli/root.go), [`init.go`](../internal/cli/init.go), [`source.go`](../internal/cli/source.go), [`index.go`](../internal/cli/index.go), [`search.go`](../internal/cli/search.go) | Cobra commands for CLI |
| `internal/config` | [`config.go`](../internal/config/config.go), [`loader.go`](../internal/config/loader.go), [`validate.go`](../internal/config/validate.go) | Config structs, TOML loading, validation |
| `internal/source` | [`registry.go`](../internal/source/registry.go) | Source add/remove/list operations |
| `internal/scan` | [`walker.go`](../internal/scan/walker.go) | File walking, ignore rules, binary detection |
| `internal/chunk` | [`chunker.go`](../internal/chunk/chunker.go) | Text chunking, language detection, heading extraction |
| `internal/embed` | [`model.go`](../internal/embed/model.go), [`service.go`](../internal/embed/service.go) | Embedding model catalog, vector generation |
| `internal/storage` | [`bundle.go`](../internal/storage/bundle.go), [`lancedb.go`](../internal/storage/lancedb.go) | Parquet bundle I/O, vector cache |
| `internal/output` | [`format.go`](../internal/output/format.go) | Human-readable and JSON output formatting |
| `internal/errs` | [`kinds.go`](../internal/errs/kinds.go) | Typed errors and user-friendly formatting |
| `internal/indexer` | [`indexer.go`](../internal/indexer/indexer.go) | Orchestrates the full indexing pipeline |

### Package Dependencies

```
cmd/sem
    └── internal/app
            └── internal/config
                    └── internal/errs

internal/cli
    └── internal/app
    └── internal/config
    └── internal/indexer
    └── internal/embed
    └── internal/storage
    └── internal/output

internal/indexer
    └── internal/scan
    └── internal/chunk
    └── internal/embed
    └── internal/storage
```

---

## 3. How to Make Common Changes

### 3.1 Add a New CLI Command

**Example: Adding a `sem version` command**

1. **Create the command file** `internal/cli/version.go`:

```go
package cli

import (
    "fmt"

    "github.com/spf13/cobra"

    "sem/internal/app"
)

func newVersionCmd(application *app.App) *cobra.Command {
    return &cobra.Command{
        Use:   "version",
        Short: "Print sem version",
        RunE: func(cmd *cobra.Command, args []string) error {
            fmt.Fprintln(cmd.OutOrStdout(), "sem version 0.1.0")
            return nil
        },
    }
}
```

2. **Register the command** in [`internal/cli/root.go`](../internal/cli/root.go):

```go
func NewRootCmd(application *app.App) *cobra.Command {
    cmd := &cobra.Command{
        Use:           "sem",
        Short:         "Local-first semantic search for repos and notes",
        SilenceUsage:  true,
        SilenceErrors: true,
    }

    cmd.AddCommand(
        newInitCmd(application),
        newSourceCmd(application),
        newIndexCmd(application),
        newSearchCmd(application),
        newVersionCmd(application),  // Add this line
    )

    return cmd
}
```

3. **Build and test**:
```bash
golang build -o sem ./cmd/sem
./sem version
```

### 3.2 Change Chunking Logic

**Location**: [`internal/chunk/chunker.go`](../internal/chunk/chunker.go)

The main function is [`Build()`](../internal/chunk/chunker.go:44) which orchestrates chunking:

```go
func Build(ctx context.Context, docs []scan.FileDocument, cfg config.ChunkingConfig) ([]Record, error)
```

**To change chunk size defaults**:

Edit [`internal/config/config.go`](../internal/config/config.go:80):

```go
Chunking: ChunkingConfig{
    MaxChars:        2200,    // Change this
    OverlapChars:    300,     // Change this
    MinChars:        400,     // Change this
    RespectHeadings: true,
},
```

**To add a new file type classification**:

Edit the [`classify()`](../internal/chunk/chunker.go:189) function:

```go
func classify(ext string) FileKind {
    switch ext {
    case "md", "markdown":
        return FileKindMarkdown
    case "go", "rs", "ts", "tsx", "js", "jsx", "py", "java", "c", "cc", "cpp", "h", "hpp", "sh", "bash", "zsh", "json", "toml", "yaml", "yml":
        return FileKindCode
    case "txt", "text", "rst":
        return FileKindText
    case "myext":  // Add your extension
        return FileKindCode
    default:
        return FileKindUnknown
    }
}
```

**To add language detection for a new extension**:

Edit the [`detectLanguage()`](../internal/chunk/chunker.go:202) function:

```go
func detectLanguage(ext string) string {
    languages := map[string]string{
        // ... existing mappings ...
        "myext": "mylanguage",  // Add this
    }
    return languages[ext]
}
```

### 3.3 Add a New Embedding Mode

**Location**: [`internal/embed/model.go`](../internal/embed/model.go)

1. **Add the model specification** to the `catalog` map:

```go
var catalog = map[string]ModelSpec{
    // ... existing models ...
    "custom": {
        Mode:            "custom",
        Name:            "my-custom-model",
        HuggingFaceID:   "org/my-custom-model",
        Dimension:       512,   // Vector dimension
        MaxTokens:       512,   // Max input tokens
        QueryPrefix:     "",    // Optional prefix for queries
        DocumentPrefix:  "",    // Optional prefix for documents
        NormalizeOutput: true,  // Whether to normalize vectors
    },
}
```

2. **Update validation** in [`internal/config/validate.go`](../internal/config/validate.go:71):

```go
func isValidEmbeddingMode(mode string) bool {
    switch mode {
    case "light", "balanced", "quality", "nomic", "custom":  // Add "custom"
        return true
    default:
        return false
    }
}
```

3. **Update documentation** in [`internal/config/config.go`](../internal/config/config.go) error message.

### 3.4 Modify Output Format

**Location**: [`internal/output/format.go`](../internal/output/format.go)

**To change human-readable output**:

Edit [`PrintHuman()`](../internal/output/format.go:41):

```go
func PrintHuman(w io.Writer, response SearchResponse) {
    if len(response.Results) == 0 {
        fmt.Fprintln(w, "No results found.")
        return
    }

    for i, result := range response.Results {
        // Modify this format
        fmt.Fprintf(w, "%d. %s\n", i+1, result.FilePath)
        fmt.Fprintf(w, "   %q\n", cleanSnippet(result.Snippet))
        fmt.Fprintf(w, "   score: %.4f | source: %s | lines: %d-%d\n", 
            result.Score, result.SourceName, result.Metadata.StartLine, result.Metadata.EndLine)
        if result.Metadata.Title != "" {
            fmt.Fprintf(w, "   title: %s\n", result.Metadata.Title)
        }
        fmt.Fprintln(w)
    }
}
```

**To add new fields to search results**:

1. Add to [`SearchResult`](../internal/output/format.go:10) struct:

```go
type SearchResult struct {
    ChunkID    string         `json:"chunk_id"`
    FilePath   string         `json:"file_path"`
    Snippet    string         `json:"snippet"`
    Score      float32        `json:"score"`
    SourceName string         `json:"source_name"`
    Metadata   ResultMetadata `json:"metadata"`
    MyNewField string         `json:"my_new_field"`  // Add this
}
```

2. Update [`internal/cli/search.go`](../internal/cli/search.go) to populate the new field.

### 3.5 Add a New Config Option

**Example: Adding a `log_level` config option**

1. **Add to config struct** in [`internal/config/config.go`](../internal/config/config.go):

```go
type GeneralConfig struct {
    DefaultBundle string `mapstructure:"default_bundle" toml:"default_bundle"`
    EmbeddingMode string `mapstructure:"embedding_mode" toml:"embedding_mode"`
    LogLevel      string `mapstructure:"log_level" toml:"log_level"`  // Add this
}
```

2. **Add default value** in the [`Default()`](../internal/config/config.go:58) function:

```go
General: GeneralConfig{
    DefaultBundle: "default",
    EmbeddingMode: "balanced",
    LogLevel:      "info",  // Add this
},
```

3. **Add validation** in [`internal/config/validate.go`](../internal/config/validate.go):

```go
func (c Config) Validate() error {
    // ... existing validation ...
    
    if c.General.LogLevel != "" {
        validLevels := []string{"debug", "info", "warn", "error"}
        isValid := false
        for _, level := range validLevels {
            if c.General.LogLevel == level {
                isValid = true
                break
            }
        }
        if !isValid {
            return errs.ValidationError{Field: "general.log_level", Message: "must be one of debug, info, warn, error"}
        }
    }
    
    return nil
}
```

4. **Use the config** in your code:

```go
cfg, err := application.LoadConfig()
if err != nil {
    return err
}
logLevel := cfg.General.LogLevel
```

---

## 4. Key Interfaces and Patterns

### 4.1 Dependency Injection Pattern

The `App` struct in [`internal/app/app.go`](../internal/app/app.go) is the main dependency container:

```go
type App struct {
    Paths Paths
}

func New() (*App, error) {
    paths, err := Resolve()
    if err != nil {
        return nil, err
    }
    return &App{Paths: paths}, nil
}

func (a *App) LoadConfig() (config.Config, error) {
    return config.Load(a.Paths.ConfigPath)
}
```

**How it works**:
1. `main.go` creates an `App` instance
2. The `App` is passed to CLI commands
3. Commands use `App` to access paths and config

This pattern makes testing easier and keeps dependencies explicit.

### 4.2 Error Handling Pattern

Errors are defined in [`internal/errs/kinds.go`](../internal/errs/kinds.go):

```go
var (
    ErrNotInitialized     = errors.New("sem is not initialized")
    ErrAlreadyInitialized = errors.New("sem is already initialized")
    ErrNoSources          = errors.New("no sources configured")
    ErrSourceExists       = errors.New("source already exists")
    ErrSourceNotFound     = errors.New("source not found")
    ErrIndexNotFound      = errors.New("index not found")
)

type ValidationError struct {
    Field   string
    Message string
}
```

**User-friendly formatting** via [`Format()`](../internal/errs/kinds.go:25):

```go
func Format(err error) string {
    switch {
    case errors.Is(err, ErrNotInitialized):
        return "sem is not initialized. Run `sem init` first."
    // ... other cases ...
    default:
        return err.Error()
    }
}
```

**Usage in CLI**:

```go
if err != nil {
    return err  // Return error, let Cobra handle it
}
```

In [`main.go`](../cmd/sem/main.go):

```go
if err := cli.NewRootCmd(application).Execute(); err != nil {
    fmt.Fprintln(os.Stderr, errs.Format(err))
    os.Exit(1)
}
```

### 4.3 Configuration Loading Pattern

Config loading uses Viper with TOML files:

```go
// From internal/config/loader.go
func Load(configPath string) (Config, error) {
    v := viper.New()
    v.SetConfigFile(configPath)
    v.SetConfigType("toml")
    v.SetEnvPrefix("SEM")           // Environment variables: SEM_*
    v.AutomaticEnv()                // Read from env

    if err := v.ReadInConfig(); err != nil {
        // Handle missing config
    }

    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return Config{}, err
    }

    if err := cfg.Validate(); err != nil {
        return Config{}, err
    }

    return cfg, nil
}
```

**Config file location**: `~/.sem/config.toml`

**Environment variable override**: `SEM_EMBEDDING_MODE=quality`

### 4.4 Context Pattern for Cancellation

Go's `context.Context` is used for cancellation:

```go
// From internal/scan/walker.go
func ScanSource(ctx context.Context, src config.SourceConfig, defaults []string) ([]FileDocument, error) {
    err := filepath.WalkDir(src.Path, func(path string, d fs.DirEntry, walkErr error) error {
        if err := ctx.Err(); err != nil {  // Check for cancellation
            return err
        }
        // ... processing ...
    })
    // ...
}
```

This allows graceful shutdown when the user presses Ctrl+C.

### 4.5 Key Structs

**Chunk Record** ([`internal/chunk/chunker.go`](../internal/chunk/chunker.go:25)):

```go
type Record struct {
    ID           string    `json:"id" parquet:"id"`
    SourceName   string    `json:"source_name" parquet:"source_name"`
    FilePath     string    `json:"file_path" parquet:"file_path"`
    Content      string    `json:"content" parquet:"content"`
    StartLine    int       `json:"start_line" parquet:"start_line"`
    EndLine      int       `json:"end_line" parquet:"end_line"`
    // ... more fields
}
```

**Embedding Record** ([`internal/storage/bundle.go`](../internal/storage/bundle.go:28)):

```go
type EmbeddingRecord struct {
    ChunkID string    `json:"chunk_id" parquet:"chunk_id"`
    Vector  []float32 `json:"vector" parquet:"vector"`
}
```

**Model Specification** ([`internal/embed/model.go`](../internal/embed/model.go:5)):

```go
type ModelSpec struct {
    Mode            string `json:"mode"`
    Name            string `json:"name"`
    HuggingFaceID   string `json:"huggingface_id"`
    Dimension       int    `json:"dimension"`
    MaxTokens       int    `json:"max_tokens"`
    QueryPrefix     string `json:"query_prefix,omitempty"`
    DocumentPrefix  string `json:"document_prefix,omitempty"`
    NormalizeOutput bool   `json:"normalize_output"`
}
```

---

## 5. Build, Test, and Development Workflow

### Building

```bash
# Build the binary
golang build -o sem ./cmd/sem

# Build with version info
golang build -ldflags "-X main.version=0.1.0" -o sem ./cmd/sem
```

### Running

```bash
# Initialize sem
./sem init

# Add a source
./sem source add ~/my-notes --name notes

# List sources
./sem source list

# Build the index
./sem index

# Search
./sem search "semantic search"
./sem search "semantic search" --json
./sem search "semantic search" --limit 20
```

### Testing

```bash
# Run all tests
golang test ./...

# Run tests with verbose output
golang test -v ./...

# Run tests for a specific package
golang test ./internal/chunk/...

# Run a specific test
golang test -run TestBuild ./internal/chunk/...
```

### Dependency Management

```bash
# Tidy dependencies (remove unused, add missing)
golang mod tidy

# Update all dependencies
golang get -u ./...

# View dependencies
golang list -m all
```

### Development Tips

#### Project Structure Navigation

```
sem/
├── cmd/sem/main.go       # Start here for entry point
├── internal/
│   ├── app/              # App initialization
│   ├── cli/              # CLI commands
│   ├── config/           # Configuration
│   ├── indexer/          # Main indexing logic
│   ├── scan/             # File scanning
│   ├── chunk/            # Text chunking
│   ├── embed/            # Embeddings
│   ├── storage/          # Data storage
│   ├── output/           # Output formatting
│   ├── errs/             # Error handling
│   └── source/           # Source management
└── docs/                 # Documentation
```

#### Debugging with Print Statements

```go
// Simple debug output
fmt.Fprintf(os.Stderr, "DEBUG: processing %s\n", path)

// Use in any function
log.Printf("Processing %d files", len(files))
```

#### Checking for Race Conditions

```bash
golang test -race ./...
```

#### Code Formatting

```bash
# Format all code
golang fmt ./...

# Or use gofmt directly
gofmt -w .
```

#### Linting

```bash
# Install golangci-lint if not installed
brew install golangci-lint

# Run linters
golangci-lint run
```

### Common Issues and Solutions

| Issue | Solution |
|-------|----------|
| `sem is not initialized` | Run `./sem init` first |
| `no sources configured` | Run `./sem source add <path>` |
| `unknown embedding mode` | Use one of: light, balanced, quality, nomic |
| `config validation error` | Check `~/.sem/config.toml` for invalid values |
| Build fails with import errors | Run `golang mod tidy` |

### File Locations

| File | Location |
|------|----------|
| Config | `~/.sem/config.toml` |
| Bundles | `~/.sem/bundles/<name>/` |
| Vector cache | `~/.sem/backends/lancedb/` |
| Models | `~/.sem/models/` |

---

## Quick Reference

### Key Files to Know

| Want to... | Edit this file |
|------------|----------------|
| Add a CLI command | `internal/cli/<command>.go` + `internal/cli/root.go` |
| Change chunking | `internal/chunk/chunker.go` |
| Add embedding model | `internal/embed/model.go` |
| Change output format | `internal/output/format.go` |
| Add config option | `internal/config/config.go` + `validate.go` |
| Fix error messages | `internal/errs/kinds.go` |
| Understand indexing | `internal/indexer/indexer.go` |

### Useful Commands

```bash
# Quick development cycle
golang build -o sem ./cmd/sem && ./sem search "test"

# Check for issues
golang vet ./...

# See dependency tree
golang mod graph

# Clean build cache
golang clean -cache
```

---

## Further Reading

- [Go by Example](https://gobyexample.com/) - Practical Go examples
- [Cobra Documentation](https://github.com/spf13/cobra) - CLI framework
- [Viper Documentation](https://github.com/spf13/viper) - Configuration management
- [Parquet Format](https://parquet.apache.org/docs/) - Columnar storage format
- [LanceDB](https://lancedb.github.io/lancedb/) - Vector database
