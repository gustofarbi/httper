package finalize

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// Options control what Response does with the already-read body.
type Options struct {
	Save    bool
	Verbose bool
	// Quiet suppresses everything but the one-line status (`# @no-log`);
	// saving still happens when Save is set.
	Quiet bool
}

// Summary is the protocol-agnostic view of a finished request that Print
// renders: HTTP responses and gRPC outcomes both reduce to it.
type Summary struct {
	StatusLine    string // e.g. "200 OK" or "14 Unavailable"
	StatusCode    int    // used in saved filenames
	Duration      time.Duration
	ContentLength int64
	Header        http.Header
	ContentType   string
}

// Response renders an HTTP response through Print.
func Response(
	w io.Writer,
	response *http.Response,
	body []byte,
	duration time.Duration,
	opts Options,
	root *os.Root,
) error {
	summary := Summary{
		StatusLine:    fmt.Sprintf("%d %s", response.StatusCode, http.StatusText(response.StatusCode)),
		StatusCode:    response.StatusCode,
		Duration:      duration,
		ContentLength: response.ContentLength,
		Header:        response.Header,
		ContentType:   response.Header.Get("Content-Type"),
	}

	return Print(w, summary, body, opts, root)
}

// Print optionally saves the body, then writes the status/duration summary
// (headers under Verbose, status line only under Quiet) and the body.
func Print(w io.Writer, summary Summary, body []byte, opts Options, root *os.Root) error {
	if opts.Save {
		if err := saveResponse(root, summary.StatusCode, summary.ContentType, body); err != nil {
			return fmt.Errorf("saving response: %w", err)
		}
	}

	if opts.Quiet {
		_, _ = fmt.Fprintf(w, "Status %s\n", summary.StatusLine)
		return nil
	}

	tw := tabwriter.NewWriter(w, 20, 20, 1, ' ', tabwriter.Debug)

	_, _ = fmt.Fprintln(tw)
	_, _ = fmt.Fprintf(tw, "Status\t%s\n", summary.StatusLine)
	_, _ = fmt.Fprintf(tw, "Duration\t%s\n", summary.Duration)
	_, _ = fmt.Fprintf(tw, "Content-Length\t%d\n", summary.ContentLength)

	// Print headers when verbose is enabled
	if opts.Verbose {
		_, _ = fmt.Fprintln(tw, "Headers\t")
		for key, values := range summary.Header {
			for _, value := range values {
				_, _ = fmt.Fprintf(tw, "  %s\t%s\n", key, value)
			}
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing tabwriter: %w", err)
	}

	// Print body
	if len(body) > 0 {
		_, _ = fmt.Fprintln(w, "\nResponse body:")

		bodyStr := string(body)
		if strings.Contains(summary.ContentType, "application/json") {
			PrettyPrintJSON(w, bodyStr)
		} else {
			_, _ = fmt.Fprintln(w, bodyStr)
		}
	}

	return nil
}
