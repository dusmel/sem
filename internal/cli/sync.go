package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/indexer"
)

func newSyncCmd(application *app.App) *cobra.Command {
	var sourceName string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Incrementally sync the index with changed files",
		Long: `Sync performs an incremental update of the index.

It compares the current filesystem state with the previous index,
only processing new, changed, and deleted files. This is much faster
than a full index rebuild for large repositories with few changes.

Use --source to restrict the sync to a single source.
Use 'sem index --full' if you need a complete rebuild.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := application.LoadConfig()
			if err != nil {
				return err
			}

			result, err := indexer.Run(cmd.Context(), application.Paths, cfg, sourceName, false)
			if err != nil {
				return err
			}

			// Print sync-specific output
			fmt.Fprintf(cmd.OutOrStdout(), "Synced %d sources: %d new, %d changed, %d deleted files\n",
				result.SourceCount, result.NewFiles, result.ChangedFiles, result.DeletedFiles)
			fmt.Fprintf(cmd.OutOrStdout(), "Chunks: %d total\n", result.ChunkCount)
			fmt.Fprintf(cmd.OutOrStdout(), "Embedding mode: %s (%s)\n", result.Model.Mode, result.Model.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Duration: %s\n", result.Duration.Round(1000000))
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "Restrict sync to a single source")
	return cmd
}
