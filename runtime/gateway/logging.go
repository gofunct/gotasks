package gateway

import (
	"go.uber.org/zap"
)

var log = Log()

func Log() *zap.Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	return logger
}
