package gateway

import (
	"context"
	api "github.com/gofunct/service/runtime/api/todo/v1"
	"github.com/gofunct/service/runtime/gateway/data"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/philips/go-bindata-assetfs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"mime"
	"net"
	"net/http"
)

func VString(key string) string {
	s := viper.GetString(key)
	return s
}

func Serve() func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		lis, err := net.Listen("tcp", VString("gw_port"))
		if err != nil {
			log.Fatal("Failed to listen:", zap.String("gw_port", VString("gw_port")))
		}
		log.Debug("Starting gateway service..")
		conn, err := grpc.Dial(viper.GetString("grpc_host")+viper.GetString("grpc_port"), grpc.WithInsecure())
		if err != nil {
			log.Fatal("failed to dial grpc backend", zap.Error(err))
		}

		mux := runtime.NewServeMux()
		err = api.RegisterTodoServiceHandler(context.Background(), mux, conn)
		if err != nil {
			panic("Cannot serve http api")
		}
		http.Serve(lis, mux)
	}
}

func ServeSwagger(mux *http.ServeMux) {
	mime.AddExtensionType(".svg", "image/svg+xml")

	// Expose files in third_party/swagger-ui/ on <host>/swagger-ui
	fileServer := http.FileServer(&assetfs.AssetFS{
		Asset:    swagger.Asset,
		AssetDir: swagger.AssetDir,
		Prefix:   "swagger/ui",
	})
	prefix := "/swagger-ui/"
	mux.Handle(prefix, http.StripPrefix(prefix, fileServer))
}
