package gateway

import (
	"github.com/opentracing/opentracing-go"
	jconfig "github.com/uber/jaeger-client-go/config"
	"io"
)

func Trace() (opentracing.Tracer, io.Closer, error) {
	var err error
	cfg, err := jconfig.FromEnv()
	if err != nil {
		return nil, nil, err
	}
	cfg.ServiceName = "goservice_gateway"
	cfg.RPCMetrics = true
	tracer, closer, err := cfg.NewTracer(jconfig.Logger(log))
	if err != nil {
		return nil, nil, err
	}
	opentracing.SetGlobalTracer(tracer)
	log.Zap.Debug("global tracer successfully registered")
	return tracer, closer, err
}
