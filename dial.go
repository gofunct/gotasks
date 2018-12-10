package service

import (
	"context"
	pb "github.com/philips/grpc-gateway-example/echopb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"os"
	"strings"
)

func WithDialFunc(port string) With {
	return func(c *Commander) {
		c.RootCmd.AddCommand(&cobra.Command{
			Use:   "dial",
			Short: "Dials the grpc server on https://localhost" + port,
			Run: func(cmd *cobra.Command, args []string) {
				var opts []grpc.DialOption
				creds := credentials.NewClientTLSFromCert(demoCertPool, "localhost"+port)
				opts = append(opts, grpc.WithTransportCredentials(creds))
				conn, err := grpc.Dial(demoAddr, opts...)
				if err != nil {
					grpclog.Fatalf("fail to dial: %v", err)
				}
				defer conn.Close()
				client := pb.NewEchoServiceClient(conn)

				msg, err := client.Echo(context.Background(), &pb.EchoMessage{strings.Join(os.Args[2:], " ")})
				println(msg.Value)

			}})
	}
}
