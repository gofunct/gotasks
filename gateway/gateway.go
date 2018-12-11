package gateway

import (
	"github.com/philips/go-bindata-assetfs"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"mime"
	"net/http"
	"github.com/gofunct/service/gateway/swagger/data"
	"context"
	"github.com/spf13/viper"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	api "github.com/gofunct/service/api/todo/v1"
)

var (
	log *zap.Logger
)

func init() {
	var err error
	log, err = zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
}

func Serve() {
	log.Debug("Starting gateway service..")
	conn, err := grpc.Dial(viper.GetString("grpc_host")+viper.GetString("grpc_port"), grpc.WithInsecure())
	if err != nil {
		panic("Couldn't contact grpc server")
	}

	mux := runtime.NewServeMux()
	err = api.RegisterTodoServiceHandler(context.Background(), mux, conn)
	if err != nil {
		panic("Cannot serve http api")
	}
	http.ListenAndServe(viper.GetString("gw_port"), mux)
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