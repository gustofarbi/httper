package grpcecho

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
)

func startServer(t *testing.T) *grpc.ClientConn {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := New()
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func TestEchoRoundTrip(t *testing.T) {
	conn := startServer(t)
	client := NewEchoServiceClient(conn)

	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-token", "secret")
	response, err := client.Echo(ctx, &EchoRequest{Message: "hello"})
	require.NoError(t, err)

	assert.Equal(t, "hello", response.Message)
	assert.Equal(t, "secret", response.Metadata["x-token"])
}

func TestCountdownStreams(t *testing.T) {
	conn := startServer(t)
	client := NewEchoServiceClient(conn)

	stream, err := client.Countdown(context.Background(), &CountdownRequest{Count: 3})
	require.NoError(t, err)

	var values []int32
	for {
		message, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		values = append(values, message.Value)
	}
	assert.Equal(t, []int32{3, 2, 1}, values)
}

func TestReflectionRegistered(t *testing.T) {
	conn := startServer(t)
	client := grpc_reflection_v1.NewServerReflectionClient(conn)

	stream, err := client.ServerReflectionInfo(context.Background())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{},
	}))

	response, err := stream.Recv()
	require.NoError(t, err)

	var names []string
	for _, service := range response.GetListServicesResponse().GetService() {
		names = append(names, service.Name)
	}
	assert.Contains(t, names, "grpcecho.EchoService")
}
