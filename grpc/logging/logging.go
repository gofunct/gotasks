package logging

import (
	"fmt"
	"go.uber.org/zap"
	"google.golang.org/grpc/grpclog"
	"github.com/spf13/viper"
	zapjaeger "github.com/uber/jaeger-client-go/log/zap"
)

func Log() *Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	zgl := &Logger{
		Zap:  logger.With(zap.String("name", viper.GetString("name")), zap.Bool("grpc_log", true)),
		JZap: zapjaeger.NewLogger(logger),
	}
	grpclog.SetLogger(zgl)
	return zgl
}

type Logger struct {
	Zap *zap.Logger
	JZap *zapjaeger.Logger
}

func (l *Logger) Fatal(args ...interface{}) {
	l.Zap.Fatal(fmt.Sprint(args...))
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

func (l *Logger) FatalViper(message, key string, args ...interface{}) {
	l.Zap.Fatal(message, zap.String(key, viper.GetString(key)))
}

func (l *Logger) DebugViper(message, key string, args ...interface{}) {
	l.Zap.Debug(message, zap.String(key, viper.GetString(key)))
}

