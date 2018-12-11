package cmd

import (
	"github.com/gofunct/service/runtime/gateway"

	"github.com/spf13/cobra"
)

// gatewayCmd represents the gateway command
var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "start a grpc gateway server with config",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: gateway.Serve(),
}

func init() { RootCmd.AddCommand(gatewayCmd) }
