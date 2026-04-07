package indexer

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sem/internal/config"
	"sem/internal/scan"
)

// FileEntry tracks per-file metadata for incremental sync.
type FileEntry struct {
	ContentHash string    `json:"content_hash"` // SHA1 of file content
	ModifiedAt  time.Time `json:"modified_at"`  // File modification time
	ByteSize    int64     `json:"byte_size"`    // File size in bytes
	ChunkIDs    []string  `json:"chunk_ids"`    // IDs of chunks from this file
}

// BundleState is the full state file persisted between indexing runs.
type BundleState struct {
	Version       string               `json:"version"`        // Format version, always "1"
	EmbeddingMode string               `json:"embedding_mode"` // Embedding mode used for this state
	ChunkingHash  string               `json:"chunking_hash"`  // Hash of chunking config to detect changes
	Files         map[string]FileEntry `json:"files"`          // Key: "sourceName|relPath"
}

// StateFileName is the name of the state file within the bundle directory.
const StateFileName = "state.json"

// LoadState reads state.json from the bundle directory.
// Returns an empty state (not an error) if the file doesn't exist.
func LoadState(bundleDir string) (BundleState, error) {
	statePath := filepath.Join(bundleDir, StateFileName)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return BundleState{
				Version:       "1",
				EmbeddingMode: "",
				ChunkingHash:  "",
				Files:         make(map[string]FileEntry),
			}, nil
		}
		return BundleState{}, fmt.Errorf("read state file: %w", err)
	}

	var state BundleState
	if err := json.Unmarshal(data, &state); err != nil {
		return BundleState{}, fmt.Errorf("parse state file: %w", err)
	}

	// Ensure Files map is initialized even if the state file has a nil map
	if state.Files == nil {
		state.Files = make(map[string]FileEntry)
	}

	return state, nil
}

// SaveState writes state.json to the bundle directory atomically.
// Writes to state.json.tmp first, then renames.
func SaveState(bundleDir string, state BundleState) error {
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("create bundle directory: %w", err)
	}

	statePath := filepath.Join(bundleDir, StateFileName)
	tempPath := statePath + ".tmp"

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("write temp state file: %w", err)
	}

	if err := os.Rename(tempPath, statePath); err != nil {
		return fmt.Errorf("rename temp state file: %w", err)
	}

	return nil
}

// DiffResult describes what changed between the current state and the filesystem.
type DiffResult struct {
	NewFiles       []string // Files on disk but not in state
	ChangedFiles   []string // Files with different content hash
	DeletedFiles   []string // Files in state but not on disk
	UnchangedFiles []string // Files with same content hash
}

// makeFileKey creates the key used to identify a file in the state.
// Format: "sourceName|relPath"
func makeFileKey(sourceName, relPath string) string {
	return fmt.Sprintf("%s|%s", sourceName, relPath)
}

// contentHash computes a SHA1 hash of the file content.
func contentHash(text string) string {
	h := sha1.Sum([]byte(text))
	return hex.EncodeToString(h[:])
}

// Diff compares the current filesystem against the stored state.
// currentFiles is a map of "sourceName|relPath" -> FileDocument from scanning.
// It computes content hashes for files that might have changed.
func Diff(state BundleState, currentFiles map[string]scan.FileDocument) DiffResult {
	result := DiffResult{
		NewFiles:       make([]string, 0),
		ChangedFiles:   make([]string, 0),
		DeletedFiles:   make([]string, 0),
		UnchangedFiles: make([]string, 0),
	}

	// Track which keys from state we've seen in currentFiles
	seen := make(map[string]bool)

	// Check each current file against state
	for key, doc := range currentFiles {
		seen[key] = true

		entry, exists := state.Files[key]
		if !exists {
			// File is new
			result.NewFiles = append(result.NewFiles, key)
			continue
		}

		// Compare modification times first (fast path)
		if doc.ModifiedAt.Equal(entry.ModifiedAt) {
			result.UnchangedFiles = append(result.UnchangedFiles, key)
			continue
		}

		// Mod times differ, compute hash to check if content actually changed
		hash := contentHash(doc.Content)
		if hash == entry.ContentHash {
			result.UnchangedFiles = append(result.UnchangedFiles, key)
		} else {
			result.ChangedFiles = append(result.ChangedFiles, key)
		}
	}

	// Find deleted files (in state but not on disk)
	for key := range state.Files {
		if !seen[key] {
			result.DeletedFiles = append(result.DeletedFiles, key)
		}
	}

	return result
}

// ChunkingConfigHash computes a deterministic hash of the chunking config.
// Used to detect when chunking settings change (which invalidates all chunk IDs).
func ChunkingConfigHash(cfg config.ChunkingConfig) string {
	// Hash the fields that affect chunk boundaries
	data := fmt.Sprintf("%d|%d|%d|%t", cfg.MaxChars, cfg.OverlapChars, cfg.MinChars, cfg.RespectHeadings)
	h := sha1.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}
