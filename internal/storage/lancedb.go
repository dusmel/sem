package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

const cacheFile = "cache.json"

type SearchHit struct {
	ChunkID string  `json:"chunk_id"`
	Score   float32 `json:"score"`
}

type Store struct {
	Dir string
}

func NewStore(dir string) Store {
	return Store{Dir: dir}
}

func (s Store) RebuildIndex(ctx context.Context, records []EmbeddingRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create vector cache directory: %w", err)
	}
	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("encode vector cache: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, cacheFile), data, 0o644); err != nil {
		return fmt.Errorf("write vector cache: %w", err)
	}
	return nil
}

func (s Store) Search(ctx context.Context, query []float32, limit int) ([]SearchHit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(s.Dir, cacheFile))
	if err != nil {
		return nil, fmt.Errorf("read vector cache: %w", err)
	}

	var records []EmbeddingRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode vector cache: %w", err)
	}

	hits := make([]SearchHit, 0, len(records))
	for _, record := range records {
		hits = append(hits, SearchHit{
			ChunkID: record.ChunkID,
			Score:   cosine(query, record.Vector),
		})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})

	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}

	return hits, nil
}

func cosine(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	var dot, an, bn float64
	for i := 0; i < limit; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		an += av * av
		bn += bv * bv
	}
	if an == 0 || bn == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(an) * math.Sqrt(bn)))
}

// MergeRecords adds new embedding records to the existing vector cache.
// It loads existing records from cache.json (if file exists), appends new ones, and rewrites the cache.
func (s Store) MergeRecords(ctx context.Context, newRecords []EmbeddingRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var allRecords []EmbeddingRecord

	// Load existing records if cache file exists
	cachePath := filepath.Join(s.Dir, cacheFile)
	if data, err := os.ReadFile(cachePath); err == nil {
		if err := json.Unmarshal(data, &allRecords); err != nil {
			return fmt.Errorf("decode existing vector cache: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read vector cache: %w", err)
	}

	// Append new records
	allRecords = append(allRecords, newRecords...)

	// Write combined data back to cache
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create vector cache directory: %w", err)
	}
	data, err := json.Marshal(allRecords)
	if err != nil {
		return fmt.Errorf("encode vector cache: %w", err)
	}
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		return fmt.Errorf("write vector cache: %w", err)
	}

	return nil
}
