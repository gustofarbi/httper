package grpcrun

import (
	"context"
	"net"
	"testing"
	"time"

	"httper/internal/grpcecho"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

func startEcho(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpcecho.New()
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return listener.Addr().String()
}

func echoTarget(t *testing.T, address, method string) Target {
	t.Helper()

	target, err := ParseTarget("grpc://" + address + "/grpcecho.EchoService/" + method)
	require.NoError(t, err)

	return target
}

func TestInvokeUnary(t *testing.T) {
	address := startEcho(t)

	outcome, err := Invoke(
		context.Background(),
		echoTarget(t, address, "Echo"),
		`{"message": "hello"}`,
		Options{Metadata: metadata.Pairs("x-token", "secret")},
	)
	require.NoError(t, err)

	assert.Equal(t, codes.OK, outcome.Code)
	require.Len(t, outcome.Messages, 1)
	assert.Contains(t, string(outcome.Messages[0]), `"hello"`)
	assert.Contains(t, string(outcome.Messages[0]), `"secret"`)
	assert.Equal(t, []string{"header-value"}, outcome.Header.Get("x-echo-header"))
	assert.Equal(t, []string{"trailer-value"}, outcome.Trailer.Get("x-echo-trailer"))
	assert.Positive(t, outcome.Duration)
}

func TestInvokeEmptyBody(t *testing.T) {
	address := startEcho(t)

	outcome, err := Invoke(context.Background(), echoTarget(t, address, "Echo"), "", Options{})
	require.NoError(t, err)
	assert.Equal(t, codes.OK, outcome.Code)
	require.Len(t, outcome.Messages, 1)
}

func TestInvokeNonOKStatusIsNotError(t *testing.T) {
	address := startEcho(t)

	outcome, err := Invoke(
		context.Background(),
		echoTarget(t, address, "Fail"),
		`{"code": 14, "message": "boom"}`,
		Options{},
	)
	require.NoError(t, err)

	assert.Equal(t, codes.Unavailable, outcome.Code)
	assert.Equal(t, "boom", outcome.Message)
	assert.Empty(t, outcome.Messages)
}

func TestInvokeServerStreaming(t *testing.T) {
	address := startEcho(t)

	var streamed [][]byte
	outcome, err := Invoke(
		context.Background(),
		echoTarget(t, address, "Countdown"),
		`{"count": 3}`,
		Options{OnMessage: func(message []byte) { streamed = append(streamed, message) }},
	)
	require.NoError(t, err)

	assert.Equal(t, codes.OK, outcome.Code)
	require.Len(t, outcome.Messages, 3)
	assert.Len(t, streamed, 3)
	assert.Contains(t, string(outcome.Messages[0]), "3")
	assert.Contains(t, string(outcome.Messages[2]), "1")
}

func TestInvokeUnknownService(t *testing.T) {
	address := startEcho(t)

	target, err := ParseTarget("grpc://" + address + "/nope.Missing/Do")
	require.NoError(t, err)

	_, err = Invoke(context.Background(), target, "", Options{})
	assert.ErrorContains(t, err, "nope.Missing")
}

func TestInvokeUnknownMethod(t *testing.T) {
	address := startEcho(t)

	_, err := Invoke(context.Background(), echoTarget(t, address, "Nope"), "", Options{})
	assert.ErrorContains(t, err, "Nope")
}

func TestInvokeClientStreamingRejected(t *testing.T) {
	address := startEcho(t)

	_, err := Invoke(context.Background(), echoTarget(t, address, "Collect"), "", Options{})
	assert.ErrorContains(t, err, "streaming")
}

func TestInvokeBadBody(t *testing.T) {
	address := startEcho(t)

	_, err := Invoke(context.Background(), echoTarget(t, address, "Echo"), `{"nope": true}`, Options{})
	assert.Error(t, err)
}

func TestInvokeTimeout(t *testing.T) {
	// Dial a blackhole: reflection will hang until the deadline.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	target, err := ParseTarget("grpc://" + listener.Addr().String() + "/pkg.Service/Method")
	require.NoError(t, err)

	start := time.Now()
	_, err = Invoke(context.Background(), target, "", Options{Timeout: 200 * time.Millisecond})
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}
