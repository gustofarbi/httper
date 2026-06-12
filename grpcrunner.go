package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gustofarbi/httper/pkg/finalize"
	"github.com/gustofarbi/httper/pkg/grpcrun"
	"github.com/gustofarbi/httper/pkg/request"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc/metadata"
)

// GRPCRunner is the gRPC sibling of Runner: it resolves a GRPC template at
// send time and executes it via pkg/grpcrun.
type GRPCRunner struct {
	Out      io.Writer
	Config   Config
	SaveRoot *os.Root
	// Timeout is the global -timeout default; a per-request @timeout wins.
	Timeout time.Duration
}

// metadataHopByHop are header names that only make sense for plain HTTP;
// everything else passes into gRPC metadata verbatim.
var metadataHopByHop = map[string]bool{
	"content-type":   true,
	"content-length": true,
	"connection":     true,
	"host":           true,
	"te":             true,
}

func (r *GRPCRunner) Send(template *request.Template, resolve func(string) string) *Result {
	result := &Result{Name: template.Name, GRPC: true}

	rawURL, headers, body := template.BuildGRPC(resolve)

	_, _ = fmt.Fprintln(r.Out, rawURL)

	target, err := grpcrun.ParseTarget(rawURL)
	if err != nil {
		slog.Error("parsing grpc target", "err", err, "request", template.Name)
		result.Err = err
		return result
	}

	timeout := r.Timeout
	if template.Directives.Timeout > 0 {
		timeout = template.Directives.Timeout
	}

	outcome, err := grpcrun.Invoke(context.Background(), target, body, grpcrun.Options{
		Insecure: r.Config.Insecure,
		Timeout:  timeout,
		Metadata: requestMetadata(headers),
	})
	if err != nil {
		slog.Error("sending grpc request", "err", err, "target", rawURL)
		result.Err = err
		return result
	}

	result.StatusCode = int(outcome.Code)
	result.Duration = outcome.Duration
	result.Header = metadataHeader(outcome.Header, outcome.Trailer)
	result.Body = responseBody(outcome)

	summary := finalize.Summary{
		StatusLine:    fmt.Sprintf("%d %s", outcome.Code, outcome.Code.String()),
		StatusCode:    int(outcome.Code),
		Duration:      outcome.Duration,
		ContentLength: int64(len(result.Body)),
		Header:        result.Header,
		ContentType:   "application/json",
	}

	opts := finalize.Options{
		Save:    r.Config.Save,
		Verbose: r.Config.Verbose,
		Quiet:   template.Directives.NoLog,
	}
	if err := finalize.Print(r.Out, summary, result.Body, opts, r.SaveRoot); err != nil {
		slog.Error("finalizing response", "err", err)
		result.Err = err
	}

	return result
}

// requestMetadata converts resolved header pairs into outgoing metadata,
// dropping HTTP-only and reserved grpc- names. No Basic-auth encoding happens
// here — gRPC metadata passes verbatim, matching the JetBrains client.
func requestMetadata(headers [][2]string) metadata.MD {
	md := metadata.MD{}
	for _, header := range headers {
		key := strings.ToLower(header[0])
		if metadataHopByHop[key] || strings.HasPrefix(key, "grpc-") {
			slog.Debug("dropping header from grpc metadata", "header", header[0])
			continue
		}
		md.Append(key, header[1])
	}

	return md
}

// metadataHeader merges response header and trailer metadata into an
// http.Header (canonicalized keys) so scripts read both via
// response.headers.valueOf.
func metadataHeader(parts ...metadata.MD) http.Header {
	header := http.Header{}
	for _, part := range parts {
		for key, values := range part {
			for _, value := range values {
				header.Add(key, value)
			}
		}
	}

	return header
}

// responseBody renders the outcome for printing and scripts: the single
// message for unary calls, a JSON array of messages for server streams.
func responseBody(outcome *grpcrun.Outcome) []byte {
	if !outcome.Streaming {
		if len(outcome.Messages) == 0 {
			return nil
		}
		return outcome.Messages[0]
	}

	return append([]byte("["), append(bytes.Join(outcome.Messages, []byte(",")), ']')...)
}
