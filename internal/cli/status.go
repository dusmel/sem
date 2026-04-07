package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/indexer"
	"sem/internal/storage"
)

func newStatusCmd(application *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show index status and staleness",
		Long: `Status displays information about the current index state,
including the bundle name, indexing time, model used, and source statistics.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := application.LoadConfig()
			if err != nil {
				return err
			}

			bundleDir := application.Paths.BundleDir(cfg.General.DefaultBundle)
			bundle := storage.NewBundle(bundleDir)

			manifest, err := bundle.LoadManifest()
			if err != nil {
				return fmt.Errorf("load manifest: %w", err)
			}

			state, err := indexer.LoadState(bundleDir)
			if err != nil {
				return fmt.Errorf("load state: %w", err)
			}

			// Print status
			fmt.Fprintf(cmd.OutOrStdout(), "Bundle: %s\n", manifest.BundleName)
			fmt.Fprintf(cmd.OutOrStdout(), "Indexed: %s\n", manifest.IndexedAt.Format("2006-01-02 15:04:05"))
			fmt.Fprintf(cmd.OutOrStdout(), "Model: %s (%s)\n", manifest.EmbeddingModel.Mode, manifest.EmbeddingModel.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Sources: %d\n", manifest.SourceCount)

			// Count files per source from state
			sourceCounts := make(map[string]int)
			for key := range state.Files {
				parts := strings.SplitN(key, "|", 2)
				if len(parts) > 0 {
					sourceCounts[parts[0]]++
				}
			}
			for _, src := range manifest.Sources {
				count := sourceCounts[src.Name]
				fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %d files\n", src.Name, count)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Total: %d files, %d chunks, %d embeddings\n",
				len(state.Files), manifest.ChunkCount, manifest.EmbeddingCount)

			// Check staleness by scanning and comparing mod times
			// For now, just show the state
			if len(state.Files) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nState: empty (run sem index)")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\nState: indexed")
			}

			return nil
		},
	}
}
