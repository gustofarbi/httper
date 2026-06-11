package main

import (
	"fmt"
	"io"
)

// Report aggregates a run for the summary line and the exit code.
type Report struct {
	Requests    int
	Tests       int
	FailedTests int
	Errors      int
}

// Failed reports whether the run should exit non-zero.
func (r Report) Failed() bool {
	return r.FailedTests > 0 || r.Errors > 0
}

// buildReport tallies results. With strict set, a request whose final status
// is outside 2xx counts as an error even without a failing test.
func buildReport(results []*Result, strict bool) Report {
	report := Report{Requests: len(results)}

	for _, result := range results {
		report.Tests += len(result.Tests)
		for _, test := range result.Tests {
			if test.Failed {
				report.FailedTests++
			}
		}

		switch {
		case result.Err != nil:
			report.Errors++
		case strict && (result.StatusCode < 200 || result.StatusCode > 299):
			report.Errors++
		}
	}

	return report
}

// printReport writes per-test FAIL/ERROR lines (PASS too under verbose) and
// the run summary.
func printReport(w io.Writer, results []*Result, report Report, verbose bool) {
	for _, result := range results {
		for _, test := range result.Tests {
			switch {
			case test.Failed:
				_, _ = fmt.Fprintf(w, "FAIL %s / %s: %s\n", result.Name, test.Name, test.Message)
			case verbose:
				_, _ = fmt.Fprintf(w, "PASS %s / %s\n", result.Name, test.Name)
			}
		}

		if result.Err != nil {
			_, _ = fmt.Fprintf(w, "ERROR %s: %s\n", result.Name, result.Err)
		}
	}

	_, _ = fmt.Fprintf(
		w,
		"\n%d requests, %d tests, %d failed, %d errors\n",
		report.Requests, report.Tests, report.FailedTests, report.Errors,
	)
}
