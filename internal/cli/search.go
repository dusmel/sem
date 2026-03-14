package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/chunk"
	"sem/internal/embed"
	"sem/internal/output"
	"sem/internal/storage"
)

func newSearchCmd(application *app.App) *cobra.Command {
	var jsonOutput bool
	var limit int
	var sourceName string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search indexed content semantically",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, application, args[0], jsonOutput, limit, sourceName)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON output")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of results")
	cmd.Flags().StringVar(&sourceName, "source", "", "Restrict search to a source")
	return cmd
}

func runSearch(cmd *cobra.Command, application *app.App, query string, jsonOutput bool, limit int, sourceName string) error {
	if limit <= 0 {
		limit = 10
	}

	started := time.Now()
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}

	bundle := storage.NewBundle(application.Paths.BundleDir(cfg.General.DefaultBundle))
	model, err := bundle.LoadModel()
	if err != nil {
		return err
	}

	service, err := embed.NewService(model.Mode)
	if err != nil {
		return err
	}

	queryVector, err := service.EmbedQuery(cmd.Context(), query)
	if err != nil {
		return fmt.Errorf("embed query: %w", err)
	}

	store := storage.NewStore(application.Paths.LanceDBDir)
	hits, err := store.Search(cmd.Context(), queryVector, limit*5)
	if err != nil {
		return err
	}

	chunks, err := bundle.LoadChunks()
	if err != nil {
		return err
	}
	chunkMap := make(map[string]chunk.Record, len(chunks))
	for _, record := range chunks {
		chunkMap[record.ID] = record
	}

	results := make([]output.SearchResult, 0, limit)
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
				FileKind:  record.Kind,
				Language:  record.Language,
				Title:     record.Title,
				StartLine: record.StartLine,
				EndLine:   record.EndLine,
			},
		})
		if len(results) == limit {
			break
		}
	}

	response := output.SearchResponse{
		Query:     query,
		Mode:      model.Mode,
		Results:   results,
		Total:     len(results),
		ElapsedMS: time.Since(started).Milliseconds(),
	}

	if jsonOutput {
		return output.PrintJSON(cmd.OutOrStdout(), response)
	}

	output.PrintHuman(cmd.OutOrStdout(), response)
	return nil
}

func snippet(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 220 {
		return content
	}
	return content[:217] + "..."
}
