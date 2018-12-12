package gateway

import (
	"context"
	mygrpc "github.com/gofunct/service/runtime/grpc"
	vi "github.com/gofunct/service/runtime/viper"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"net"
	"time"
)

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

	return grpc.DialContext(ctx, vi.VString("grpc_port"), dialOpts...)
}
