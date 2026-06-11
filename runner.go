package main

import (
	"fmt"
	"httper/pkg/finalize"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

// Runner executes parsed requests against an injectable HTTP client and writes
// their output to Out. Pulling these dependencies out of package-level globals
// lets tests drive it with a test server client and a buffer.
type Runner struct {
	Client   *http.Client
	Out      io.Writer
	Config   Config
	SaveRoot *os.Root
}

func (r *Runner) Send(httpRequest *http.Request) {
	_, _ = fmt.Fprintln(r.Out, httpRequest.URL)

	// HTTP/2 (prior knowledge) needs an explicit h2 transport. For every other
	// protocol leave the client's transport as injected — overwriting it would
	// discard a caller-provided transport (e.g. a test server's TLS config).
	if strings.HasPrefix(httpRequest.Proto, "HTTP/2") {
		r.Client.Transport = &http2.Transport{}
	}

	start := time.Now()
	response, err := r.Client.Do(httpRequest)
	if err != nil {
		slog.Error("sending request", "err", err, "url", httpRequest.URL.String())
		return
	}

	defer func() {
		if err := response.Body.Close(); err != nil {
			slog.Debug("closing response body", "err", err)
		}
	}()

	if err := finalize.Response(
		r.Out,
		response,
		time.Since(start),
		r.Config.Save,
		r.Config.Verbose,
		r.SaveRoot,
	); err != nil {
		slog.Error("finalizing response", "err", err)
	}
}
