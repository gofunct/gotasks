package service

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"log"
	"os"
)

type Commander struct {
	ConfigFile string
	RootCmd    *cobra.Command
	Logger     *zap.Logger
}

type With func(commander *Commander)

func WithRootInfo(name, info string) With {
	return func(c *Commander) {
		c.RootCmd = &cobra.Command{Use: name, Short: info}
	}
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(with ...With) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal("failed to create logger", zap.Error(err))
	}
	c := &Commander{
		ConfigFile: "",
		RootCmd:    nil,
		Logger:     logger,
	}

	for _, f := range with {
		f(c)
	}

	if err := c.RootCmd.Execute(); err != nil {
		logger.Fatal("failed to run root", zap.Error(err))
		os.Exit(-1)
	}
}

// initConfig reads in config file and ENV variables if set.
func WithConfig(path, name string) With {

	return func(c *Commander) {
		if c.ConfigFile != "" { // enable ability to specify config file via flag
			viper.SetConfigFile(c.ConfigFile)
		}

		viper.SetConfigName(name) // name of config file (without extension)
		viper.AddConfigPath(path) // adding current directory as first search path
		viper.AutomaticEnv()      // read in environment variables that match

		// If a config file is found, read it in.
		if err := viper.ReadInConfig(); err == nil {
			fmt.Println("Using config file:", viper.ConfigFileUsed())
		}
	}
}
