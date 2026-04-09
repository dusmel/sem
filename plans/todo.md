# sem — Pending & Deferred Items

> Collected from Stage 1, 2, and 3 docs. Organized by target stage for prioritization.

---

## Stage 3 — Good Search Quality (IN PROGRESS)

> Spec: `plans/phase-3.spec.md`

### Stream 1: Hybrid Search (COMPLETE)

- [x] **Exact search via ripgrep** — shell out to `rg --json` for exact content search
- [x] **Search mode flag** — `--mode semantic|exact|hybrid` on `sem search`, default hybrid
- [x] **Hybrid ranking** — Reciprocal Rank Fusion (RRF, k=60) to merge semantic + exact results
- [ ] **File/path search** — exact matching on file paths, names, extensions
- [ ] **Snippet highlighting** — ANSI highlighting of matching terms in search results

### Stream 2: Better Code Chunking (COMPLETE)

- [x] **Language-aware chunking** — regex-based function/class boundary detection for Go, Python, JS/TS, Rust
- [x] **Markdown heading-aware chunking** — implement `respect_headings` config (defined but not used)
- [x] **Chunk metadata enrichment** — add FunctionName, SectionLevel to chunk metadata

### Stream 3: Filters & Search UX (COMPLETE)

- [x] **`--source` filter** — already partially wired, complete integration
- [x] **`--language` filter** — filter results by programming language
- [x] **`--kind` filter** — filter by file kind (markdown, code, text)
- [x] **`--dir` filter** — filter by subdirectory path (e.g. `notes/`, `src/`)

### Stream 4: Embedding Optimizations

- [x] **4.1 ONNX O1-O4 optimized variants** — download and prefer graph-optimized models (1.5-2x speedup)
- [x] **4.2 Dynamic max_tokens per mode** — use model-specific limits (MiniLM=256, BGE=512) instead of config default
- [x] **4.3 Quantized BGE Small model** — use community INT8 model from `Qdrant/bge-small-en-v1.5-onnx-Q` (5-9x speedup)
- [x] **4.4 Qdrant BGE Base optimized** — use `model_optimized.onnx` from `Qdrant/bge-base-en-v1.5-onnx-Q` for quality mode (~1.5-2x speedup)

### Stream 5: Low-Hanging Fruit

- [x] **5.1 Progress bars** — show indexing progress with count/ETA (`schollz/progressbar/v3`)
- [x] **5.2 `--verbose` flag** — debug logging for search/index operations
- [x] **5.3 Tokenizer accent stripping** — NFD + remove combining marks for better non-ASCII matching
- [x] **5.4 `sem doctor`** — validate environment: ripgrep, ONNX Runtime, model files, config
- [x] **5.5 Snippet highlighting** — ANSI color codes for matched terms in human-readable output

### Stream 6: Integration Tests

- [x] **6.1 E2E workflow tests** — end-to-end: init → source add → index → sync → search

### README

- [x] **R1 Practical usage manual** — simple → complex cases, real examples, filter combos, JSON scripting, embedding modes

---

## Deferred to Later Phases

- [ ] **Nomic BPE tokenizer** — deferred (spec §5.4.4)
- [ ] **`--file-match` flag** — search filenames only, not content
- [ ] **ClassName/Imports metadata** — deferred (spec §5.2.4)
- [ ] **Export script for BGE Base INT8** — dropped (Qdrant optimized model is sufficient)
- [ ] **`[search]` config section** — deferred (spec §5.1.3)

---

## Stage 4 — TUI

- [ ] **`sem tui` command** — OpenTUI-based interface
- [ ] **Search bar** with live results
- [ ] **Results pane** with keyboard navigation
- [ ] **Preview pane** with syntax highlighting
- [ ] **Open in editor** from results

---

## Stage 5 — AI Tool Integration

- [ ] **Stable JSON contract** — versioned, documented JSON output schema for agent consumption
- [ ] **MCP server** — expose search as an MCP tool for OpenCode/KiloCode/Claude Code
- [ ] **CLI tool integration** — documented patterns for agents calling `sem search --json`

---

## Stage 6 — Optional Automation

- [ ] **`sem watch` command** — file watcher with debounced re-indexing
- [ ] **Partial reindexing** — only re-process changed files detected by watcher
- [ ] **Scheduled sync** — cron-like recipes for periodic sync

---

## Stage 7 — Portability & Extra Backends

- [ ] **Bundle export/import** — `sem export --format bundle --out <path>`
- [ ] **Backend adapter interface** — abstract VectorBackend for swappable storage
- [ ] **Meilisearch adapter** — import bundle into Meilisearch
- [ ] **Qdrant adapter** — import bundle into Qdrant

---

## Stage 8 — Polish

- [ ] **Saved searches** — name and recall frequent queries
- [ ] **Search history** — persistent history of queries
- [ ] **Bookmarks** — save/star specific results
- [ ] **Packaging/distribution** — goreleaser, cross-compilation, Homebrew tap
- [ ] **Bundle ONNX Runtime with releases** — users should NOT need to install separately

---

## Cross-Cutting / Unassigned

### Quality & Testing

- [ ] **Model download resume** — resume partial downloads instead of restarting

### Performance

- [ ] **Concurrent embedding** — worker pool with `errgroup` for parallel batch processing
- [ ] **Better performance for non-ARM** — target for embedding generation should not exceed 5-7sec
- [ ] **Append-only Parquet** — avoid full rewrite on every sync; add periodic compaction
- [ ] **Streaming state** — for indexes >100k files, stream state instead of loading all into memory
- [ ] **Real LanceDB** — replace JSON cache with actual LanceDB Go bindings
- [ ] **CoreML execution provider** — use M1 Neural Engine via `AppendExecutionProviderCoreML()`

### UX

- [ ] **Multiple bundles** — config supports it, CLI doesn't expose it
- [ ] **Source-specific ignore** — config supports `exclude_patterns` per source, not fully tested
- [ ] **Staleness detection** — `sem status` should detect changed files since last index and report "stale"
- [ ] **Model download prompt** — confirm before downloading large models (~430MB for quality)

### Architecture Debt

- [ ] **Remove hash-based embedding fallback** — kept for robustness; decide whether to keep or remove
- [ ] **ONNX Runtime auto-download** — download shared library during `sem init` or first use
- [ ] **Error kinds** — `ErrModelNotFound`, `ErrModelDownload`, `ErrONNXRuntime` were planned but not all implemented