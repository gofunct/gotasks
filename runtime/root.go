package runtime

import (
	"github.com/gofunct/service/runtime/grpc"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
)

func init() {
	log = grpc.Log()
	Root = newConfig()
	os.Setenv("JAEGER_SERVICE_NAME", viper.GetString("name"))
	os.Setenv("JAEGER_RPC_METRICS", "true")
}

func Initialize(){
	viper.SetConfigName("goservice") // name of config file (without extension)
	viper.AddConfigPath("$HOME/.appname")  // call multiple times to add many search paths
	viper.AddConfigPath(".")               // optionally look for config in the working directory
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Zap.Debug("Using config file:", zap.String("config", viper.ConfigFileUsed()))
	} else {
		log.Zap.Fatal("Fatal error config file: %s \n", zap.Error(err))
	}
}

var Root *RootConfig
var CfgFile string
var log *grpc.Logger

type RootConfig struct {
	Grpc     *GrpcConfig
	Postgres *DbConfig
	Gateway  *GatewayConfig
}


type RunFuncE func(cmd *cobra.Command, args []string) error

type RunFunc func(cmd *cobra.Command, args []string)

type GrpcConfig struct {
	Network string
	Host      string
	Port      string
	DebugPort string
}

type DbConfig struct {
	Name string
	Network string
	Host string
	Port string
	Pass string
	User string
}

type GatewayConfig struct {
	Network string
	Host      string
	Port      string
	DebugPort string
}

func newConfig() *RootConfig {
	return &RootConfig{

		Grpc: &GrpcConfig{
			Network:   "grpc_network",
			Host:      viper.GetString("grpc_host"),
			Port:      viper.GetString("grpc_port"),
			DebugPort: viper.GetString("grpc_debug_port"),
		},
		Postgres: &DbConfig{
			Name:    viper.GetString("db_name"),
			Network: "db_network",
			Host:    viper.GetString("db_host"),
			Port:    viper.GetString("db_port"),
			Pass:    viper.GetString("db_pass"),
			User:    viper.GetString("db_user"),
		},
		Gateway: &GatewayConfig{
			Network:   "gw_network",
			Host:      viper.GetString("gw_host"),
			Port:      viper.GetString("gw_port"),
			DebugPort: viper.GetString("gw_debug_port"),
		},
	}
}