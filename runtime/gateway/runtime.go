package gateway

import (
	"context"
	api "github.com/gofunct/service/runtime/api/todo/v1"
	"github.com/gofunct/service/runtime/gateway/data"
	mygrpc "github.com/gofunct/service/runtime/grpc"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/opentracing/opentracing-go"
	"github.com/philips/go-bindata-assetfs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"mime"
	"net"
	"net/http"
	"net/http/pprof"
	"time"
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
		tracer, closer, err := Trace()
		if err != nil {
			log.Zap.Fatal("Cannot initialize Jaeger Tracer %s", zap.Error(err))
		}
		defer closer.Close()

		log.Zap.Debug("Starting gateway service..")
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

func Connect(tracer opentracing.Tracer) (*grpc.ClientConn, error) {
	interceptor := mygrpc.NewMetricsIntercept()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := []grpc_zap.Option{
		grpc_zap.WithDurationField(func(duration time.Duration) zapcore.Field {
			return zap.Int64("grpc.time_ns", duration.Nanoseconds())
		}),
	}
	streamInterceptors := grpc.StreamClientInterceptor(grpc_middleware.ChainStreamClient(
		grpc_zap.StreamClientInterceptor(log.Zap, opts...),
		grpc_opentracing.StreamClientInterceptor(grpc_opentracing.WithTracer(tracer)),
		interceptor.StreamClient(),
	))

	unaryInterceptors := grpc.UnaryClientInterceptor(grpc_middleware.ChainUnaryClient(
		grpc_zap.UnaryClientInterceptor(log.Zap, opts...),
		grpc_opentracing.UnaryClientInterceptor(grpc_opentracing.WithTracer(tracer)),
		interceptor.UnaryClient(),
	))

	dialOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(unaryInterceptors),
		grpc.WithStreamInterceptor(streamInterceptors),
		grpc.WithStatsHandler(interceptor),
		grpc.WithDialer(interceptor.Dialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("tcp", addr, timeout)
		}))}

	prometheus.DefaultRegisterer.Register(interceptor)

	return grpc.DialContext(ctx, viper.GetString("grpc_port"), dialOpts...)

}

