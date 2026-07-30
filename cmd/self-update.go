package cmd

import (
	"github.com/spf13/cobra"
	"github.com/synctv-org/synctv/internal/bootstrap"
	"github.com/synctv-org/synctv/internal/version"
)

const SelfUpdateLong = version.SelfUpdateDisabledMessage

var SelfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "self-update",
	Long:  SelfUpdateLong,
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		return bootstrap.New().Add(
			bootstrap.InitStdLog,
		).Run(cmd.Context())
	},
	RunE: SelfUpdate,
}

func SelfUpdate(_ *cobra.Command, _ []string) error {
	return version.ErrSelfUpdateDisabled
}

func init() {
	RootCmd.AddCommand(SelfUpdateCmd)
}
