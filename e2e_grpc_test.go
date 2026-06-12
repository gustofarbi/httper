package main

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"httper/internal/grpcecho"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grpcFixtureHost is the literal testdata/grpc.http targets (cmd/echo's gRPC
// port); e2e tests rewrite it to the in-process server's address.
const grpcFixtureHost = "localhost:8081"

func newGRPCServer(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpcecho.New()
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return listener.Addr().String()
}

func runGRPCContent(t *testing.T, address, content string) ([]*Result, string) {
	t.Helper()
	srv := newTestServer(t)
	return runResults(t, srv, strings.ReplaceAll(content, grpcFixtureHost, address), "testdata")
}

func TestE2EGRPC(t *testing.T) {
	address := newGRPCServer(t)

	t.Run("grpc.http fixture passes all tests", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/grpc.http")
		require.NoError(t, err)

		results, _ := runGRPCContent(t, address, string(raw))
		require.Len(t, results, 3)
		for _, result := range results {
			require.NoError(t, result.Err, result.Name)
			for _, test := range result.Tests {
				assert.False(t, test.Failed, "%s / %s: %s", result.Name, test.Name, test.Message)
			}
		}
	})

	t.Run("unary echo prints json and OK status", func(t *testing.T) {
		results, out := runGRPCContent(t, address, `
### grpc-echo
GRPC localhost:8081/grpcecho.EchoService/Echo
X-Token: secret

{"message": "hello"}
`)
		require.Len(t, results, 1)
		require.NoError(t, results[0].Err)
		assert.True(t, results[0].GRPC)
		assert.Equal(t, 0, results[0].StatusCode)
		assert.Contains(t, out, "0 OK")
		assert.Contains(t, out, `"hello"`)
		assert.Contains(t, out, `"secret"`) // metadata echoed back
	})

	t.Run("response handler script sees status and body", func(t *testing.T) {
		results, _ := runGRPCContent(t, address, `
### grpc-script
GRPC localhost:8081/grpcecho.EchoService/Echo

{"message": "scripted"}

> {%
client.test("grpc ok", function() {
  client.assert(response.status === 0, "expected OK status");
  client.assert(response.body.message === "scripted", "body mismatch");
  client.assert(response.headers.valueOf("x-echo-header") === "header-value", "missing header metadata");
});
%}
`)
		require.Len(t, results, 1)
		require.NoError(t, results[0].Err)
		require.Len(t, results[0].Tests, 1)
		assert.False(t, results[0].Tests[0].Failed, results[0].Tests[0].Message)
	})

	t.Run("request chaining via client.global", func(t *testing.T) {
		results, _ := runGRPCContent(t, address, `
### first
GRPC localhost:8081/grpcecho.EchoService/Echo

{"message": "chained-value"}

> {%
client.global.set("fromGrpc", response.body.message);
%}

### second
GRPC localhost:8081/grpcecho.EchoService/Echo

{"message": "{{fromGrpc}}"}

> {%
client.test("chained", function() {
  client.assert(response.body.message === "chained-value", "got " + response.body.message);
});
%}
`)
		require.Len(t, results, 2)
		require.Len(t, results[1].Tests, 1)
		assert.False(t, results[1].Tests[0].Failed, results[1].Tests[0].Message)
	})

	t.Run("non-OK status is result not error", func(t *testing.T) {
		results, out := runGRPCContent(t, address, `
### grpc-fail
GRPC localhost:8081/grpcecho.EchoService/Fail

{"code": 14, "message": "boom"}
`)
		require.Len(t, results, 1)
		require.NoError(t, results[0].Err)
		assert.Equal(t, 14, results[0].StatusCode)
		assert.Contains(t, out, "14 Unavailable")

		report := buildReport(results, false)
		assert.False(t, report.Failed())

		strictReport := buildReport(results, true)
		assert.True(t, strictReport.Failed())
	})

	t.Run("strict passes for OK grpc status", func(t *testing.T) {
		results, _ := runGRPCContent(t, address, `
### grpc-ok
GRPC localhost:8081/grpcecho.EchoService/Echo

{"message": "fine"}
`)
		report := buildReport(results, true)
		assert.False(t, report.Failed(), "gRPC status 0 must pass strict mode")
	})

	t.Run("server streaming prints every message", func(t *testing.T) {
		results, out := runGRPCContent(t, address, `
### grpc-stream
GRPC localhost:8081/grpcecho.EchoService/Countdown

{"count": 3}

> {%
client.test("three messages", function() {
  client.assert(response.body.length === 3, "got " + response.body.length);
  client.assert(response.body[0].value === 3, "first message");
});
%}
`)
		require.Len(t, results, 1)
		require.NoError(t, results[0].Err)
		require.Len(t, results[0].Tests, 1)
		assert.False(t, results[0].Tests[0].Failed, results[0].Tests[0].Message)
		assert.Contains(t, out, `"value"`)
	})

	t.Run("unknown service reports error", func(t *testing.T) {
		results, _ := runGRPCContent(t, address, `
### grpc-missing
GRPC localhost:8081/nope.Missing/Do
`)
		require.Len(t, results, 1)
		assert.Error(t, results[0].Err)

		report := buildReport(results, false)
		assert.True(t, report.Failed())
	})

	t.Run("timeout directive applies", func(t *testing.T) {
		// Dial a blackhole so reflection hangs until @timeout fires.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = listener.Close() })

		start := time.Now()
		results, _ := runGRPCContent(t, listener.Addr().String(), `
### grpc-timeout
# @timeout 1
GRPC localhost:8081/grpcecho.EchoService/Echo
`)
		require.Len(t, results, 1)
		assert.Error(t, results[0].Err)
		assert.Less(t, time.Since(start), 10*time.Second)
	})

	t.Run("no-log prints only the status line", func(t *testing.T) {
		_, out := runGRPCContent(t, address, `
### grpc-quiet
# @no-log
GRPC localhost:8081/grpcecho.EchoService/Echo

{"message": "hush"}
`)
		assert.Contains(t, out, "Status 0 OK")
		assert.NotContains(t, out, "hush")
	})
}
