package config

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
)

func init() { RootCmd = newConfig() }

type Config struct {
	Grpc     *GrpcConfig
	Postgres *DbConfig
	Gateway  *GatewayConfig
	Root     *cobra.Command
}

type GrpcConfig struct {
	Host      string
	Port      string
	DebugPort string
}

type DbConfig struct {
	Name string
	Host string
	Port string
	Pass string
	User string
}

type GatewayConfig struct {
	Host      string
	Port      string
	DebugPort string
}

var RootCmd *Config

func newConfig() *Config {
	initConfig()
	return &Config{
		Grpc: &GrpcConfig{
			Host:      viper.GetString("grpc_host"),
			Port:      viper.GetString("grpc_port"),
			DebugPort: viper.GetString("grpc_debug_port"),
		},
		Postgres: &DbConfig{
			Port: viper.GetString("db_port"),
			Host: viper.GetString("db_host"),
			Pass: viper.GetString("db_pass"),
			User: viper.GetString("db_user"),
			Name: viper.GetString("db_name"),
		},
		Gateway: &GatewayConfig{
			Host:      viper.GetString("gw_host"),
			Port:      viper.GetString("gw_port"),
			DebugPort: viper.GetString("gw_debug_port"),
		},
		Root: &cobra.Command{
			Use:   viper.GetString("name"),
			Short: viper.GetString("info"),
			PersistentPreRun: func(cmd *cobra.Command, args []string) {
				os.Setenv("JAEGER_SERVICE_NAME", viper.GetString("name"))
				os.Setenv("JAEGER_AGENT_HOST_PORT", viper.GetString("JAEGER_AGENT_HOST_PORT"))
				os.Setenv("JAEGER_RPC_METRICS", viper.GetString("JAEGER_RPC_METRICS"))
			},
		},
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetConfigType("yaml")
	viper.SetConfigName("service") // name of config file (without extension)
	viper.AddConfigPath(".")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))

	}
}
