package main

import (
	"github.com/gofunct/service"
	pb "github.com/gofunct/service/test"
	"go.uber.org/zap"
	"net"
)

var (
	example  *service.Service
	logger   *zap.Logger
	listener net.Listener
)

func init() {
	var err error
	logger, err = zap.NewDevelopment()
	if err != nil {
		return
	}

	example = service.NewService(8000, logger, "0.0.0.0", "example")
}

func Listen(s *service.Service) {
	defer s.Flush()
	// Create a new api server.
	demoServer := pb.NewDemoServer()

	// Register your service.
	pb.RegisterDemoServiceServer(s.GRPCServer, demoServer)

	s.Run()
}

func Ping(s *service.Service) {
	conn := service.NewClientConn(example)
	go func() { pb.Ping(conn) }()
}

func main() {
	Listen(example)
	Ping(example)
}
