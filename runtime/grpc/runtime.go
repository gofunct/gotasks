package grpc

import (
	"github.com/gofunct/service/runtime/api/todo"
	api "github.com/gofunct/service/runtime/api/todo/v1"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"net"
	"net/http"
)

func VString(key string) string {
	s := viper.GetString(key)
	return s
}

func Serve() func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		lis, err := net.Listen("tcp", VString("grpc_port"))
		if err != nil {
			log.FatalViper("Failed to listen:", "grpc_port")
		}
		tracer, closer, err := Trace(log.JZap)
		if err != nil {
			log.Zap.Fatal("Cannot initialize Jaeger Tracer %s", zap.Error(err))
		}
		defer closer.Close()

		// Set GRPC Interceptors
		server := NewServer(tracer)

		api.RegisterTodoServiceServer(server, &todo.Service{DB: NewDB()})

		mux := NewMux()
		log.Zap.Debug("Starting debug service..", zap.String("grpc_debug_port", VString("grpc_debug_port")))
		go func() { http.ListenAndServe(VString("grpc_debug_port"), mux) }()

		log.Zap.Debug("Starting grpc service..", zap.String("grpc_port", VString("grpc_port")))
		server.Serve(lis)
	}
}
