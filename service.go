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
	jcfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-client-go/rpcmetrics"
	jprom "github.com/uber/jaeger-lib/metrics/prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"
)

type Opt func(s *Service)

// RegisterImplementation allows you to register your gRPC server
type RegisterImplementation func(s *grpc.Server)

// ServerConfig is a generic server configuration
type Config struct {
	Port int
	Host string
	Name string
}

// Address Gets a logical addr for a ServerConfig
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Service is a gRPC based server with extra features
type Service struct {
	ID          string
	UnaryInts   []grpc.UnaryServerInterceptor
	StreamInts  []grpc.StreamServerInterceptor
	GRPCOptions []grpc.ServerOption
	Config      Config
	GRPCServer  *grpc.Server
	HttpServer  *http.Server
	Flusher     func()
	Logger      *zap.Logger

	healthcheck.Handler
}

func WithConfig(port int, logger *zap.Logger, host, name string) Opt {
	return func(s *Service) {
		s.Config = Config{Host: host, Port: port}
		s.Logger.Debug("service config set")

	}
}

// NewService creates a new s with a given name
func NewService(port int, logger *zap.Logger, host, name string, opts ...grpc.ServerOption) *Service {
	server := &Service{
		ID: utils.GenerateID(name),
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
		GRPCOptions: opts,
		Config: Config{
			Port: port,
			Host: host,
			Name: name,
		},
		Logger:  logger,
		Handler: healthcheck.NewMetricsHandler(prometheus.DefaultRegisterer, name),
	}

	server = SetTracer(server)

	server = SetTransport(server)

	server.Logger.Debug("service configured")

	return server
}

// AddUnaryInterceptor adds a unary interceptor to the RPC server
func (s *Service) AddUnaryInterceptor(unint grpc.UnaryServerInterceptor) {
	s.UnaryInts = append(s.UnaryInts, unint)
}

// AddStreamInterceptor adds a stream interceptor to the RPC server
func (s *Service) AddStreamInterceptor(sint grpc.StreamServerInterceptor) {
	s.StreamInts = append(s.StreamInts, sint)
}

// Shutdown gracefully shuts down the gRPC and metrics servers
func (s *Service) Shutdown() {
	s.Logger.Info(fmt.Sprint(s.Config.Name, "lile: Gracefully shutting down gRPC and Prometheus"))

	s.GRPCServer.GracefulStop()

	// 30 seconds is the default grace period in Kubernetes
	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()
	if err := s.HttpServer.Shutdown(ctx); err != nil {
		s.Logger.Debug("Timeout during shutdown of metrics server. Error: %v", zap.Error(err))
	}
}

func (s *Service) createGrpcServer() *grpc.Server {
	s.GRPCOptions = append(s.GRPCOptions, grpc.UnaryInterceptor(
		grpc_middleware.ChainUnaryServer(s.UnaryInts...)))

	s.GRPCOptions = append(s.GRPCOptions, grpc.StreamInterceptor(
		grpc_middleware.ChainStreamServer(s.StreamInts...)))

	s.GRPCServer = grpc.NewServer(
		s.GRPCOptions...,
	)

	grpc_prometheus.EnableHandlingTimeHistogram(
		func(opt *prometheus.HistogramOpts) {
			opt.Buckets = prometheus.ExponentialBuckets(0.005, 1.4, 20)
		},
	)

	grpc_prometheus.Register(s.GRPCServer)

	return s.GRPCServer
}

func SetTransport(s *Service) *Service {

	s.GRPCServer = s.createGrpcServer()
	s.Logger.Debug("service grpc server set")

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/ready", s.ReadyEndpoint)
	mux.HandleFunc("/live", s.LiveEndpoint)
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))

	s.HttpServer = &http.Server{
		Addr:    s.Config.Address(),
		Handler: s.DynamicRouter(s.GRPCServer, mux),
	}
	s.Logger.Debug("service http server set")

	return s
}

func (s *Service) Run() {
	log.Fatal(s.HttpServer.ListenAndServe())
}

func (s *Service) DynamicRouter(server *grpc.Server, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			s.GRPCServer.ServeHTTP(w, r)
		} else {
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

func SetTracer(s *Service) *Service {
	factory := jprom.New()
	cfg, err := jcfg.FromEnv()
	if err != nil {
		panic(err)
	}
	cfg.ServiceName = s.Config.Name
	cfg.Sampler = &jcfg.SamplerConfig{
		Type:  "const",
		Param: 1,
	}
	tracer, closer, err := cfg.NewTracer(
		jcfg.Logger(jaegerlog.StdLogger),
		jcfg.Metrics(factory),
		jcfg.Observer(rpcmetrics.NewObserver(factory, rpcmetrics.DefaultNameNormalizer)))

	if err != nil {
		panic(err)
	}

	opentracing.SetGlobalTracer(tracer)

	s.Logger.Debug("global tracer set")

	s.Flusher = func() {
		if closer != nil {
			closer.Close()
		}
	}
	return s
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

func WrapDefaultDialer(s string) {
	http.DefaultTransport.(*http.Transport).DialContext = conntrack.NewDialContextFunc(
		conntrack.DialWithTracing(),
		conntrack.DialWithName(s+"_dialer"),
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
