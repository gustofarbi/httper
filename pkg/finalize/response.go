package finalize

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

func Response(
	response *http.Response,
	duration time.Duration,
	save, verbose bool,
	root *os.Root,
) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		slog.Error("reading response body", "err", err)
		return
	}

	if save {
		if err := saveResponse(root, response, body); err != nil {
			slog.Error("saving response", "err", err)
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 20, 20, 1, ' ', tabwriter.Debug)

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Status\t%d %s\n", response.StatusCode, http.StatusText(response.StatusCode))
	_, _ = fmt.Fprintf(w, "Duration\t%s\n", duration)
	_, _ = fmt.Fprintf(w, "Content-Length\t%d\n", response.ContentLength)

	// Print headers when verbose is enabled
	if verbose {
		_, _ = fmt.Fprintln(w, "Headers\t")
		for key, values := range response.Header {
			for _, value := range values {
				_, _ = fmt.Fprintf(w, "  %s\t%s\n", key, value)
			}
		}
	}

	if err := w.Flush(); err != nil {
		slog.Error("flushing tabwriter", "err", err)
	}

	// Print body
	if len(body) > 0 {
		fmt.Println("\nResponse body:")

		contentType := response.Header.Get("Content-Type")
		bodyStr := string(body)

		if strings.Contains(contentType, "application/json") {
			PrettyPrintJSON(bodyStr)
		} else {
			fmt.Println(bodyStr)
		}
	}
}
