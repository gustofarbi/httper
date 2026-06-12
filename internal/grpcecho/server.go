// Package grpcecho is the gRPC counterpart of internal/echo: a
// reflection-enabled echo service shared by cmd/echo and the in-process
// end-to-end tests, one method per fixture scenario.
package grpcecho

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// New returns a plaintext gRPC server with the echo service and server
// reflection registered — the single source of truth for cmd/echo and the
// in-process e2e tests.
func New() *grpc.Server {
	server := grpc.NewServer()
	RegisterEchoServiceServer(server, &service{})
	reflection.Register(server)

	return server
}

type service struct {
	UnimplementedEchoServiceServer
}

// Echo returns the message plus the incoming metadata, and sets a response
// header and trailer so clients can assert on metadata.
func (*service) Echo(ctx context.Context, request *EchoRequest) (*EchoResponse, error) {
	incoming := map[string]string{}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for key, values := range md {
			if len(values) > 0 {
				incoming[key] = values[0]
			}
		}
	}

	_ = grpc.SetHeader(ctx, metadata.Pairs("x-echo-header", "header-value"))
	_ = grpc.SetTrailer(ctx, metadata.Pairs("x-echo-trailer", "trailer-value"))

	return &EchoResponse{Message: request.Message, Metadata: incoming}, nil
}

func (*service) Fail(_ context.Context, request *FailRequest) (*EchoResponse, error) {
	message := request.Message
	if message == "" {
		message = "requested failure"
	}

	return nil, status.Error(codes.Code(request.Code), message)
}

func (*service) Countdown(request *CountdownRequest, stream EchoService_CountdownServer) error {
	for value := request.Count; value > 0; value-- {
		if err := stream.Send(&CountdownResponse{Value: value}); err != nil {
			return err
		}
	}

	return nil
}
