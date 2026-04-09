package cli

import (
	"github.com/spf13/cobra"

	"sem/internal/app"
)

func NewRootCmd(application *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sem",
		Short:         "Local-first semantic search for repos and notes",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(
		newInitCmd(application),
		newSourceCmd(application),
		newIndexCmd(application),
		newSyncCmd(application),
		newStatusCmd(application),
		newSearchCmd(application),
		newDoctorCmd(application),
	)

	return cmd
}
