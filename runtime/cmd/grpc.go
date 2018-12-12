package cmd

import (
	"github.com/gofunct/gotasks/runtime/grpc"
	"github.com/spf13/cobra"
)

// grpcCmd represents the grpc command
var grpcCmd = &cobra.Command{
	Use:   "grpc",
	Short: "start a grpc server with config",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: grpc.Serve(),
}

func init() { RootCmd.AddCommand(grpcCmd) }
