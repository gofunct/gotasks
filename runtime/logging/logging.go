package logging

import (
	"fmt"
	"github.com/spf13/viper"
	zapjaeger "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"google.golang.org/grpc/grpclog"
)

var GrpcLog = Log(true)
var GatewayLog = Log(false)

func Log(g bool) *Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	var zgl *Logger

	if g {
		zgl = &Logger{
			Zap:  logger.With(zap.String("service", "grpc"), zap.Bool("grpc_log", true), zap.Any("config", viper.AllSettings())),
			JZap: zapjaeger.NewLogger(logger),
		}
		zap.ReplaceGlobals(zgl.Zap)

		grpclog.SetLogger(zgl)
	} else {
		zgl = &Logger{
			Zap:  logger.With(zap.String("service", "gateway"), zap.Bool("grpc_log", false), zap.Any("config", viper.AllSettings())),
			JZap: zapjaeger.NewLogger(logger),
		}
		zap.ReplaceGlobals(zgl.Zap)
	}

	return zgl
}

func LogViper() *zap.Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	return logger.With(zap.String("service", "viper"), zap.Any("config", viper.AllSettings()))
}

type Logger struct {
	Zap  *zap.Logger
	JZap *zapjaeger.Logger
}

func (l *Logger) Fatal(args ...interface{}) {
	var msg string
	var err error
	for _, e := range args {
		switch x := e.(type) {
		case string:
			msg = x
			continue
		case error:
			err = x
			continue
		}
	}
	l.Zap.Fatal(msg, zap.Error(err), zap.Any("config", viper.AllSettings()))
}

func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Zap.Fatal(fmt.Sprintf(format, args...))
}

func (l *Logger) Fatalln(args ...interface{}) {
	l.Zap.Fatal(fmt.Sprint(args...))
}

func (l *Logger) Print(args ...interface{}) {
	l.Zap.Info(fmt.Sprint(args...))
}

func (l *Logger) Printf(format string, args ...interface{}) {
	l.Zap.Info(fmt.Sprintf(format, args...))
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.Zap.Info(fmt.Sprintf(format, args...))
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Zap.Info(fmt.Sprintf(format, args...))
}

func (l *Logger) Error(msg string) {
	l.Zap.Info(fmt.Sprint(msg))
}

func (l *Logger) Println(args ...interface{}) {
	l.Zap.Info(fmt.Sprint(args...))
}
