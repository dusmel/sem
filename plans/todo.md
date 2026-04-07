# sem — Pending & Deferred Items

> Collected from Stage 1 and Stage 2 docs. Organized by target stage for prioritization.

---

## Stage 3 — Good Search Quality

### High Priority

- [ ] **Hybrid search** — merge semantic + exact (ripgrep) results with reranking
- [ ] **Exact content search** — ripgrep integration for symbols, file names, config keys
- [ ] **Better code chunking** — AST-aware chunking for Go, Python, JS/TS, Rust
- [ ] **Source/type filters** — `--source`, `--language`, `--kind` flags on search

### Medium Priority

- [ ] **Improved tokenizer** — accent stripping (NFD + remove combining marks), Chinese character handling, BPE support for non-BERT models (nomic uses BPE not WordPiece)
- [ ] **Search mode flag** — `--mode semantic|exact|hybrid` (currently only semantic)
- [ ] **Snippet highlighting** — highlight matching terms in search results

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

- [ ] **`sem doctor`** — health check: ONNX Runtime, model files, bundle integrity, config validity
- [ ] **Saved searches** — name and recall frequent queries
- [ ] **Search history** — persistent history of queries
- [ ] **Bookmarks** — save/star specific results
- [ ] **Improved highlighting** — better snippet formatting and match highlighting
- [ ] **Packaging/distribution** — goreleaser, cross-compilation, Homebrew tap
- [ ] **Bundle ONNX Runtime with releases** — users should NOT need to install separately (explicitly requested)

---

## Cross-Cutting / Unassigned

These don't map to a specific stage but should be addressed when relevant.

### Quality & Testing

- [ ] **Test files** — zero `*_test.go` files exist; add unit tests for core packages (chunk, embed, storage, indexer)
- [ ] **Integration tests** — end-to-end workflow tests: init → source add → index → sync → search
- [ ] **State.json atomic writes** — write to `state.json.tmp` then rename (currently not atomic, risk of corruption on crash)
- [ ] **Model download resume** — resume partial downloads instead of restarting

### Performance

- [ ] **Concurrent embedding** — worker pool with `errgroup` for parallel batch processing
- [ ] **Append-only Parquet** — avoid full rewrite on every sync; add periodic compaction
- [ ] **Streaming state** — for indexes >100k files, stream state instead of loading all into memory
- [ ] **Real LanceDB** — replace JSON cache with actual LanceDB Go bindings (currently using JSON file + brute-force cosine similarity)
- [ ] **Quantized models for balanced/quality/nomic** — only light mode has INT8 ARM64 variant; export or find quantized versions for other modes
- [ ] **CoreML execution provider** — use M1 Neural Engine via `AppendExecutionProviderCoreML()` for faster inference

### UX

- [ ] **Progress bars** — show indexing progress with ETA and throughput
- [ ] **`--verbose` flag** — opt-in debug logging
- [ ] **Multiple bundles** — config supports it, CLI doesn't expose it
- [ ] **Source-specific ignore** — config supports `exclude_patterns` per source, not fully tested
- [ ] **Staleness detection** — `sem status` should detect changed files since last index and report "stale"
- [ ] **Model download prompt** — confirm before downloading large models (~430MB for quality)

### Architecture Debt

- [ ] **Remove hash-based embedding fallback** — phase-2 spec said "remove entirely, no fallback" but we kept it for robustness; decide whether to keep or remove
- [ ] **ONNX Runtime auto-download** — download shared library during `sem init` or first use instead of requiring manual install
- [ ] **Config validation** — `embedding_mode` in `[general]` and `[embedding]` are redundant; consolidate
- [ ] **Error kinds** — `ErrModelNotFound`, `ErrModelDownload`, `ErrONNXRuntime` were planned in spec but not all implemented
