package service

import (
	"context"
	"fmt"
	"github.com/gofunct/service/middleware"
	"github.com/gofunct/service/utils"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/heptiolabs/healthcheck"
	"github.com/mwitkow/go-conntrack"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	jcfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-client-go/rpcmetrics"
	jprom "github.com/uber/jaeger-lib/metrics/prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"
)

var (
	service = NewService("gofunct")
)

// BaseCommand provides the basic flags vars for running a service
func BaseCommand(serviceName, shortDescription string) *cobra.Command {
	command := &cobra.Command{
		Use:   serviceName,
		Short: shortDescription,
	}

	command.PersistentFlags().StringVar(&service.Config.Host, "host", "0.0.0.0", "gRPC service hostname")
	command.PersistentFlags().IntVar(&service.Config.Port, "port", 8000, "gRPC port")

	return command
}

// RegisterImplementation allows you to register your gRPC server
type RegisterImplementation func(s *grpc.Server)

// ServerConfig is a generic server configuration
type ServerConfig struct {
	Port int
	Host string
}

// Address Gets a logical addr for a ServerConfig
func (c *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Service is a gRPC based server with extra features
type Service struct {
	ID                 string
	Name               string
	UnaryInts          []grpc.UnaryServerInterceptor
	StreamInts         []grpc.StreamServerInterceptor
	GRPCImplementation RegisterImplementation
	GRPCOptions        []grpc.ServerOption
	Config             ServerConfig
	Logger             *zap.Logger
	GRPCServer         *grpc.Server
	HttpServer         *http.Server
	Flusher            func()

	healthcheck.Handler
}

// NewService creates a new service with a given name
func NewService(n string) *Service {
	return &Service{
		ID:                 utils.GenerateID(n),
		Name:               n,
		Config:             ServerConfig{Host: "0.0.0.0", Port: 8000},
		GRPCImplementation: func(s *grpc.Server) {},
		UnaryInts: []grpc.UnaryServerInterceptor{
			grpc_ctxtags.UnaryServerInterceptor(),
			grpc_prometheus.UnaryServerInterceptor,
			grpc_opentracing.UnaryServerInterceptor(),
			middleware.DebugLoggingInterceptor(),
			grpc_recovery.UnaryServerInterceptor(),
		},
		StreamInts: []grpc.StreamServerInterceptor{
			grpc_prometheus.StreamServerInterceptor,
			grpc_opentracing.StreamServerInterceptor(),
			grpc_recovery.StreamServerInterceptor(),
		},
	}
}

// GlobalService returns the global service
func GlobalService() *Service {
	return service
}

// Name sets the name for the service
func Name(n string) {
	service.ID = utils.GenerateID(n)
	service.Name = n
}

// Server attaches the gRPC implementation to the service
func Server(r func(s *grpc.Server)) {
	service.GRPCImplementation = r
}

// AddUnaryInterceptor adds a unary interceptor to the RPC server
func AddUnaryInterceptor(unint grpc.UnaryServerInterceptor) {
	service.UnaryInts = append(service.UnaryInts, unint)
}

// AddStreamInterceptor adds a stream interceptor to the RPC server
func AddStreamInterceptor(sint grpc.StreamServerInterceptor) {
	service.StreamInts = append(service.StreamInts, sint)
}

// URLForService returns a service URL via a registry or a simple DNS name
// if not available via the registry
func URLForService(name string) string {

	host := name
	port := "80"

	if val, ok := os.LookupEnv("SERVICE_HOST_OVERRIDE"); ok {
		host = val
	}
	if val, ok := os.LookupEnv("SERVICE_PORT_OVERRIDE"); ok {
		port = val
	}

	return fmt.Sprintf("%s:%s", host, port)

}

// Shutdown gracefully shuts down the gRPC and metrics servers
func (s *Service) Shutdown() {
	s.Logger.Info(fmt.Sprint(s.Name, "lile: Gracefully shutting down gRPC and Prometheus"))

	service.GRPCServer.GracefulStop()

	// 30 seconds is the default grace period in Kubernetes
	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()
	if err := service.HttpServer.Shutdown(ctx); err != nil {
		s.Logger.Debug("Timeout during shutdown of metrics server. Error: %v", zap.Error(err))
	}
}

func createGrpcServer() *grpc.Server {
	service.GRPCOptions = append(service.GRPCOptions, grpc.UnaryInterceptor(
		grpc_middleware.ChainUnaryServer(service.UnaryInts...)))

	service.GRPCOptions = append(service.GRPCOptions, grpc.StreamInterceptor(
		grpc_middleware.ChainStreamServer(service.StreamInts...)))

	service.GRPCServer = grpc.NewServer(
		service.GRPCOptions...,
	)

	service.GRPCImplementation(service.GRPCServer)

	grpc_prometheus.EnableHandlingTimeHistogram(
		func(opt *prometheus.HistogramOpts) {
			opt.Buckets = prometheus.ExponentialBuckets(0.005, 1.4, 20)
		},
	)

	grpc_prometheus.Register(service.GRPCServer)
	return service.GRPCServer
}

func (s *Service) Run() error {
	var err error
	if err = s.setGlobalJaegerTracer(); err != nil {
		return err
	}
	defer s.Flush()

	s.GRPCServer = createGrpcServer()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/ready", s.ReadyEndpoint)
	mux.HandleFunc("/live", s.LiveEndpoint)
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))

	service.HttpServer = &http.Server{
		Addr:    service.Config.Address(),
		Handler: s.DynamicRouter(mux),
	}
	s.Logger.Info(fmt.Sprint("serving on port: ", service.Config.Address()))

	return service.HttpServer.ListenAndServe()
}

func (s *Service) DynamicRouter(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			s.Logger.Info(fmt.Sprint("serving http handler- ", r.URL.String()))
			s.GRPCServer.ServeHTTP(w, r)
		} else {
			s.Logger.Info(fmt.Sprint("serving http handler- ", r.URL.String()))
			mux.ServeHTTP(w, r)
		}
	})
}

func (s *Service) GetGlobalTracer() opentracing.Tracer {
	return opentracing.GlobalTracer()
}

func (s *Service) Flush() {
	if s.Flusher != nil {
		s.Flusher()
	}
}

func (s *Service) setGlobalJaegerTracer() error {
	factory := jprom.New()

	cfg, err := jcfg.FromEnv()
	if err != nil {
		return err
	}
	cfg.ServiceName = s.Name
	cfg.Sampler = &jcfg.SamplerConfig{
		Type:  "const",
		Param: 1,
	}

	tracer, closer, err := cfg.NewTracer(
		jcfg.Logger(jaegerlog.StdLogger),
		jcfg.Metrics(factory),
		jcfg.Observer(rpcmetrics.NewObserver(factory, rpcmetrics.DefaultNameNormalizer)))

	if err != nil {
		return err
	}

	// Health check
	s.Handler.AddReadinessCheck(
		"jaeger",
		healthcheck.Timeout(func() error { return err }, time.Second*10))

	opentracing.SetGlobalTracer(tracer)
	s.Logger.Debug("global tracer set")

	s.Flusher = func() {
		if closer != nil {
			closer.Close()
		}
	}
	return err
}

func (s *Service) AddEvent(ctx context.Context, key, event string) {
	span, _ := opentracing.StartSpanFromContext(ctx, "say-hello")
	defer span.Finish()

	// Add tag
	span.SetTag(key, Event{
		event: event,
	})

	span.LogKV("event", "println")
}

type Event struct {
	event string
}

func WrapDefaultDialer(service string) {
	http.DefaultTransport.(*http.Transport).DialContext = conntrack.NewDialContextFunc(
		conntrack.DialWithTracing(),
		conntrack.DialWithName(service+"_dialer"),
		conntrack.DialWithDialer(&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}))
}

func (m *Service) CheckEndpoints(tcp, dns, http bool, urls ...string) {
	for _, url := range urls {
		if tcp {
			m.AddReadinessCheck("tcp_"+url, healthcheck.TCPDialCheck(url, 2*time.Second))
		}
		if dns {
			m.AddReadinessCheck("dns_"+url, healthcheck.DNSResolveCheck(url, 2*time.Second))
		}
		if dns {
			m.AddReadinessCheck("http_get_"+url, healthcheck.HTTPGetCheck(url, 2*time.Second))
		}

	}
}
