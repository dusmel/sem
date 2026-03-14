package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"sem/internal/app"
	"sem/internal/config"
	"sem/internal/errs"
	"sem/internal/source"
)

func newSourceCmd(application *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage indexed sources",
	}
	cmd.AddCommand(
		newSourceAddCmd(application),
		newSourceListCmd(application),
		newSourceRemoveCmd(application),
	)
	return cmd
}

func newSourceAddCmd(application *app.App) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Add a source directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := application.LoadConfig()
			if err != nil {
				return err
			}

			added, err := source.Add(&cfg, args[0], name)
			if err != nil {
				return err
			}

			if err := config.Save(application.Paths.ConfigPath, cfg); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added source %s -> %s\n", added.Name, added.Path)
			fmt.Fprintln(cmd.OutOrStdout(), "Next step: sem index")
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Explicit source name")
	return cmd
}

func newSourceListCmd(application *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := application.LoadConfig()
			if err != nil {
				return err
			}
			if len(cfg.Sources) == 0 {
				return errs.ErrNoSources
			}

			fmt.Fprintln(cmd.OutOrStdout(), "NAME\tENABLED\tPATH")
			for _, src := range cfg.Sources {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%t\t%s\n", src.Name, src.Enabled, src.Path)
			}
			return nil
		},
	}
}

func newSourceRemoveCmd(application *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a configured source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := application.LoadConfig()
			if err != nil {
				return err
			}

			if err := source.Remove(&cfg, args[0]); err != nil {
				return err
			}

			if err := config.Save(application.Paths.ConfigPath, cfg); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed source %s\n", args[0])
			fmt.Fprintln(cmd.OutOrStdout(), "Run sem index to rebuild the bundle without this source.")
			return nil
		},
	}
}