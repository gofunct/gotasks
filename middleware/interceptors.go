package middleware

import (
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"golang.org/x/time/rate"
	"strings"
	"context"
	"net"
)

// ContextClientInterceptor passes around headers for tracing and linkerd
func ContextClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, resp interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		pairs := make([]string, 0)

		if md, ok := metadata.FromIncomingContext(ctx); ok {
			for key, values := range md {
				if strings.HasPrefix(strings.ToLower(key), "l5d") {
					for _, value := range values {
						pairs = append(pairs, key, value)
					}
				}

				if strings.HasPrefix(strings.ToLower(key), "x-") {
					for _, value := range values {
						pairs = append(pairs, key, value)
					}
				}
			}
		}

		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
		return invoker(ctx, method, req, resp, cc, opts...)
	}
}

func DebugLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		fmt.Println(info, "requst", req)
		resp, err := handler(ctx, req)
		fmt.Println(info, "response", resp, "err", err)
		return resp, err
	}
}

func RateLimitingServerInterceptor(r rate.Limit, txn int) grpc.UnaryServerInterceptor {
	limiter := rate.NewLimiter(r, txn)
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := limiter.Wait(ctx); err != nil {
			return nil, status.Error(codes.Canceled, "context exceeded")
		}
		return handler(ctx, req)
	}
}

// CheckClientIsLocal checks that request comes from the local networks
func CheckClientIsLocal(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	localNetworks := []*net.IPNet{
		&net.IPNet{IP: net.IP{0x7f, 0x0, 0x0, 0x0}, Mask: net.IPMask{0xff, 0x0, 0x0, 0x0}},
		&net.IPNet{IP: net.IP{0xa, 0x0, 0x0, 0x0}, Mask: net.IPMask{0xff, 0x0, 0x0, 0x0}},
		&net.IPNet{IP: net.IP{0x64, 0x40, 0x0, 0x0}, Mask: net.IPMask{0xff, 0xc0, 0x0, 0x0}},
		&net.IPNet{IP: net.IP{0xac, 0x10, 0x0, 0x0}, Mask: net.IPMask{0xff, 0xf0, 0x0, 0x0}},
		&net.IPNet{IP: net.IP{0xc0, 0xa8, 0x0, 0x0}, Mask: net.IPMask{0xff, 0xff, 0x0, 0x0}},
	}
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.PermissionDenied, "peer info not-found")
	}
	addr, ok := p.Addr.(*net.TCPAddr)
	if !ok {
		return nil, status.Errorf(codes.PermissionDenied, "broken peer info")
	}
	for _, n := range localNetworks {
		if n.Contains(addr.IP.To4()) {
			return handler(ctx, req)
		}
	}
	return nil, status.Errorf(codes.PermissionDenied, "Request must be from local. Your IP: %s", addr.IP)
}