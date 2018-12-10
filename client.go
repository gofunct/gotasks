package service

import (
	"fmt"
	"github.com/gofunct/service/middleware"
	"google.golang.org/grpc"
	"log"
)

func NewClientConn(s *Service) *grpc.ClientConn {
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%v", s.Config.Port),
		grpc.WithUnaryInterceptor(middleware.ContextClientInterceptor()),
		grpc.WithInsecure(),
	)
	if err != nil {
		log.Fatal(err)
	}
	return conn
}
