package main

import (
	"github.com/gofunct/service/runtime/cmd"
	"github.com/gofunct/service/runtime/gateway"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
)

var log = gateway.Log()

func main() {
	cmd.Execute()
}

func init() {
	viper.SetConfigName("goservice")        // name of config file (without extension)
	viper.AddConfigPath(os.Getenv("$HOME")) // name of config file (without extension)
	viper.AddConfigPath("../../")           // call multiple times to add many search paths
	viper.AddConfigPath(".")                // optionally look for config in the working directory
	viper.AutomaticEnv()                    // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Zap.Debug("Using config file:", zap.String("config", viper.ConfigFileUsed()))
	} else {
		log.Fatal("Fatal error config file: %s \n", zap.Error(err))
	}
}
