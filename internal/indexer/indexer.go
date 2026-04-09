package indexer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"

	"sem/internal/app"
	"sem/internal/chunk"
	"sem/internal/config"
	"sem/internal/embed"
	"sem/internal/errs"
	"sem/internal/log"
	"sem/internal/scan"
	"sem/internal/storage"
)

type Result struct {
	SourceCount int
	FileCount   int
	ChunkCount  int
	Duration    time.Duration
	Model       embed.ModelSpec
}

// SyncResult extends Result with incremental sync details.
type SyncResult struct {
	Result
	NewFiles     int
	ChangedFiles int
	DeletedFiles int
}

// ProgressCallbacks provides hooks for indexing progress updates.
// Each callback receives the current count and total count for that phase.
type ProgressCallbacks struct {
	OnScanStart     func(total int)
	OnScanProgress  func(current, total int)
	OnChunkStart    func(total int)
	OnChunkProgress func(current, total int)
	OnEmbedStart    func(total int)
	OnEmbedProgress func(current, total int)
	OnWriteStart    func(total int)
	OnWriteProgress func(current, total int)
}

// shouldShowProgress returns true if progress bars should be displayed.
// Progress bars are disabled when:
// - verbose mode is enabled (conflicts with debug logging on stderr)
// - stderr is not a TTY (piped output)
func shouldShowProgress(verbose bool) bool {
	if verbose {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// newProgressBar creates a progress bar with standard options.
func newProgressBar(total int, description string) *progressbar.ProgressBar {
	return progressbar.NewOptions(total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
}

// Run performs full or incremental indexing based on the full flag.
// If full is true, it rebuilds the entire index from scratch.
// If full is false, it performs incremental sync based on file changes.
func Run(ctx context.Context, paths app.Paths, cfg config.Config, sourceName string, full bool, logger *log.Logger, progress *ProgressCallbacks) (SyncResult, error) {
	started := time.Now()
	sources := enabledSources(cfg.Sources, sourceName)
	if len(sources) == 0 {
		if sourceName != "" {
			return SyncResult{}, errs.ErrSourceNotFound
		}
		return SyncResult{}, errs.ErrNoSources
	}
	logger.Debug("indexing %d sources", len(sources))

	bundleDir := paths.BundleDir(cfg.General.DefaultBundle)
	bundle := storage.NewBundle(bundleDir)
	store := storage.NewStore(paths.LanceDBDir)

	// Load previous state to determine if we can do incremental
	state, err := LoadState(bundleDir)
	if err != nil {
		return SyncResult{}, fmt.Errorf("load state: %w", err)
	}

	// Determine if we need a full rebuild
	service, err := embed.NewServiceWithModelDir(cfg.Embedding.Mode, cfg.Embedding.ModelCacheDir)
	if err != nil {
		return SyncResult{}, fmt.Errorf("initialize embedder: %w", err)
	}
	defer service.Close()
	currentModel := service.Model()
	logger.Debug("embedding model: %s (%s)", currentModel.Mode, currentModel.Name)

	// Check for conditions that force a full rebuild
	if !full && len(state.Files) > 0 {
		// Check if embedding mode changed
		if state.EmbeddingMode != "" && state.EmbeddingMode != string(currentModel.Mode) {
			full = true
		}
		// Check if chunking config changed
		currentChunkingHash := ChunkingConfigHash(cfg.Chunking)
		if state.ChunkingHash != "" && state.ChunkingHash != currentChunkingHash {
			full = true
		}
	}

	// If no previous state exists, do a full rebuild
	if len(state.Files) == 0 {
		full = true
	}

	if full {
		return runFullRebuild(ctx, service, bundle, store, sources, cfg, state, started, logger, progress)
	}

	return runIncrementalSync(ctx, service, bundle, store, sources, cfg, state, sourceName, started, logger, progress)
}

// runFullRebuild performs a complete rebuild of the index.
func runFullRebuild(
	ctx context.Context,
	service *embed.Service,
	bundle storage.Bundle,
	store storage.Store,
	sources []config.SourceConfig,
	cfg config.Config,
	state BundleState,
	started time.Time,
	logger *log.Logger,
	progress *ProgressCallbacks,
) (SyncResult, error) {
	// Phase 1: Scan all documents
	if progress != nil && progress.OnScanStart != nil {
		progress.OnScanStart(len(sources)) // We don't know file count yet
	}

	documents := make([]scan.FileDocument, 0, 256)
	for _, src := range sources {
		docs, err := scan.ScanSource(ctx, src, cfg.Ignore)
		if err != nil {
			return SyncResult{}, err
		}
		documents = append(documents, docs...)
		if progress != nil && progress.OnScanProgress != nil {
			progress.OnScanProgress(len(documents), len(documents))
		}
	}
	logger.Debug("scanned %d files from %d sources", len(documents), len(sources))

	// Phase 2: Chunk all documents
	if progress != nil && progress.OnChunkStart != nil {
		progress.OnChunkStart(len(documents))
	}

	records, err := chunk.Build(ctx, documents, cfg.Chunking)
	if err != nil {
		return SyncResult{}, fmt.Errorf("chunk files: %w", err)
	}
	if progress != nil && progress.OnChunkProgress != nil {
		progress.OnChunkProgress(len(documents), len(documents))
	}
	logger.Debug("produced %d chunks", len(records))

	// Phase 3: Embed all chunks
	if progress != nil && progress.OnEmbedStart != nil {
		progress.OnEmbedStart(len(records))
	}

	texts := make([]string, 0, len(records))
	for _, record := range records {
		texts = append(texts, record.Content)
	}

	embedStart := time.Now()
	var onEmbedProgress embed.EmbedProgress
	if progress != nil && progress.OnEmbedProgress != nil {
		onEmbedProgress = progress.OnEmbedProgress
	}
	vectors, err := service.EmbedDocuments(ctx, texts, onEmbedProgress)
	if err != nil {
		return SyncResult{}, fmt.Errorf("embed documents: %w", err)
	}
	logger.Debug("embedding %d vectors took %s", len(vectors), time.Since(embedStart))

	// Create embedding records
	vectorRecords := make([]storage.EmbeddingRecord, 0, len(vectors))
	for i, vector := range vectors {
		vectorRecords = append(vectorRecords, storage.EmbeddingRecord{
			ChunkID: records[i].ID,
			Vector:  vector,
		})
	}

	// Phase 4: Write bundle
	if progress != nil && progress.OnWriteStart != nil {
		progress.OnWriteStart(3) // manifest + state + cache
	}

	// Build new state
	newState := buildState(documents, records, cfg)

	// Write bundle (full overwrite)
	manifest := storage.Manifest{
		Version:        "0.2.0",
		BundleName:     cfg.General.DefaultBundle,
		EmbeddingModel: service.Model(),
		IndexedAt:      time.Now().UTC(),
		SourceCount:    len(sources),
		FileCount:      len(documents),
		ChunkCount:     len(records),
		EmbeddingCount: len(vectorRecords),
		Sources:        sources,
	}
	if err := bundle.Write(ctx, records, vectorRecords, manifest, service.Model()); err != nil {
		return SyncResult{}, err
	}
	if progress != nil && progress.OnWriteProgress != nil {
		progress.OnWriteProgress(1, 3)
	}

	// Save state
	if err := SaveState(bundle.Dir, newState); err != nil {
		return SyncResult{}, fmt.Errorf("save state: %w", err)
	}
	if progress != nil && progress.OnWriteProgress != nil {
		progress.OnWriteProgress(2, 3)
	}

	// Rebuild vector cache
	if err := store.RebuildIndex(ctx, vectorRecords); err != nil {
		return SyncResult{}, fmt.Errorf("rebuild vector cache: %w", err)
	}
	if progress != nil && progress.OnWriteProgress != nil {
		progress.OnWriteProgress(3, 3)
	}

	return SyncResult{
		Result: Result{
			SourceCount: len(sources),
			FileCount:   len(documents),
			ChunkCount:  len(records),
			Duration:    time.Since(started),
			Model:       service.Model(),
		},
		NewFiles:     len(documents),
		ChangedFiles: 0,
		DeletedFiles: 0,
	}, nil
}

// runIncrementalSync performs incremental sync based on file changes.
func runIncrementalSync(
	ctx context.Context,
	service *embed.Service,
	bundle storage.Bundle,
	store storage.Store,
	sources []config.SourceConfig,
	cfg config.Config,
	state BundleState,
	sourceName string,
	started time.Time,
	logger *log.Logger,
	progress *ProgressCallbacks,
) (SyncResult, error) {
	// Phase 1: Scan all enabled sources
	if progress != nil && progress.OnScanStart != nil {
		progress.OnScanStart(len(sources))
	}

	documents := make([]scan.FileDocument, 0, 256)
	for _, src := range sources {
		docs, err := scan.ScanSource(ctx, src, cfg.Ignore)
		if err != nil {
			return SyncResult{}, err
		}
		documents = append(documents, docs...)
		if progress != nil && progress.OnScanProgress != nil {
			progress.OnScanProgress(len(documents), len(documents))
		}
	}

	// Build current files map
	currentFiles := make(map[string]scan.FileDocument, len(documents))
	for _, doc := range documents {
		key := makeFileKey(doc.SourceName, doc.RelPath)
		currentFiles[key] = doc
	}

	// Compute diff
	diff := Diff(state, currentFiles)
	logger.Debug("diff: %d new, %d changed, %d deleted", len(diff.NewFiles), len(diff.ChangedFiles), len(diff.DeletedFiles))

	// If nothing changed, return early
	if len(diff.NewFiles) == 0 && len(diff.ChangedFiles) == 0 && len(diff.DeletedFiles) == 0 {
		return SyncResult{
			Result: Result{
				SourceCount: len(sources),
				FileCount:   len(documents),
				ChunkCount:  len(state.Files), // Approximate
				Duration:    time.Since(started),
				Model:       service.Model(),
			},
			NewFiles:     0,
			ChangedFiles: 0,
			DeletedFiles: 0,
		}, nil
	}

	// Collect chunk IDs to remove
	var chunkIDsToRemove []string

	// If --source is specified, remove all chunks from that source
	if sourceName != "" {
		for key, entry := range state.Files {
			if strings.HasPrefix(key, sourceName+"|") {
				chunkIDsToRemove = append(chunkIDsToRemove, entry.ChunkIDs...)
			}
		}
	} else {
		// Otherwise, remove chunks from deleted and changed files
		for _, key := range diff.DeletedFiles {
			if entry, ok := state.Files[key]; ok {
				chunkIDsToRemove = append(chunkIDsToRemove, entry.ChunkIDs...)
			}
		}
		for _, key := range diff.ChangedFiles {
			if entry, ok := state.Files[key]; ok {
				chunkIDsToRemove = append(chunkIDsToRemove, entry.ChunkIDs...)
			}
		}
	}

	// Remove chunks from bundle if any
	if len(chunkIDsToRemove) > 0 {
		if err := bundle.RemoveChunks(ctx, chunkIDsToRemove); err != nil {
			return SyncResult{}, fmt.Errorf("remove chunks: %w", err)
		}
	}

	// Process new and changed files
	var filesToProcess []scan.FileDocument

	if sourceName != "" {
		// For --source, process all files from that source
		for _, doc := range documents {
			if doc.SourceName == sourceName {
				filesToProcess = append(filesToProcess, doc)
			}
		}
	} else {
		// Otherwise, process only new and changed files
		for _, key := range diff.NewFiles {
			if doc, ok := currentFiles[key]; ok {
				filesToProcess = append(filesToProcess, doc)
			}
		}
		for _, key := range diff.ChangedFiles {
			if doc, ok := currentFiles[key]; ok {
				filesToProcess = append(filesToProcess, doc)
			}
		}
	}

	var newRecords []chunk.Record
	var newVectors []storage.EmbeddingRecord

	if len(filesToProcess) > 0 {
		// Phase 2: Chunk files to process
		if progress != nil && progress.OnChunkStart != nil {
			progress.OnChunkStart(len(filesToProcess))
		}

		records, err := chunk.Build(ctx, filesToProcess, cfg.Chunking)
		if err != nil {
			return SyncResult{}, fmt.Errorf("chunk files: %w", err)
		}
		newRecords = records
		if progress != nil && progress.OnChunkProgress != nil {
			progress.OnChunkProgress(len(filesToProcess), len(filesToProcess))
		}

		// Phase 3: Embed chunks
		if len(records) > 0 {
			if progress != nil && progress.OnEmbedStart != nil {
				progress.OnEmbedStart(len(records))
			}

			texts := make([]string, 0, len(records))
			for _, record := range records {
				texts = append(texts, record.Content)
			}

			embedStart := time.Now()
			var onEmbedProgress embed.EmbedProgress
			if progress != nil && progress.OnEmbedProgress != nil {
				onEmbedProgress = progress.OnEmbedProgress
			}
			vectors, err := service.EmbedDocuments(ctx, texts, onEmbedProgress)
			if err != nil {
				return SyncResult{}, fmt.Errorf("embed documents: %w", err)
			}
			logger.Debug("embedding %d vectors took %s", len(vectors), time.Since(embedStart))

			for i, vector := range vectors {
				newVectors = append(newVectors, storage.EmbeddingRecord{
					ChunkID: records[i].ID,
					Vector:  vector,
				})
			}
		}
	}

	// Phase 4: Merge into bundle
	if progress != nil && progress.OnWriteStart != nil {
		progress.OnWriteStart(3) // manifest + state + cache
	}

	manifest, err := bundle.LoadManifest()
	if err != nil {
		// If no manifest exists, create a new one
		manifest = storage.Manifest{
			Version:        "0.2.0",
			BundleName:     cfg.General.DefaultBundle,
			EmbeddingModel: service.Model(),
			Sources:        sources,
		}
	}

	// Update manifest
	manifest.Version = "0.2.0"
	manifest.EmbeddingModel = service.Model()
	manifest.IndexedAt = time.Now().UTC()
	manifest.SourceCount = len(sources)
	manifest.FileCount = len(documents)
	// ChunkCount and EmbeddingCount will be updated by Merge based on actual totals

	if len(newRecords) > 0 {
		if err := bundle.Merge(ctx, newRecords, newVectors, manifest, service.Model()); err != nil {
			return SyncResult{}, fmt.Errorf("merge chunks: %w", err)
		}
	} else {
		// No new records, just update metadata
		if err := bundle.WriteMetadata(manifest, service.Model()); err != nil {
			return SyncResult{}, fmt.Errorf("update metadata: %w", err)
		}
	}
	if progress != nil && progress.OnWriteProgress != nil {
		progress.OnWriteProgress(1, 3)
	}

	// Update state
	newState := updateState(state, currentFiles, newRecords, diff, sourceName)
	if err := SaveState(bundle.Dir, newState); err != nil {
		return SyncResult{}, fmt.Errorf("save state: %w", err)
	}
	if progress != nil && progress.OnWriteProgress != nil {
		progress.OnWriteProgress(2, 3)
	}

	// Rebuild vector cache (full rebuild for simplicity)
	allEmbeddings, err := bundle.LoadEmbeddings()
	if err != nil {
		return SyncResult{}, fmt.Errorf("load embeddings for cache rebuild: %w", err)
	}
	if err := store.RebuildIndex(ctx, allEmbeddings); err != nil {
		return SyncResult{}, fmt.Errorf("rebuild vector cache: %w", err)
	}
	if progress != nil && progress.OnWriteProgress != nil {
		progress.OnWriteProgress(3, 3)
	}

	// Calculate final stats
	finalManifest, err := bundle.LoadManifest()
	if err != nil {
		finalManifest = manifest
	}

	return SyncResult{
		Result: Result{
			SourceCount: len(sources),
			FileCount:   len(documents),
			ChunkCount:  finalManifest.ChunkCount,
			Duration:    time.Since(started),
			Model:       service.Model(),
		},
		NewFiles:     len(diff.NewFiles),
		ChangedFiles: len(diff.ChangedFiles),
		DeletedFiles: len(diff.DeletedFiles),
	}, nil
}

// buildState creates a new BundleState from documents and records.
func buildState(documents []scan.FileDocument, records []chunk.Record, cfg config.Config) BundleState {
	state := BundleState{
		Version:       "1",
		EmbeddingMode: "", // Will be set by caller
		ChunkingHash:  ChunkingConfigHash(cfg.Chunking),
		Files:         make(map[string]FileEntry),
	}

	// Group records by file
	recordsByFile := make(map[string][]chunk.Record)
	for _, r := range records {
		key := makeFileKey(r.SourceName, r.RelPath)
		recordsByFile[key] = append(recordsByFile[key], r)
	}

	// Build file entries
	for _, doc := range documents {
		key := makeFileKey(doc.SourceName, doc.RelPath)
		fileRecords := recordsByFile[key]
		chunkIDs := make([]string, len(fileRecords))
		for i, r := range fileRecords {
			chunkIDs[i] = r.ID
		}

		state.Files[key] = FileEntry{
			ContentHash: contentHash(doc.Content),
			ModifiedAt:  doc.ModifiedAt,
			ByteSize:    doc.ByteSize,
			ChunkIDs:    chunkIDs,
		}
	}

	return state
}

// updateState updates the existing state with changes from incremental sync.
func updateState(
	state BundleState,
	currentFiles map[string]scan.FileDocument,
	newRecords []chunk.Record,
	diff DiffResult,
	sourceName string,
) BundleState {
	newState := BundleState{
		Version:       state.Version,
		EmbeddingMode: state.EmbeddingMode,
		ChunkingHash:  state.ChunkingHash,
		Files:         make(map[string]FileEntry, len(currentFiles)),
	}

	// Copy existing state files (excluding deleted)
	for key, entry := range state.Files {
		// Skip deleted files
		isDeleted := false
		for _, deletedKey := range diff.DeletedFiles {
			if key == deletedKey {
				isDeleted = true
				break
			}
		}
		if !isDeleted {
			newState.Files[key] = entry
		}
	}

	// If --source is used, remove all entries for that source (they'll be re-added)
	if sourceName != "" {
		for key := range newState.Files {
			if strings.HasPrefix(key, sourceName+"|") {
				delete(newState.Files, key)
			}
		}
	} else {
		// Remove changed files (they'll be re-added)
		for _, key := range diff.ChangedFiles {
			delete(newState.Files, key)
		}
	}

	// Group new records by file
	recordsByFile := make(map[string][]chunk.Record)
	for _, r := range newRecords {
		key := makeFileKey(r.SourceName, r.RelPath)
		recordsByFile[key] = append(recordsByFile[key], r)
	}

	// Add new/updated file entries
	for key, doc := range currentFiles {
		// Skip if not a new or changed file (unless --source was used)
		if sourceName == "" {
			isNewOrChanged := false
			for _, k := range diff.NewFiles {
				if key == k {
					isNewOrChanged = true
					break
				}
			}
			if !isNewOrChanged {
				for _, k := range diff.ChangedFiles {
					if key == k {
						isNewOrChanged = true
						break
					}
				}
			}
			if !isNewOrChanged {
				continue
			}
		}

		fileRecords := recordsByFile[key]
		chunkIDs := make([]string, len(fileRecords))
		for i, r := range fileRecords {
			chunkIDs[i] = r.ID
		}

		newState.Files[key] = FileEntry{
			ContentHash: contentHash(doc.Content),
			ModifiedAt:  doc.ModifiedAt,
			ByteSize:    doc.ByteSize,
			ChunkIDs:    chunkIDs,
		}
	}

	return newState
}

func enabledSources(sources []config.SourceConfig, sourceName string) []config.SourceConfig {
	active := make([]config.SourceConfig, 0, len(sources))
	for _, src := range sources {
		if !src.Enabled {
			continue
		}
		if sourceName != "" && src.Name != sourceName {
			continue
		}
		active = append(active, src)
	}
	return active
}
