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
	return bundle.writeMetadata(manifest, model)
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
	if err := b.writeMetadata(manifest, model); err != nil {
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

func (b Bundle) writeMetadata(manifest Manifest, model embed.ModelSpec) error {
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
