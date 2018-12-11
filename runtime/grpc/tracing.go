package grpc

import (
	"github.com/opentracing/opentracing-go"
	jconfig "github.com/uber/jaeger-client-go/config"
	zapjaeger "github.com/uber/jaeger-client-go/log/zap"
	"io"
)

func Trace(log *zapjaeger.Logger) (opentracing.Tracer, io.Closer, error) {
	var err error
	cfg, err := jconfig.FromEnv()
	if err != nil {
		return nil, nil, err
	}
	cfg.ServiceName = "goservice_grpc"
	cfg.RPCMetrics = true
	tracer, closer, err := cfg.NewTracer(jconfig.Logger(log))
	if err != nil {
		return nil, nil, err
	}
	opentracing.SetGlobalTracer(tracer)
	log.Infof("global tracer successfully registered")
	return tracer, closer, err
}
