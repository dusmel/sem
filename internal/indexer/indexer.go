package indexer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sem/internal/app"
	"sem/internal/chunk"
	"sem/internal/config"
	"sem/internal/embed"
	"sem/internal/errs"
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

// Run performs full or incremental indexing based on the full flag.
// If full is true, it rebuilds the entire index from scratch.
// If full is false, it performs incremental sync based on file changes.
func Run(ctx context.Context, paths app.Paths, cfg config.Config, sourceName string, full bool) (SyncResult, error) {
	started := time.Now()
	sources := enabledSources(cfg.Sources, sourceName)
	if len(sources) == 0 {
		if sourceName != "" {
			return SyncResult{}, errs.ErrSourceNotFound
		}
		return SyncResult{}, errs.ErrNoSources
	}

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
		return runFullRebuild(ctx, service, bundle, store, sources, cfg, state, started)
	}

	return runIncrementalSync(ctx, service, bundle, store, sources, cfg, state, sourceName, started)
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
) (SyncResult, error) {
	// Scan all documents
	documents := make([]scan.FileDocument, 0, 256)
	for _, src := range sources {
		docs, err := scan.ScanSource(ctx, src, cfg.Ignore)
		if err != nil {
			return SyncResult{}, err
		}
		documents = append(documents, docs...)
	}

	// Chunk all documents
	records, err := chunk.Build(ctx, documents, cfg.Chunking)
	if err != nil {
		return SyncResult{}, fmt.Errorf("chunk files: %w", err)
	}

	// Embed all chunks
	texts := make([]string, 0, len(records))
	for _, record := range records {
		texts = append(texts, record.Content)
	}

	vectors, err := service.EmbedDocuments(ctx, texts)
	if err != nil {
		return SyncResult{}, fmt.Errorf("embed documents: %w", err)
	}

	// Create embedding records
	vectorRecords := make([]storage.EmbeddingRecord, 0, len(vectors))
	for i, vector := range vectors {
		vectorRecords = append(vectorRecords, storage.EmbeddingRecord{
			ChunkID: records[i].ID,
			Vector:  vector,
		})
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

	// Save state
	if err := SaveState(bundle.Dir, newState); err != nil {
		return SyncResult{}, fmt.Errorf("save state: %w", err)
	}

	// Rebuild vector cache
	if err := store.RebuildIndex(ctx, vectorRecords); err != nil {
		return SyncResult{}, fmt.Errorf("rebuild vector cache: %w", err)
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
) (SyncResult, error) {
	// Scan all enabled sources
	documents := make([]scan.FileDocument, 0, 256)
	for _, src := range sources {
		docs, err := scan.ScanSource(ctx, src, cfg.Ignore)
		if err != nil {
			return SyncResult{}, err
		}
		documents = append(documents, docs...)
	}

	// Build current files map
	currentFiles := make(map[string]scan.FileDocument, len(documents))
	for _, doc := range documents {
		key := makeFileKey(doc.SourceName, doc.RelPath)
		currentFiles[key] = doc
	}

	// Compute diff
	diff := Diff(state, currentFiles)

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
		// Chunk files to process
		records, err := chunk.Build(ctx, filesToProcess, cfg.Chunking)
		if err != nil {
			return SyncResult{}, fmt.Errorf("chunk files: %w", err)
		}
		newRecords = records

		// Embed chunks
		if len(records) > 0 {
			texts := make([]string, 0, len(records))
			for _, record := range records {
				texts = append(texts, record.Content)
			}

			vectors, err := service.EmbedDocuments(ctx, texts)
			if err != nil {
				return SyncResult{}, fmt.Errorf("embed documents: %w", err)
			}

			for i, vector := range vectors {
				newVectors = append(newVectors, storage.EmbeddingRecord{
					ChunkID: records[i].ID,
					Vector:  vector,
				})
			}
		}
	}

	// Merge new chunks into bundle
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

	// Update state
	newState := updateState(state, currentFiles, newRecords, diff, sourceName)
	if err := SaveState(bundle.Dir, newState); err != nil {
		return SyncResult{}, fmt.Errorf("save state: %w", err)
	}

	// Rebuild vector cache (full rebuild for simplicity)
	allEmbeddings, err := bundle.LoadEmbeddings()
	if err != nil {
		return SyncResult{}, fmt.Errorf("load embeddings for cache rebuild: %w", err)
	}
	if err := store.RebuildIndex(ctx, allEmbeddings); err != nil {
		return SyncResult{}, fmt.Errorf("rebuild vector cache: %w", err)
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
