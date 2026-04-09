package search

import (
	"sort"
)

// SemanticHit is a result from vector similarity search.
type SemanticHit struct {
	ChunkID string
	Score   float32
}

// ChunkInfo is the minimal info needed from a chunk record to map exact matches.
type ChunkInfo struct {
	ID           string
	FilePath     string
	RelPath      string
	SourceName   string
	StartLine    int
	EndLine      int
	Content      string
	Kind         string
	Language     string
	Title        string
	FunctionName string
	SectionLevel int
}

// HybridResult is a search result with both semantic and exact scores.
type HybridResult struct {
	ChunkID      string
	FilePath     string
	RelPath      string
	Snippet      string
	Score        float64 // RRF score
	SemanticRank int     // -1 if not in semantic results
	ExactRank    int     // -1 if not in exact results
	SourceName   string
	Metadata     ResultMetadata
}

// ResultMetadata contains additional result information.
type ResultMetadata struct {
	FileKind     string
	Language     string
	Title        string
	StartLine    int
	EndLine      int
	FunctionName string
	SectionLevel int
}

// rrfItem holds intermediate RRF scoring data.
type rrfItem struct {
	chunkID      string
	filePath     string
	relPath      string
	snippet      string
	score        float64
	semanticRank int
	exactRank    int
	sourceName   string
	metadata     ResultMetadata
}

// DefaultRRFConstant is the default RRF constant (k).
// k=60 is a well-tested value that works well in most cases.
const DefaultRRFConstant = 60

// MergeWithRRF merges semantic search results with exact search results
// using Reciprocal Rank Fusion. k is the RRF constant (typically 60).
//
// RRF_score(d) = Σ 1 / (k + rank_i(d))
//
// Semantic results are []SemanticHit (chunkID + score).
// Exact results are []ExactMatch (filePath + lineNumber).
// Chunks are needed to map exact matches to chunk IDs.
func MergeWithRRF(
	semanticHits []SemanticHit,
	exactMatches []ExactMatch,
	chunks map[string]ChunkInfo,
	k int,
) []HybridResult {
	if k <= 0 {
		k = DefaultRRFConstant
	}

	// Build a map: filePath → []ChunkInfo for fast lookup
	chunksByFile := buildChunksByFile(chunks)

	// Intermediate map: chunkID → rrfItem
	rrfItems := make(map[string]*rrfItem)

	// Process semantic results
	for rank, hit := range semanticHits {
		chunk, exists := chunks[hit.ChunkID]
		if !exists {
			// Skip semantic hits without corresponding chunks
			continue
		}

		chunkID := hit.ChunkID
		score := 1.0 / (float64(k) + float64(rank) + 1.0)

		if item, exists := rrfItems[chunkID]; exists {
			item.score += score
			item.semanticRank = rank
		} else {
			rrfItems[chunkID] = &rrfItem{
				chunkID:      chunkID,
				filePath:     chunk.FilePath,
				relPath:      chunk.RelPath,
				snippet:      chunk.Content,
				score:        score,
				semanticRank: rank,
				exactRank:    -1,
				sourceName:   chunk.SourceName,
				metadata: ResultMetadata{
					FileKind:     chunk.Kind,
					Language:     chunk.Language,
					Title:        chunk.Title,
					StartLine:    chunk.StartLine,
					EndLine:      chunk.EndLine,
					FunctionName: chunk.FunctionName,
					SectionLevel: chunk.SectionLevel,
				},
			}
		}
	}

	// Process exact results
	for rank, match := range exactMatches {
		// Find the chunk that contains this match's line number
		chunkID := FindChunkForLine(chunksByFile[match.FilePath], match.LineNumber)

		score := 1.0 / (float64(k) + float64(rank) + 1.0)

		if chunkID != "" {
			// Found a matching chunk
			if item, exists := rrfItems[chunkID]; exists {
				item.score += score
				item.exactRank = rank
			} else {
				// Exact match points to a chunk not in semantic results
				chunk := chunks[chunkID]
				rrfItems[chunkID] = &rrfItem{
					chunkID:      chunkID,
					filePath:     chunk.FilePath,
					relPath:      chunk.RelPath,
					snippet:      chunk.Content,
					score:        score,
					semanticRank: -1,
					exactRank:    rank,
					sourceName:   chunk.SourceName,
					metadata: ResultMetadata{
						FileKind:     chunk.Kind,
						Language:     chunk.Language,
						Title:        chunk.Title,
						StartLine:    chunk.StartLine,
						EndLine:      chunk.EndLine,
						FunctionName: chunk.FunctionName,
						SectionLevel: chunk.SectionLevel,
					},
				}
			}
		} else {
			// No matching chunk found - create synthetic result from exact match
			// Use a unique synthetic chunkID
			syntheticChunkID := "exact:" + match.FilePath + ":" + string(rune(match.LineNumber))
			rrfItems[syntheticChunkID] = &rrfItem{
				chunkID:      syntheticChunkID,
				filePath:     match.FilePath,
				relPath:      match.RelPath,
				snippet:      match.LineText,
				score:        score,
				semanticRank: -1,
				exactRank:    rank,
				sourceName:   match.SourceName,
				metadata: ResultMetadata{
					FileKind:  "unknown",
					Language:  "",
					Title:     "",
					StartLine: match.LineNumber,
					EndLine:   match.LineNumber,
				},
			}
		}
	}

	// Convert map to slice for sorting
	results := make([]HybridResult, 0, len(rrfItems))
	for _, item := range rrfItems {
		results = append(results, HybridResult{
			ChunkID:      item.chunkID,
			FilePath:     item.filePath,
			RelPath:      item.relPath,
			Snippet:      item.snippet,
			Score:        item.score,
			SemanticRank: item.semanticRank,
			ExactRank:    item.exactRank,
			SourceName:   item.sourceName,
			Metadata:     item.metadata,
		})
	}

	// Sort by RRF score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// buildChunksByFile creates a map from file path to chunks in that file.
func buildChunksByFile(chunks map[string]ChunkInfo) map[string][]ChunkInfo {
	byFile := make(map[string][]ChunkInfo)
	for _, chunk := range chunks {
		byFile[chunk.FilePath] = append(byFile[chunk.FilePath], chunk)
	}

	// Sort chunks by StartLine for each file to ensure consistent ordering
	for _, chunks := range byFile {
		sort.Slice(chunks, func(i, j int) bool {
			return chunks[i].StartLine < chunks[j].StartLine
		})
	}

	return byFile
}

// FindChunkForLine finds the chunk that contains the given line number.
// Returns the most specific (smallest range) chunk, or empty string if none found.
func FindChunkForLine(chunksByFile []ChunkInfo, lineNumber int) string {
	if len(chunksByFile) == 0 {
		return ""
	}

	var bestChunk *ChunkInfo
	bestRange := 0

	for i := range chunksByFile {
		chunk := &chunksByFile[i]
		if chunk.StartLine <= lineNumber && lineNumber <= chunk.EndLine {
			// This chunk contains the line
			chunkRange := chunk.EndLine - chunk.StartLine
			if bestChunk == nil || chunkRange < bestRange {
				bestChunk = chunk
				bestRange = chunkRange
			}
		}
	}

	if bestChunk != nil {
		return bestChunk.ID
	}

	return ""
}
