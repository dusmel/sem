# Final Plan

## 1) Product direction

Build a **local-first semantic search tool** for:

- repos
- markdown notes
- docs
- knowledge vaults

It should be:

- **fast**
- **lightweight**
- **terminal-first**
- **nice to use daily**
- **portable**
- **AI-tool friendly**
- **not locked into one backend**

---

# 2) Core architecture

## Rule
**Portable bundle is the source of truth**
**Backend is a replaceable runtime cache**

## Default runtime backend
- **LanceDB**

## Why
This keeps the first version:
- local
- fast
- lightweight
- no Docker
- no always-running service

Later, the same canonical bundle can be imported into:
- **Qdrant**
- **Meilisearch**

So the plan still supports **all 5 external targets**, but in stages:

### AI/tool integrations
1. **OpenCode**
2. **KiloCode**
3. **Claude Code**

### Optional backend targets
4. **Meilisearch**
5. **Qdrant**

---

# 3) Revised implementation stack

Since you want this to be **really fast and light**, the stack should move to a **native binary**.

## Recommended v1 language
**Rust**

## Why Rust over Zig for v1
Rust is the better practical choice for this project because it gives you:

- strong ecosystem for CLI/TUI
- better library support for:
  - embeddings
  - Parquet/Arrow
  - file watching
  - process integration
  - JSON/schema handling
- easier path to LanceDB integration
- fast execution and low memory
- simpler path to shipping a stable tool

## Zig
Zig is still a valid option later, but for **v1** it will likely mean:
- more custom plumbing
- slower implementation
- more integration work

### Recommendation
- **Use Rust for v1**
- keep **Zig** as an optional future rewrite/optimization path only if needed

---

## Terminal framework
Use **OpenTUI** for the TUI layer.

## CLI approach
Single native binary:
- fast CLI subcommands
- TUI mode in the same app

### Suggested native stack
- **Rust**
- **OpenTUI** for TUI
- **clap** or equivalent for CLI parsing
- **serde** for config/data
- **Parquet/Arrow** for bundle storage
- **LanceDB** adapter
- **ripgrep** for exact search
- **notify**-style watcher for optional watch mode
- local embedding runtime adapter

---

# 4) Embedding modes

Use **4 local embedding modes**.

## A. `balanced` — default
**Model:** `BAAI/bge-small-en-v1.5`

### Why default
Best overall fit for your current **M1 / 8 GB**:
- good semantic quality
- reasonably fast
- moderate memory use
- works well for mixed notes + docs + code

### Use when
- this should be the default mode for most users

---

## B. `light`
**Model:** `all-MiniLM-L6-v2`

### Best for
- fastest indexing
- lowest RAM use
- lowest system impact

### Tradeoff
- weakest semantic quality of the set
- less reliable on nuanced searches

### Use when
- machine is under pressure
- corpus is large
- exact/hybrid search is doing most of the work

---

## C. `quality`
**Model:** `BAAI/bge-base-en-v1.5`

### Best for
- stronger semantic ranking
- better retrieval quality
- future default after a hardware upgrade

### Tradeoff
- slower
- more RAM
- heavier than `balanced`

### Use when
- quality matters more than speed
- corpus is moderate size
- especially after you upgrade hardware

---

## D. `nomic`
**Model:** `Nomic Embed Text`

### Best for
- strong document/markdown retrieval
- a good alternative semantic profile to BGE
- useful if your corpus leans more notes/docs than code

### Tradeoff
- usually not the lightest option
- likely medium speed / medium RAM
- still more text-oriented than code-specialized

### Code vs markdown
- **Markdown/docs:** strong
- **Code:** okay for concept-level retrieval, but not exact symbol search

### Use when
- you want an alternative to BGE
- your corpus is note-heavy
- you want to compare retrieval feel without going fully “quality” mode

---

## Final embedding recommendation

### Default now
- **`balanced` = BGE Small**

### Additional choices
- **`light` = MiniLM**
- **`quality` = BGE Base**
- **`nomic` = Nomic Embed Text**

---

# 5) Default system behavior

For your current device:

## Defaults
- backend: **LanceDB**
- embedding mode: **balanced**
- search mode: **hybrid**
- watch mode: **off by default**

This gives you:
- good quality
- low friction
- low idle memory
- solid local performance on M1 / 8 GB

---

# 6) Search strategy

## Target default
**Hybrid search**

That means:

- **semantic** search for concepts
- **exact** search for symbols, file names, paths, config keys
- merged/reranked into one result set

## Exact search backend
- **ripgrep**

## Why this matters
Embeddings alone are not enough for code.

### Semantic works well for:
- “where is retry logic handled?”
- “notes about postgres indexing”
- “auth flow in the api”

### Exact works better for:
- `UserRepository`
- `validateJwt`
- `billingWebhook.ts`
- `X-Request-ID`

---

# 7) End-to-end workflow

## Setup
```bash
sem init
sem source add ~/vault --name vault
sem source add ~/code --name repos
```

## First index
```bash
sem index
```

## Search
```bash
sem search "jwt auth flow"
sem search "rate limiting notes" --mode hybrid
sem search "UserRepository" --mode exact
```

## TUI
```bash
sem tui
```

## Update
```bash
sem sync
```

## Optional watch mode
```bash
sem watch
```

## AI/tool usage
```bash
sem search "how is auth implemented" --json
```

## Export/import
```bash
sem export --format bundle --out ~/sem-export
sem import --backend qdrant --from ~/sem-export
sem import --backend meilisearch --from ~/sem-export
```

---

# 8) Storage design

## Rule
**Bundle is truth. Backend is runtime cache.**

## Bundle stores
- chunks
- metadata
- embeddings
- source definitions
- model info
- manifest/version info

## Example layout
```text
~/.sem/
  config.toml
  bundles/
    default/
      manifest.json
      chunks.parquet
      embeddings.parquet
      model.json
  backends/
    lancedb/
```

This keeps the app portable and backend-agnostic.

---

# 9) Stage plan

## Stage 1 — Core local MVP
Build:
- config
- source registry
- file scanning
- ignore rules
- basic chunking
- local embeddings
- LanceDB adapter
- `index`
- `search`
- `--json`

### Result
Useful local semantic search from terminal.

---

## Stage 2 — Practical daily use
Add:
- incremental `sync`
- deterministic IDs
- content hashing
- snippets
- `.gitignore` support
- default skip rules

### Result
Reliable reindexing and low maintenance.

---

## Stage 3 — Good search quality
Add:
- exact file/path search
- exact content search via ripgrep
- hybrid ranking
- repo/source/type filters
- better code chunking

### Result
Strong mixed search across notes and code.

---

## Stage 4 — TUI
Add:
- `sem tui`
- OpenTUI-based interface
- search bar
- results pane
- preview pane
- syntax highlighting
- keyboard navigation
- open in editor

### Result
Polished terminal daily driver.

---

## Stage 5 — AI tool integration ( mostly skills)
Add:
- stable JSON contract
- CLI tool integration
- optional MCP server

### This is where support lands for:
- **OpenCode**
- **KiloCode**
- **Claude Code**

### Result
Same local search system works for both you and agents.

---

## Stage 6 — Optional automation
Add:
- `watch`
- debounced file updates
- partial reindexing
- scheduled sync recipes

### Result
Fresh index when wanted, without permanent background cost.

---

## Stage 7 — Portability and extra backends
Add:
- bundle export/import
- backend adapter interface
- import/export for:
  - **Meilisearch**
  - **Qdrant**

### Result
No lock-in. Same UX, different backend.

---

## Stage 8 — Polish
Add:
- `doctor`
- saved searches
- history
- bookmarks
- improved highlighting
- packaging/distribution

---

# 10) Final command surface

```bash
sem init
sem source add <path> # name takes from repo or vault name on the path
sem source list
sem source remove <name>

sem index
sem sync
sem watch
sem status
sem doctor

sem search "<query>"
sem search "<query>" --mode semantic|exact|hybrid
sem search "<query>" --json

sem tui

sem export --format bundle
sem import --backend lancedb|meilisearch|qdrant
```

---

# 11) Final implementation rules

## Hard rules
- local-first
- native binary
- no Docker required
- no always-on service required
- portable bundle as source of truth
- backend replaceable
- hybrid search is the target default
- local embeddings by default
- watch mode optional
- AI integration built on stable CLI/JSON first

---

# 12) Final recommendation

## Build it like this
- **Rust for tooling and bun typescript or python for rest**
- **OpenTUI**
- **LanceDB**
- **portable Parquet bundle**
- **ripgrep hybrid search**
- **local embeddings**
- **CLI first**
- **TUI second**
- **AI integrations next**
- **Meilisearch/Qdrant later**

## Embedding modes
- `light` → MiniLM
- `balanced` → BGE Small
- `quality` → BGE Base
- `high-quality` → Nomic Embed Text

## Best default for your current machine
- backend: **LanceDB**
- embedding: **balanced**
- search: **hybrid**
- watcher: **off by default**

This is now a better fit for your goals:
- **faster**
- **lighter**
- **native**
- **still portable**
- **still staged**
- and still supports all 5 integrations/targets over time.

If you want, I can next turn this into a **native build spec** for the Rust/OpenTUI version:
- project structure
- crate/module layout
- config schema
- storage schema
- command behavior
- MVP tasks in implementation order.
