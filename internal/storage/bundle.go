package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/parquet-go/parquet-go"

	"sem/internal/chunk"
	"sem/internal/config"
	"sem/internal/embed"
	"sem/internal/errs"
)

const (
	chunksFile     = "chunks.parquet"
	embeddingsFile = "embeddings.parquet"
	manifestFile   = "manifest.json"
	modelFile      = "model.json"
)

type EmbeddingRecord struct {
	ChunkID string    `json:"chunk_id" parquet:"chunk_id"`
	Vector  []float32 `json:"vector" parquet:"vector"`
}

type Manifest struct {
	Version        string                `json:"version"`
	BundleName     string                `json:"bundle_name"`
	EmbeddingModel embed.ModelSpec       `json:"embedding_model"`
	IndexedAt      time.Time             `json:"indexed_at"`
	SourceCount    int                   `json:"source_count"`
	FileCount      int                   `json:"file_count"`
	ChunkCount     int                   `json:"chunk_count"`
	EmbeddingCount int                   `json:"embedding_count"`
	Sources        []config.SourceConfig `json:"sources"`
}

type Bundle struct {
	Dir string
}

func NewBundle(dir string) Bundle {
	return Bundle{Dir: dir}
}

func Initialize(bundleDir, bundleName string, model embed.ModelSpec) error {
	bundle := NewBundle(bundleDir)
	manifest := Manifest{
		Version:        "0.1.0-stage1",
		BundleName:     bundleName,
		EmbeddingModel: model,
		IndexedAt:      time.Time{},
		Sources:        []config.SourceConfig{},
	}
	return bundle.WriteMetadata(manifest, model)
}

func (b Bundle) Write(ctx context.Context, chunks []chunk.Record, vectors []EmbeddingRecord, manifest Manifest, model embed.ModelSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		return fmt.Errorf("create bundle directory: %w", err)
	}
	if err := writeParquet(filepath.Join(b.Dir, chunksFile), chunks); err != nil {
		return fmt.Errorf("write chunks bundle: %w", err)
	}
	if err := writeParquet(filepath.Join(b.Dir, embeddingsFile), vectors); err != nil {
		return fmt.Errorf("write embeddings bundle: %w", err)
	}
	if err := b.WriteMetadata(manifest, model); err != nil {
		return err
	}
	return nil
}

func (b Bundle) LoadChunks() ([]chunk.Record, error) {
	path := filepath.Join(b.Dir, chunksFile)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, errs.ErrIndexNotFound
	}
	rows, err := readParquet[chunk.Record](path)
	if err != nil {
		return nil, fmt.Errorf("read chunks bundle: %w", err)
	}
	return rows, nil
}

func (b Bundle) LoadEmbeddings() ([]EmbeddingRecord, error) {
	path := filepath.Join(b.Dir, embeddingsFile)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, errs.ErrIndexNotFound
	}
	rows, err := readParquet[EmbeddingRecord](path)
	if err != nil {
		return nil, fmt.Errorf("read embeddings bundle: %w", err)
	}
	return rows, nil
}

func (b Bundle) LoadModel() (embed.ModelSpec, error) {
	data, err := os.ReadFile(filepath.Join(b.Dir, modelFile))
	if errors.Is(err, os.ErrNotExist) {
		return embed.ModelSpec{}, errs.ErrIndexNotFound
	}
	if err != nil {
		return embed.ModelSpec{}, fmt.Errorf("read model metadata: %w", err)
	}

	var model embed.ModelSpec
	if err := json.Unmarshal(data, &model); err != nil {
		return embed.ModelSpec{}, fmt.Errorf("decode model metadata: %w", err)
	}
	return model, nil
}

func (b Bundle) LoadManifest() (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(b.Dir, manifestFile))
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, errs.ErrIndexNotFound
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return manifest, nil
}

// WriteMetadata writes the manifest and model files to the bundle directory.
func (b Bundle) WriteMetadata(manifest Manifest, model embed.ModelSpec) error {
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	modelData, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		return fmt.Errorf("encode model metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(b.Dir, manifestFile), manifestData, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(b.Dir, modelFile), modelData, 0o644); err != nil {
		return fmt.Errorf("write model metadata: %w", err)
	}
	return nil
}

func writeParquet[T any](path string, rows []T) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := parquet.NewGenericWriter[T](file)
	if len(rows) > 0 {
		if _, err := writer.Write(rows); err != nil {
			return err
		}
	}
	return writer.Close()
}

func readParquet[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := parquet.NewGenericReader[T](file)
	defer reader.Close()

	out := make([]T, 0, 128)
	for {
		batch := make([]T, 128)
		n, err := reader.Read(batch)
		if n > 0 {
			out = append(out, batch[:n]...)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// RemoveChunks removes chunks and their corresponding embeddings by chunk ID.
// It loads existing data, filters out the specified chunk IDs, and rewrites the Parquet files.
func (b Bundle) RemoveChunks(ctx context.Context, chunkIDs []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Build a set for O(1) lookup
	idSet := make(map[string]struct{}, len(chunkIDs))
	for _, id := range chunkIDs {
		idSet[id] = struct{}{}
	}

	// Load existing chunks, filter, and rewrite
	chunksPath := filepath.Join(b.Dir, chunksFile)
	var filteredChunks []chunk.Record
	if _, err := os.Stat(chunksPath); err == nil {
		existingChunks, err := readParquet[chunk.Record](chunksPath)
		if err != nil {
			return fmt.Errorf("read existing chunks: %w", err)
		}
		filteredChunks = make([]chunk.Record, 0, len(existingChunks))
		for _, c := range existingChunks {
			if _, found := idSet[c.ID]; !found {
				filteredChunks = append(filteredChunks, c)
			}
		}
	}

	// Load existing embeddings, filter, and rewrite
	embeddingsPath := filepath.Join(b.Dir, embeddingsFile)
	var filteredEmbeddings []EmbeddingRecord
	if _, err := os.Stat(embeddingsPath); err == nil {
		existingEmbeddings, err := readParquet[EmbeddingRecord](embeddingsPath)
		if err != nil {
			return fmt.Errorf("read existing embeddings: %w", err)
		}
		filteredEmbeddings = make([]EmbeddingRecord, 0, len(existingEmbeddings))
		for _, e := range existingEmbeddings {
			if _, found := idSet[e.ChunkID]; !found {
				filteredEmbeddings = append(filteredEmbeddings, e)
			}
		}
	}

	// Rewrite the Parquet files
	if err := writeParquet(chunksPath, filteredChunks); err != nil {
		return fmt.Errorf("write filtered chunks: %w", err)
	}
	if err := writeParquet(embeddingsPath, filteredEmbeddings); err != nil {
		return fmt.Errorf("write filtered embeddings: %w", err)
	}

	// Update the manifest with new counts
	manifest, err := b.LoadManifest()
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	manifest.ChunkCount = len(filteredChunks)
	manifest.EmbeddingCount = len(filteredEmbeddings)
	manifest.IndexedAt = time.Now()

	model, err := b.LoadModel()
	if err != nil {
		return fmt.Errorf("load model: %w", err)
	}

	if err := b.WriteMetadata(manifest, model); err != nil {
		return fmt.Errorf("write updated metadata: %w", err)
	}

	return nil
}

// Merge adds new chunks and embeddings to the existing bundle data.
// It loads existing data, appends the new records, and rewrites the Parquet files.
func (b Bundle) Merge(ctx context.Context, newChunks []chunk.Record, newVectors []EmbeddingRecord, manifest Manifest, model embed.ModelSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Load existing chunks (if file exists) and append new ones
	chunksPath := filepath.Join(b.Dir, chunksFile)
	var allChunks []chunk.Record
	if _, err := os.Stat(chunksPath); err == nil {
		existingChunks, err := readParquet[chunk.Record](chunksPath)
		if err != nil {
			return fmt.Errorf("read existing chunks: %w", err)
		}
		allChunks = existingChunks
	}
	allChunks = append(allChunks, newChunks...)

	// Load existing embeddings (if file exists) and append new ones
	embeddingsPath := filepath.Join(b.Dir, embeddingsFile)
	var allEmbeddings []EmbeddingRecord
	if _, err := os.Stat(embeddingsPath); err == nil {
		existingEmbeddings, err := readParquet[EmbeddingRecord](embeddingsPath)
		if err != nil {
			return fmt.Errorf("read existing embeddings: %w", err)
		}
		allEmbeddings = existingEmbeddings
	}
	allEmbeddings = append(allEmbeddings, newVectors...)

	// Rewrite the Parquet files with combined data
	if err := writeParquet(chunksPath, allChunks); err != nil {
		return fmt.Errorf("write combined chunks: %w", err)
	}
	if err := writeParquet(embeddingsPath, allEmbeddings); err != nil {
		return fmt.Errorf("write combined embeddings: %w", err)
	}

	// Update manifest counts to reflect actual totals
	manifest.ChunkCount = len(allChunks)
	manifest.EmbeddingCount = len(allEmbeddings)

	// Write updated manifest and model
	if err := b.WriteMetadata(manifest, model); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return nil
}
