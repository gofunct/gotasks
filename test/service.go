package proto

import (
	"bufio"
	"fmt"
	"context"
	"google.golang.org/grpc"
	"log"
	"os"
	"strings"
	"time"
)

// DemoServiceServer defines a Server.
type ServiceServer struct{}

func NewDemoServer() *ServiceServer {
	return &ServiceServer{}
}

// SayHello implements a interface defined by protobuf.
func (s *ServiceServer) SayHello(ctx context.Context, request *HelloRequest) (*HelloResponse, error) {
	return &HelloResponse{Message: fmt.Sprintf("Hello %s", request.Name)}, nil
}

func Ping(conn *grpc.ClientConn) {
	// Create a gRPC server client.
	client := NewDemoServiceClient(conn)
	fmt.Println("Start to call the method called SayHello every 3 seconds")
	go func() {
		for {
			// Call “SayHello” method and wait for response from gRPC Server.
			_, err := client.SayHello(context.Background(), &HelloRequest{Name: "Test"})
			if err != nil {
				log.Printf("Calling the SayHello method unsuccessfully. ErrorInfo: %+v", err)
				log.Printf("You should to stop the process")
				return
			}
			time.Sleep(3 * time.Second)
		}
	}()
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("You can press n or N to stop the process of client")
	for scanner.Scan() {
		if strings.ToLower(scanner.Text()) == "n" {
			os.Exit(0)
		}
	}
}

