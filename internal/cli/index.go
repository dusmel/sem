package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/indexer"
)

func newIndexCmd(application *app.App) *cobra.Command {
	var sourceName string
	var full bool

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build the semantic index",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := application.LoadConfig()
			if err != nil {
				return err
			}

			result, err := indexer.Run(cmd.Context(), application.Paths, cfg, sourceName, full)
			if err != nil {
				return err
			}

			if full || result.NewFiles+result.ChangedFiles+result.DeletedFiles == 0 {
				// Full rebuild or no changes detected
				fmt.Fprintf(cmd.OutOrStdout(), "Indexed %d sources, %d files, %d chunks in %s\n",
					result.SourceCount, result.FileCount, result.ChunkCount, result.Duration.Round(1000000))
			} else {
				// Incremental sync
				fmt.Fprintf(cmd.OutOrStdout(), "Synced %d sources in %s\n",
					result.SourceCount, result.Duration.Round(1000000))
				fmt.Fprintf(cmd.OutOrStdout(), "Files: %d new, %d changed, %d deleted\n",
					result.NewFiles, result.ChangedFiles, result.DeletedFiles)
				fmt.Fprintf(cmd.OutOrStdout(), "Chunks: %d total\n", result.ChunkCount)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Embedding mode: %s (%s)\n", result.Model.Mode, result.Model.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "Restrict indexing to a single source")
	cmd.Flags().BoolVar(&full, "full", false, "Force a full rebuild instead of incremental sync")
	return cmd
}
