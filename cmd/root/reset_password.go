package root

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/synctv-org/synctv/internal/bootstrap"
	"github.com/synctv-org/synctv/internal/db"
)

var ResetPasswordCmd = &cobra.Command{
	Use:   "reset-password <user-id>",
	Short: "reset a root password to a protected recovery file",
	Long: `reset a root password to a generated value stored in a mode 0600
recovery file. Stop the server before running this command.`,
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		return bootstrap.New().Add(
			bootstrap.InitStdLog,
			bootstrap.InitConfig,
			bootstrap.InitDatabase,
		).Run(cmd.Context())
	},
	RunE: func(_ *cobra.Command, args []string) error {
		path, err := db.ResetRootPasswordToRecoveryFile(args[0])
		if err != nil {
			return err
		}
		log.Warnf(
			"root password reset; recovery password saved to %s (mode 0600); use it to sign in and immediately choose a new password",
			path,
		)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(ResetPasswordCmd)
}
