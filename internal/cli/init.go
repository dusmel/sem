package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/config"
	"sem/internal/embed"
	"sem/internal/storage"
)

func newInitCmd(application *app.App) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize sem in ~/.sem",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.Paths.EnsureLayout("default"); err != nil {
				return err
			}

			cfg, err := config.WriteDefault(application.Paths.ConfigPath, application.Paths.BaseDir, force)
			if err != nil {
				return err
			}

			model, err := embed.Catalog(cfg.Embedding.Mode)
			if err != nil {
				return err
			}

			if err := storage.Initialize(application.Paths.BundleDir(cfg.General.DefaultBundle), cfg.General.DefaultBundle, model); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Initialized sem at %s\n", application.Paths.BaseDir)
			fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", application.Paths.ConfigPath)
			fmt.Fprintln(cmd.OutOrStdout(), "Next step: sem source add <path>")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite the existing configuration and default metadata")
	return cmd
}
