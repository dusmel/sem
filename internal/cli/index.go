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

			_ = full
			result, err := indexer.Run(cmd.Context(), application.Paths, cfg, sourceName)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Indexed %d sources, %d files, %d chunks in %s\n", result.SourceCount, result.FileCount, result.ChunkCount, result.Duration.Round(1000000))
			fmt.Fprintf(cmd.OutOrStdout(), "Embedding mode: %s (%s)\n", result.Model.Mode, result.Model.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "Restrict indexing to a single source")
	cmd.Flags().BoolVar(&full, "full", false, "Accepted for forward compatibility; Stage 1 always performs a full rebuild")
	return cmd
}
