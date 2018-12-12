package gateway

import (
	"context"
	api "github.com/gofunct/gotasks/api/todo/v1"
	"github.com/gofunct/gotasks/runtime/gateway/data"
	"github.com/gofunct/gotasks/runtime/logging"
	vi "github.com/gofunct/gotasks/runtime/viper"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/philips/go-bindata-assetfs"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"
	"mime"
	"net/http"
	"net/http/pprof"
)

var log = logging.Log(false)

func Serve() func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		tracer, closer, err := Trace()
		if err != nil {
			log.Zap.Fatal("Cannot initialize Jaeger Tracer %s", zap.Error(err))
		}
		defer closer.Close()

		conn, err := Connect(tracer)
		if err != nil {
			log.Fatal("failed to dial grpc backend", zap.Error(err))
		}
		gwmux := runtime.NewServeMux()
		mux := NewMux(gwmux)

		err = api.RegisterTodoServiceHandler(context.Background(), gwmux, conn)
		if err != nil {
			panic("Cannot serve http api")
		}

		if len(viper.GetStringSlice("domains")) > 0 {

			log.Zap.Debug("Starting secure gateway service at :443")
			serveTLS(mux)
		} else {
			log.Zap.Debug("Starting insecure gateway service at:", zap.String("gw_port", vi.VString("gw_port")))
			http.ListenAndServe(vi.VString("gw_port"), mux)
		}
	}
}

func serveTLS(mux *http.ServeMux) {
	log.Fatal(http.Serve(autocert.NewListener(vi.VString("domains")), mux))
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

func NewMux(gwmux *runtime.ServeMux) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", gwmux)
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	return mux
}
