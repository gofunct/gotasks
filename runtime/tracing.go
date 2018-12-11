package runtime

import (
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/viper"
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
	cfg.ServiceName = viper.GetString("use")
	cfg.RPCMetrics = true
	tracer, closer, err := cfg.NewTracer(jconfig.Logger(log))
	if err != nil {
		return nil, nil, err
	}
	opentracing.SetGlobalTracer(tracer)
	log.Infof("global tracer successfully registered")
	return tracer, closer, err
}
