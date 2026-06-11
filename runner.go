package main

import (
	"fmt"
	"httper/pkg/finalize"
	"httper/pkg/request"
	"httper/pkg/script"
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

// Result captures one request execution for the run report and for response
// handler scripts.
type Result struct {
	Name       string
	StatusCode int
	Duration   time.Duration
	Err        error
	Header     http.Header
	Body       []byte
	Tests      []script.TestResult
}

// clientFor copies the base client and applies per-request directives. The
// copy shares the base transport unless the request needs HTTP/2 prior
// knowledge, which requires an explicit h2 transport.
func clientFor(base *http.Client, directives request.Directives, proto string) *http.Client {
	client := *base

	if directives.Timeout > 0 {
		client.Timeout = directives.Timeout
	}
	if directives.NoRedirect {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	if directives.NoCookieJar {
		client.Jar = nil
	}
	if strings.HasPrefix(proto, "HTTP/2") {
		client.Transport = &http2.Transport{}
	}

	return &client
}

func (r *Runner) Send(template *request.Template, httpRequest *http.Request) *Result {
	result := &Result{Name: template.Name}

	_, _ = fmt.Fprintln(r.Out, httpRequest.URL)

	client := clientFor(r.Client, template.Directives, httpRequest.Proto)

	start := time.Now()
	response, err := client.Do(httpRequest)
	result.Duration = time.Since(start)
	if err != nil {
		slog.Error("sending request", "err", err, "url", httpRequest.URL.String())
		result.Err = err
		return result
	}

	defer func() {
		if err := response.Body.Close(); err != nil {
			slog.Debug("closing response body", "err", err)
		}
	}()

	result.StatusCode = response.StatusCode
	result.Header = response.Header

	result.Body, err = io.ReadAll(response.Body)
	if err != nil {
		slog.Error("reading response body", "err", err)
		result.Err = err
		return result
	}

	if err := finalize.Response(
		r.Out,
		response,
		result.Body,
		result.Duration,
		finalize.Options{
			Save:    r.Config.Save,
			Verbose: r.Config.Verbose,
			Quiet:   template.Directives.NoLog,
		},
		r.SaveRoot,
	); err != nil {
		slog.Error("finalizing response", "err", err)
		result.Err = err
	}

	return result
}
