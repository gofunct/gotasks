package service

import (
	"context"
	"fmt"
	"github.com/opentracing/opentracing-go"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/go-pg/pg"
	api "github.com/gofunct/service/api/todo/v1"
	"github.com/gofunct/service/service/todo"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	grpc_runtime "github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/uber/jaeger-client-go/config"

	"github.com/philips/go-bindata-assetfs"
	"github.com/philips/grpc-gateway-example/pkg/ui/data/swagger"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Panic handler prints the stack trace when recovering from a panic.
var panicHandler = grpc_recovery.RecoveryHandlerFunc(func(p interface{}) error {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	log.Errorf("panic recovered: %+v", string(buf))
	return status.Errorf(codes.Internal, "%s", p)
})

func main() {
	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Usage = "Todo app"
	app.Version = "0.0.1"
	app.Flags = commonFlags
	app.Action = start

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func start(c *cli.Context) {
	lis, err := net.Listen("tcp", c.String("bind-grpc"))
	if err != nil {
		log.Fatalf("Failed to listen: %v", c.String("bind-grpc"))
	}
	logger := NewLogger()

	tracer, closer, err := NewTracer("todo", logger)
	if err != nil {
		logger.Fatalf("Cannot initialize Jaeger Tracer %s", err)
	}
	defer closer.Close()

	// Set GRPC Interceptors
	server := NewServer(logger, tracer)

	// Register Todo service, prometheus and HTTP service handler
	api.RegisterTodoServiceServer(server, &todo.Service{DB: NewDB(
		c.String("db-user"),
		c.String("db-password"),
		c.String("db-name"),
		c.String("db-host"),
		c.String("db-port"),
	)})

	go DebugMux(c.String("bind-debug-http"))

	log.Println("Starting Todo service..")
	go server.Serve(lis)

	conn, err := grpc.Dial(c.String("bind-grpc"), grpc.WithInsecure())
	if err != nil {
		panic("Couldn't contact grpc server")
	}

	mux := grpc_runtime.NewServeMux()
	err = api.RegisterTodoServiceHandler(context.Background(), mux, conn)
	if err != nil {
		panic("Cannot serve http api")
	}
	http.ListenAndServe(c.String("bind-http"), mux)
}

func DebugMux(port string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	http.ListenAndServe(port, mux)
}

func NewDB(user, pw, name, host, port string) *pg.DB {
	// Connect to PostgresQL
	db := pg.Connect(&pg.Options{
		User:                  user,
		Password:              pw,
		Database:              name,
		Addr:                  host + ":" + port,
		RetryStatementTimeout: true,
		MaxRetries:            4,
		MinRetryBackoff:       250 * time.Millisecond,
	})

	// Create Table from Todo struct generated by gRPC
	db.CreateTable(&api.Todo{}, nil)
	return db
}

func NewServer(logger *log.Entry, tracer opentracing.Tracer) *grpc.Server {
	interceptor := NewInterceptor(InterceptorOpts{true})

	s := grpc.NewServer(
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_opentracing.StreamServerInterceptor(grpc_opentracing.WithTracer(tracer)),
			interceptor.StreamServer(),
			grpc_logrus.StreamServerInterceptor(logger),
			grpc_recovery.StreamServerInterceptor(grpc_recovery.WithRecoveryHandler(panicHandler)),
		)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_opentracing.UnaryServerInterceptor(grpc_opentracing.WithTracer(tracer)),
			interceptor.UnaryServer(),
			grpc_logrus.UnaryServerInterceptor(logger),
			grpc_recovery.UnaryServerInterceptor(grpc_recovery.WithRecoveryHandler(panicHandler)),
		)),
	)

	grpc_health_v1.RegisterHealthServer(s, health.NewServer())
	grpc_prometheus.Register(s)
	RegisterInterceptor(s, interceptor)
	return s
}

func NewTracer(name string, entry *log.Entry) (opentracing.Tracer, io.Closer, error) {
	// Jaeger tracing
	cfg := config.Configuration{
		Sampler: &config.SamplerConfig{
			Type:  "const",
			Param: 1,
		},
		Reporter: &config.ReporterConfig{
			LocalAgentHostPort: "127.0.0.1" + ":" + "5775",
		},
		RPCMetrics: true,
	}

	return cfg.New(
		name,
		config.Logger(jaegerLoggerAdapter{entry}),
	)
}

func NewLogger() *log.Entry {
	// Logrus
	logger := log.NewEntry(log.New())
	grpc_logrus.ReplaceGrpcLogger(logger)
	log.SetLevel(log.InfoLevel)
	return logger
}

type jaegerLoggerAdapter struct {
	logger *log.Entry
}

func (l jaegerLoggerAdapter) Error(msg string) {
	l.logger.Error(msg)
}

func (l jaegerLoggerAdapter) Infof(msg string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(msg, args...))
}

func GrpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			otherHandler.ServeHTTP(w, r)
		}
	})
}

func ServeSwagger(mux *http.ServeMux, dir string) {
	mime.AddExtensionType(".svg", "image/svg+xml")

	fileServer := http.FileServer(&assetfs.AssetFS{
		Asset:    swagger.Asset,
		AssetDir: swagger.AssetDir,
		Prefix:   dir,
	})
	prefix := "/swagger-ui/"
	mux.Handle(prefix, http.StripPrefix(prefix, fileServer))
}