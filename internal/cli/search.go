package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/chunk"
	"sem/internal/config"
	"sem/internal/embed"
	"sem/internal/log"
	"sem/internal/output"
	"sem/internal/search"
	"sem/internal/storage"
)

func newSearchCmd(application *app.App) *cobra.Command {
	var jsonOutput bool
	var limit int
	var sourceName string
	var searchMode string
	var language string
	var kind string
	var dir string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search indexed content semantically",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, application, args[0], jsonOutput, limit, sourceName, searchMode, language, kind, dir, verbose)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON output")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of results")
	cmd.Flags().StringVar(&sourceName, "source", "", "Restrict search to a source")
	cmd.Flags().StringVar(&searchMode, "mode", "hybrid", "Search mode: semantic, exact, or hybrid")
	cmd.Flags().StringVar(&language, "language", "", "Filter by language (comma-separated: go,python,rust)")
	cmd.Flags().StringVar(&kind, "kind", "", "Filter by file kind (code, markdown, text)")
	cmd.Flags().StringVar(&dir, "dir", "", "Filter by subdirectory path (e.g. 'src/', 'notes/')")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show debug output on stderr")
	return cmd
}

func runSearch(cmd *cobra.Command, application *app.App, query string, jsonOutput bool, limit int, sourceName string, searchMode string, language string, kind string, dir string, verbose bool) error {
	logger := log.New(verbose)

	if limit <= 0 {
		limit = 10
	}

	// Validate search mode
	switch searchMode {
	case "semantic", "exact", "hybrid":
		// valid
	default:
		return fmt.Errorf("unknown search mode %q (valid: semantic, exact, hybrid)", searchMode)
	}

	started := time.Now()
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	logger.Debug("config loaded from %s", application.Paths.ConfigPath)
	logger.Debug("embedding mode: %s", cfg.Embedding.Mode)

	// Load bundle and model for semantic/hybrid modes
	bundle := storage.NewBundle(application.Paths.BundleDir(cfg.General.DefaultBundle))

	// Check if ripgrep is available (needed for exact/hybrid modes)
	_, rgAvailable := search.IsRipgrepAvailable()
	if (searchMode == "exact" || searchMode == "hybrid") && !rgAvailable {
		fmt.Fprintln(os.Stderr, "Warning: ripgrep not found, falling back to semantic search")
		searchMode = "semantic"
	}

	// Build source paths and map for filtering
	sourcePaths, sourceMap := buildSourcePaths(cfg, sourceName)

	switch searchMode {
	case "semantic":
		return runSemanticSearch(cmd, application, query, jsonOutput, limit, sourceName, language, kind, dir, started, bundle, cfg, logger)
	case "exact":
		return runExactSearch(cmd, query, jsonOutput, limit, sourceName, language, kind, dir, started, sourcePaths, sourceMap, logger)
	case "hybrid":
		return runHybridSearch(cmd, application, query, jsonOutput, limit, sourceName, language, kind, dir, started, bundle, cfg, sourcePaths, sourceMap, logger)
	}

	return nil
}

func snippet(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 220 {
		return content
	}
	return content[:217] + "..."
}

func filterByLanguage(results []output.SearchResult, languages string) []output.SearchResult {
	if languages == "" {
		return results
	}
	allowed := strings.Split(languages, ",")
	for i := range allowed {
		allowed[i] = strings.TrimSpace(allowed[i])
	}

	var filtered []output.SearchResult
	for _, r := range results {
		for _, lang := range allowed {
			if strings.EqualFold(r.Metadata.Language, lang) {
				filtered = append(filtered, r)
				break
			}
		}
	}
	return filtered
}

func filterByKind(results []output.SearchResult, kind string) []output.SearchResult {
	if kind == "" {
		return results
	}

	var filtered []output.SearchResult
	for _, r := range results {
		if strings.EqualFold(r.Metadata.FileKind, kind) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func filterByDir(results []output.SearchResult, dir string) []output.SearchResult {
	if dir == "" {
		return results
	}

	// Normalize: strip trailing slashes, then add one back
	dir = strings.TrimRight(dir, "/") + "/"

	var filtered []output.SearchResult
	for _, r := range results {
		// Match against the full absolute path
		if strings.Contains(r.FilePath, dir) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// detectKindFromPath infers file kind from extension.
func detectKindFromPath(path string) string {
	ext := ""
	if idx := strings.LastIndex(path, "."); idx != -1 {
		ext = strings.ToLower(path[idx+1:])
	}
	switch ext {
	case "md", "markdown":
		return "markdown"
	case "go", "rs", "ts", "tsx", "js", "jsx", "py", "java", "c", "cc", "cpp", "h", "hpp", "sh", "bash", "zsh", "json", "toml", "yaml", "yml":
		return "code"
	case "txt", "text", "rst", "canvas":
		return "text"
	default:
		return "unknown"
	}
}

// detectLangFromPath infers programming language from extension.
func detectLangFromPath(path string) string {
	ext := ""
	if idx := strings.LastIndex(path, "."); idx != -1 {
		ext = strings.ToLower(path[idx+1:])
	}
	languages := map[string]string{
		"go": "go", "rs": "rust", "ts": "typescript", "tsx": "typescript",
		"js": "javascript", "jsx": "javascript", "py": "python",
		"sh": "shell", "bash": "shell", "zsh": "shell",
		"json": "json", "toml": "toml", "yaml": "yaml", "yml": "yaml",
		"md": "markdown", "markdown": "markdown",
	}
	return languages[ext]
}

// buildSourcePaths creates source paths and source map for filtering.
func buildSourcePaths(cfg config.Config, sourceName string) ([]string, map[string]string) {
	sourcePaths := []string{}
	sourceMap := map[string]string{}

	for _, src := range cfg.Sources {
		if !src.Enabled {
			continue
		}
		if sourceName != "" && src.Name != sourceName {
			continue
		}
		sourcePaths = append(sourcePaths, src.Path)
		sourceMap[src.Path] = src.Name
	}

	return sourcePaths, sourceMap
}

// runSemanticSearch performs semantic-only search.
func runSemanticSearch(cmd *cobra.Command, application *app.App, query string, jsonOutput bool, limit int, sourceName string, language string, kind string, dir string, started time.Time, bundle storage.Bundle, cfg config.Config, logger *log.Logger) error {
	model, err := bundle.LoadModel()
	if err != nil {
		return err
	}
	logger.Debug("embedding mode: %s (%s)", model.Mode, model.Name)

	service, err := embed.NewServiceWithModelDir(model.Mode, cfg.Embedding.ModelCacheDir)
	if err != nil {
		return err
	}
	defer service.Close()

	embedStart := time.Now()
	queryVector, err := service.EmbedQuery(cmd.Context(), query)
	if err != nil {
		return fmt.Errorf("embed query: %w", err)
	}
	logger.Debug("query embedding took %s", time.Since(embedStart))

	// Collect more results than needed so filters have enough to work with
	fetchLimit := limit * 5
	if fetchLimit < 50 {
		fetchLimit = 50
	}

	store := storage.NewStore(application.Paths.LanceDBDir)
	hits, err := store.Search(cmd.Context(), queryVector, fetchLimit)
	if err != nil {
		return err
	}
	logger.Debug("semantic search returned %d hits", len(hits))

	chunks, err := bundle.LoadChunks()
	if err != nil {
		return err
	}
	logger.Debug("loaded %d chunks from bundle", len(chunks))
	chunkMap := make(map[string]chunk.Record, len(chunks))
	for _, record := range chunks {
		chunkMap[record.ID] = record
	}

	results := make([]output.SearchResult, 0, fetchLimit)
	for _, hit := range hits {
		record, ok := chunkMap[hit.ChunkID]
		if !ok {
			continue
		}
		if sourceName != "" && record.SourceName != sourceName {
			continue
		}
		results = append(results, output.SearchResult{
			ChunkID:    record.ID,
			FilePath:   record.FilePath,
			Snippet:    snippet(record.Content),
			Score:      hit.Score,
			SourceName: record.SourceName,
			Metadata: output.ResultMetadata{
				FileKind:     record.Kind,
				Language:     record.Language,
				Title:        record.Title,
				StartLine:    record.StartLine,
				EndLine:      record.EndLine,
				FunctionName: record.FunctionName,
				SectionLevel: record.SectionLevel,
			},
		})
		if len(results) == fetchLimit {
			break
		}
	}

	beforeFilter := len(results)
	// Apply language, kind, and dir filters before final limit
	results = filterByLanguage(results, language)
	results = filterByKind(results, kind)
	results = filterByDir(results, dir)
	logger.Debug("filters applied: %d -> %d results", beforeFilter, len(results))

	// Apply limit after filtering
	if len(results) > limit {
		results = results[:limit]
	}

	response := output.SearchResponse{
		Query:     query,
		Mode:      "semantic",
		Filters:   output.SearchFilters{Source: sourceName, Language: language, Kind: kind, Dir: dir},
		Results:   results,
		Total:     len(results),
		ElapsedMS: time.Since(started).Milliseconds(),
	}

	logger.Debug("total search took %s", time.Since(started))

	if jsonOutput {
		return output.PrintJSON(cmd.OutOrStdout(), response)
	}

	output.PrintHuman(cmd.OutOrStdout(), response)
	return nil
}

// runExactSearch performs exact text search using ripgrep.
func runExactSearch(cmd *cobra.Command, query string, jsonOutput bool, limit int, sourceName string, language string, kind string, dir string, started time.Time, sourcePaths []string, sourceMap map[string]string, logger *log.Logger) error {
	rgStart := time.Now()
	rgResult, err := search.SearchExact(cmd.Context(), query, sourcePaths, sourceMap, limit*5)
	if err != nil {
		return err
	}
	logger.Debug("ripgrep took %s, found %d matches", time.Since(rgStart), len(rgResult.Matches))

	// Collect all ripgrep results (already limited by SearchExact to limit*5)
	// so filters have enough to work with
	results := make([]output.SearchResult, 0, len(rgResult.Matches))
	for _, match := range rgResult.Matches {
		// Trim leading whitespace from line text and adjust submatch positions
		lineText := match.LineText
		leadingWS := len(lineText) - len(strings.TrimLeft(lineText, " \t"))
		snippetText := strings.TrimSpace(lineText)

		matchedTerms := make([]output.MatchedTerm, 0, len(match.Submatches))
		for _, sub := range match.Submatches {
			// Adjust positions for trimmed leading whitespace
			start := sub.Start - leadingWS
			end := sub.End - leadingWS
			if start >= 0 && end <= len(snippetText) && start < end {
				matchedTerms = append(matchedTerms, output.MatchedTerm{Start: start, End: end})
			}
		}

		results = append(results, output.SearchResult{
			ChunkID:    "",
			FilePath:   match.FilePath,
			Snippet:    snippetText,
			Score:      1.0,
			SourceName: match.SourceName,
			Metadata: output.ResultMetadata{
				FileKind:  detectKindFromPath(match.FilePath),
				Language:  detectLangFromPath(match.FilePath),
				Title:     "",
				StartLine: match.LineNumber,
				EndLine:   match.LineNumber,
			},
			MatchedTerms: matchedTerms,
		})
	}

	beforeFilter := len(results)
	// Apply language, kind, and dir filters before final limit
	results = filterByLanguage(results, language)
	results = filterByKind(results, kind)
	results = filterByDir(results, dir)
	logger.Debug("filters applied: %d -> %d results", beforeFilter, len(results))

	// Apply limit after filtering
	if len(results) > limit {
		results = results[:limit]
	}

	response := output.SearchResponse{
		Query:     query,
		Mode:      "exact",
		Filters:   output.SearchFilters{Source: sourceName, Language: language, Kind: kind, Dir: dir},
		Results:   results,
		Total:     len(results),
		ElapsedMS: time.Since(started).Milliseconds(),
	}

	logger.Debug("total search took %s", time.Since(started))

	if jsonOutput {
		return output.PrintJSON(cmd.OutOrStdout(), response)
	}

	output.PrintHuman(cmd.OutOrStdout(), response)
	return nil
}

// runHybridSearch combines semantic and exact search using RRF.
func runHybridSearch(cmd *cobra.Command, application *app.App, query string, jsonOutput bool, limit int, sourceName string, language string, kind string, dir string, started time.Time, bundle storage.Bundle, cfg config.Config, sourcePaths []string, sourceMap map[string]string, logger *log.Logger) error {
	// Run both searches concurrently
	model, err := bundle.LoadModel()
	if err != nil {
		return err
	}
	logger.Debug("embedding mode: %s (%s)", model.Mode, model.Name)

	service, err := embed.NewServiceWithModelDir(model.Mode, cfg.Embedding.ModelCacheDir)
	if err != nil {
		return err
	}
	defer service.Close()

	// Collect more results than needed so filters have enough to work with.
	// When dir or kind filters are active, we need a larger pool because many
	// results will be discarded after ranking.
	fetchLimit := limit * 10
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	// Semantic search
	embedStart := time.Now()
	queryVector, err := service.EmbedQuery(cmd.Context(), query)
	if err != nil {
		return fmt.Errorf("embed query: %w", err)
	}
	logger.Debug("query embedding took %s", time.Since(embedStart))

	store := storage.NewStore(application.Paths.LanceDBDir)
	semanticHits, err := store.Search(cmd.Context(), queryVector, fetchLimit)
	if err != nil {
		return err
	}
	logger.Debug("semantic search returned %d hits", len(semanticHits))

	// Exact search
	rgStart := time.Now()
	rgResult, err := search.SearchExact(cmd.Context(), query, sourcePaths, sourceMap, fetchLimit)
	if err != nil {
		return err
	}
	logger.Debug("ripgrep took %s, found %d matches", time.Since(rgStart), len(rgResult.Matches))

	// Load chunks for RRF mapping
	chunks, err := bundle.LoadChunks()
	if err != nil {
		return err
	}
	logger.Debug("loaded %d chunks from bundle", len(chunks))

	// Convert chunks to ChunkInfo map
	chunkInfoMap := make(map[string]search.ChunkInfo, len(chunks))
	for _, record := range chunks {
		chunkInfoMap[record.ID] = search.ChunkInfo{
			ID:           record.ID,
			FilePath:     record.FilePath,
			RelPath:      record.RelPath,
			SourceName:   record.SourceName,
			StartLine:    record.StartLine,
			EndLine:      record.EndLine,
			Content:      record.Content,
			Kind:         record.Kind,
			Language:     record.Language,
			Title:        record.Title,
			FunctionName: record.FunctionName,
			SectionLevel: record.SectionLevel,
		}
	}

	// Convert semantic hits to SemanticHit
	semanticHitList := make([]search.SemanticHit, len(semanticHits))
	for i, hit := range semanticHits {
		semanticHitList[i] = search.SemanticHit{
			ChunkID: hit.ChunkID,
			Score:   hit.Score,
		}
	}

	// Merge with RRF
	hybridResults := search.MergeWithRRF(semanticHitList, rgResult.Matches, chunkInfoMap, search.DefaultRRFConstant)
	logger.Debug("RRF merge produced %d results", len(hybridResults))

	results := make([]output.SearchResult, 0, fetchLimit)
	for _, result := range hybridResults {
		if sourceName != "" && result.SourceName != sourceName {
			continue
		}
		results = append(results, output.SearchResult{
			ChunkID:    result.ChunkID,
			FilePath:   result.FilePath,
			Snippet:    snippet(result.Snippet),
			Score:      float32(result.Score),
			SourceName: result.SourceName,
			Metadata: output.ResultMetadata{
				FileKind:     result.Metadata.FileKind,
				Language:     result.Metadata.Language,
				Title:        result.Metadata.Title,
				StartLine:    result.Metadata.StartLine,
				EndLine:      result.Metadata.EndLine,
				FunctionName: result.Metadata.FunctionName,
				SectionLevel: result.Metadata.SectionLevel,
			},
		})
		if len(results) == fetchLimit {
			break
		}
	}

	beforeFilter := len(results)
	// Apply language, kind, and dir filters before final limit
	results = filterByLanguage(results, language)
	results = filterByKind(results, kind)
	results = filterByDir(results, dir)
	logger.Debug("filters applied: %d -> %d results", beforeFilter, len(results))

	// Apply limit after filtering
	if len(results) > limit {
		results = results[:limit]
	}

	response := output.SearchResponse{
		Query:     query,
		Mode:      "hybrid",
		Filters:   output.SearchFilters{Source: sourceName, Language: language, Kind: kind, Dir: dir},
		Results:   results,
		Total:     len(results),
		ElapsedMS: time.Since(started).Milliseconds(),
	}

	logger.Debug("total search took %s", time.Since(started))

	if jsonOutput {
		return output.PrintJSON(cmd.OutOrStdout(), response)
	}

	output.PrintHuman(cmd.OutOrStdout(), response)
	return nil
}
