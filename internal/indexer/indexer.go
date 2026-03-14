package indexer

import (
	"context"
	"fmt"
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

func Run(ctx context.Context, paths app.Paths, cfg config.Config, sourceName string) (Result, error) {
	started := time.Now()
	sources := enabledSources(cfg.Sources, sourceName)
	if len(sources) == 0 {
		if sourceName != "" {
			return Result{}, errs.ErrSourceNotFound
		}
		return Result{}, errs.ErrNoSources
	}

	service, err := embed.NewService(cfg.Embedding.Mode)
	if err != nil {
		return Result{}, fmt.Errorf("initialize embedder: %w", err)
	}

	documents := make([]scan.FileDocument, 0, 256)
	for _, src := range sources {
		docs, err := scan.ScanSource(ctx, src, cfg.Ignore.DefaultPatterns)
		if err != nil {
			return Result{}, err
		}
		documents = append(documents, docs...)
	}

	records, err := chunk.Build(ctx, documents, cfg.Chunking)
	if err != nil {
		return Result{}, fmt.Errorf("chunk files: %w", err)
	}

	texts := make([]string, 0, len(records))
	for _, record := range records {
		texts = append(texts, record.Content)
	}

	vectors, err := service.EmbedDocuments(ctx, texts)
	if err != nil {
		return Result{}, fmt.Errorf("embed documents: %w", err)
	}

	vectorRecords := make([]storage.EmbeddingRecord, 0, len(vectors))
	for i, vector := range vectors {
		vectorRecords = append(vectorRecords, storage.EmbeddingRecord{
			ChunkID: records[i].ID,
			Vector:  vector,
		})
	}

	bundleDir := paths.BundleDir(cfg.General.DefaultBundle)
	bundle := storage.NewBundle(bundleDir)
	manifest := storage.Manifest{
		Version:        "0.1.0-stage1",
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
		return Result{}, err
	}

	store := storage.NewStore(paths.LanceDBDir)
	if err := store.RebuildIndex(ctx, vectorRecords); err != nil {
		return Result{}, fmt.Errorf("rebuild vector cache: %w", err)
	}

	return Result{
		SourceCount: len(sources),
		FileCount:   len(documents),
		ChunkCount:  len(records),
		Duration:    time.Since(started),
		Model:       service.Model(),
	}, nil
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
