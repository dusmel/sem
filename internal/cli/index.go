package cli

import (
	"fmt"
	"os"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/indexer"
	"sem/internal/log"
	"sem/internal/progress"
)

func newIndexCmd(application *app.App) *cobra.Command {
	var sourceName string
	var full bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build the semantic index",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := log.New(verbose)
			logger.Debug("config loaded from %s", application.Paths.ConfigPath)

			cfg, err := application.LoadConfig()
			if err != nil {
				return err
			}
			logger.Debug("embedding mode: %s", cfg.Embedding.Mode)

			// Build progress callbacks if progress bars should be shown
			var prog *indexer.ProgressCallbacks
			if progress.ShouldDisable(verbose) {
				// Verbose mode or non-TTY: use debug logging only
				prog = buildDebugProgressCallbacks(logger)
			} else {
				// TTY mode: use progress bars
				prog = buildProgressBarCallbacks()
			}

			result, err := indexer.Run(cmd.Context(), application.Paths, cfg, sourceName, full, logger, prog)
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
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show debug output on stderr")
	return cmd
}

// buildProgressBarCallbacks creates progress callbacks that display progress bars.
func buildProgressBarCallbacks() *indexer.ProgressCallbacks {
	// Use closures to capture bar references
	type barHolder struct {
		bar *progressbar.ProgressBar
	}
	embedBar := &barHolder{}
	writeBar := &barHolder{}

	return &indexer.ProgressCallbacks{
		OnEmbedStart: func(total int) {
			embedBar.bar = progressbar.NewOptions(total,
				progressbar.OptionSetDescription("Embedding chunks..."),
				progressbar.OptionShowCount(),
				progressbar.OptionSetPredictTime(true),
				progressbar.OptionSetSpinnerChangeInterval(0),
				progressbar.OptionSetTheme(progressbar.Theme{
					Saucer:        "=",
					SaucerPadding: " ",
					BarStart:      "[",
					BarEnd:        "]",
				}),
				progressbar.OptionSetWriter(os.Stderr),
			)
		},
		OnEmbedProgress: func(current, total int) {
			if embedBar.bar != nil {
				embedBar.bar.Set(current)
			}
		},
		OnWriteStart: func(total int) {
			writeBar.bar = progressbar.NewOptions(total,
				progressbar.OptionSetDescription("Writing bundle..."),
				progressbar.OptionShowCount(),
				progressbar.OptionSetPredictTime(true),
				progressbar.OptionSetSpinnerChangeInterval(0),
				progressbar.OptionSetTheme(progressbar.Theme{
					Saucer:        "=",
					SaucerPadding: " ",
					BarStart:      "[",
					BarEnd:        "]",
				}),
				progressbar.OptionSetWriter(os.Stderr),
			)
		},
		OnWriteProgress: func(current, total int) {
			if writeBar.bar != nil {
				writeBar.bar.Set(current)
			}
		},
	}
}

// buildDebugProgressCallbacks creates progress callbacks that log via debug logger.
// Only logs when a phase completes (current == total) for clean output.
func buildDebugProgressCallbacks(logger *log.Logger) *indexer.ProgressCallbacks {
	return &indexer.ProgressCallbacks{
		OnScanProgress: func(current, total int) {
			if current == total {
				logger.Debug("scanned %d/%d files", current, total)
			}
		},
		OnChunkProgress: func(current, total int) {
			if current == total {
				logger.Debug("chunked %d/%d files", current, total)
			}
		},
		OnEmbedProgress: func(current, total int) {
			if current == total {
				logger.Debug("embedded %d/%d chunks", current, total)
			}
		},
		OnWriteProgress: func(current, total int) {
			if current == total {
				logger.Debug("write progress %d/%d", current, total)
			}
		},
	}
}
