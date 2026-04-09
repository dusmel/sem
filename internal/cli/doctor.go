package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/doctor"
)

func newDoctorCmd(application *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the health of your sem environment",
		Long: `Doctor validates the sem environment and reports any issues.

It checks for:
  - ripgrep installation (required for exact search)
  - ONNX Runtime availability (required for real embeddings)
  - Cached model files for each embedding mode
  - Configuration file validity
  - Source path accessibility
  - Bundle existence and data`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := application.LoadConfig()
			if err != nil {
				// Config load failure is itself a doctor finding
				fmt.Fprintln(cmd.OutOrStdout(), "sem doctor")
				fmt.Fprintln(cmd.OutOrStdout(), "─────────────────────────────────────────")
				fmt.Fprintf(cmd.OutOrStdout(), "✗ config failed to load: %v\n", err)
				fmt.Fprintln(cmd.OutOrStdout(), "\nRun 'sem init' to set up your environment.")
				return nil
			}

			bundleDir := application.Paths.BundleDir(cfg.General.DefaultBundle)
			checks := doctor.RunAll(cfg, application.Paths.ConfigPath, cfg.Embedding.ModelCacheDir, bundleDir)

			// Print header
			fmt.Fprintln(cmd.OutOrStdout(), "sem doctor")
			fmt.Fprintln(cmd.OutOrStdout(), "─────────────────────────────────────────")

			issueCount := 0
			for _, check := range checks {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", check.Status.Symbol(), check.Message)
				if check.Hint != "" && check.Status != doctor.Pass {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", check.Hint)
				}
				if check.Status != doctor.Pass {
					issueCount++
				}
			}

			fmt.Fprintln(cmd.OutOrStdout())
			if issueCount == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "All checks passed.")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%d issue(s) found.", issueCount)
				fmt.Fprintln(cmd.OutOrStdout())
				// Check if bundle is missing — suggest index
				hasBundleIssue := false
				for _, check := range checks {
					if check.Name == "bundle" && check.Status != doctor.Pass {
						hasBundleIssue = true
						break
					}
				}
				if hasBundleIssue {
					fmt.Fprintln(cmd.OutOrStdout(), "Run 'sem index' to get started.")
				}
			}

			return nil
		},
	}

	return cmd
}
