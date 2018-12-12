package cmd

import (
	"fmt"
	"github.com/gofunct/gotasks/runtime/viper"
	"os"

	"github.com/spf13/cobra"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:               "gotasks",
	Short:             "a golang utility to help create grpc microservices",
	Version:           "0.1",
	PersistentPreRunE: viper.Viperize(),
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the RootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
