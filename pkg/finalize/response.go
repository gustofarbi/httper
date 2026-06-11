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

func Response(
	w io.Writer,
	response *http.Response,
	body []byte,
	duration time.Duration,
	opts Options,
	root *os.Root,
) error {
	if opts.Save {
		if err := saveResponse(root, response, body); err != nil {
			return fmt.Errorf("saving response: %w", err)
		}
	}

	if opts.Quiet {
		_, _ = fmt.Fprintf(w, "Status %d %s\n", response.StatusCode, http.StatusText(response.StatusCode))
		return nil
	}

	tw := tabwriter.NewWriter(w, 20, 20, 1, ' ', tabwriter.Debug)

	_, _ = fmt.Fprintln(tw)
	_, _ = fmt.Fprintf(tw, "Status\t%d %s\n", response.StatusCode, http.StatusText(response.StatusCode))
	_, _ = fmt.Fprintf(tw, "Duration\t%s\n", duration)
	_, _ = fmt.Fprintf(tw, "Content-Length\t%d\n", response.ContentLength)

	// Print headers when verbose is enabled
	if opts.Verbose {
		_, _ = fmt.Fprintln(tw, "Headers\t")
		for key, values := range response.Header {
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

		contentType := response.Header.Get("Content-Type")
		bodyStr := string(body)

		if strings.Contains(contentType, "application/json") {
			PrettyPrintJSON(w, bodyStr)
		} else {
			_, _ = fmt.Fprintln(w, bodyStr)
		}
	}

	return nil
}
