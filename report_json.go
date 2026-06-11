package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Explicit DTOs: Result holds an error and http.Header, neither of which
// marshals usefully, and the body has no place in a test report.

type jsonReport struct {
	Files   []jsonFile  `json:"files"`
	Summary jsonSummary `json:"summary"`
}

type jsonFile struct {
	File     string        `json:"file"`
	Requests []jsonRequest `json:"requests"`
}

type jsonRequest struct {
	Name       string     `json:"name"`
	Status     int        `json:"status"`
	DurationMs int64      `json:"durationMs"`
	Error      string     `json:"error,omitempty"`
	Tests      []jsonTest `json:"tests,omitempty"`
}

type jsonTest struct {
	Name    string `json:"name"`
	Failed  bool   `json:"failed"`
	Message string `json:"message,omitempty"`
}

type jsonSummary struct {
	Requests int `json:"requests"`
	Tests    int `json:"tests"`
	Failed   int `json:"failed"`
	Errors   int `json:"errors"`
}

func writeJSONReport(w io.Writer, suites []Suite, report Report) error {
	doc := jsonReport{
		Files: make([]jsonFile, 0, len(suites)),
		Summary: jsonSummary{
			Requests: report.Requests,
			Tests:    report.Tests,
			Failed:   report.FailedTests,
			Errors:   report.Errors,
		},
	}

	for _, suite := range suites {
		file := jsonFile{File: suite.Name, Requests: make([]jsonRequest, 0, len(suite.Results))}

		for _, result := range suite.Results {
			r := jsonRequest{
				Name:       result.Name,
				Status:     result.StatusCode,
				DurationMs: result.Duration.Milliseconds(),
			}
			if result.Err != nil {
				r.Error = result.Err.Error()
			}
			for _, test := range result.Tests {
				r.Tests = append(r.Tests, jsonTest{Name: test.Name, Failed: test.Failed, Message: test.Message})
			}
			file.Requests = append(file.Requests, r)
		}

		doc.Files = append(doc.Files, file)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("encoding json report: %w", err)
	}

	return nil
}
