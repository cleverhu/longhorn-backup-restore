package cmd

import (
	"github.com/spf13/cobra"
)

var (
	Kubeconfig  = "./kubeconfig"
	ApiEndpoint = ""
)

func NewLonghornBackupRestoreCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "longhorn-backup-restore",
	}
	cmd.AddCommand(NewBakCommand())
	cmd.AddCommand(NewRecoverCmd())
	cmd.PersistentFlags().StringVar(&Kubeconfig, "kubeconfig", Kubeconfig, "Path to a kubeconfig file, specifying how to connect to the API server. Providing --kubeconfig enables API server mode, omitting --kubeconfig enables standalone mode.")
	cmd.PersistentFlags().StringVar(&ApiEndpoint, "api-endpoint", ApiEndpoint, "The endpoint for longhorn api.")
	return cmd
}
