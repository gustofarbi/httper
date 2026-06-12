package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gustofarbi/httper/pkg/script"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reportSuites() []Suite {
	return []Suite{
		{
			Name: "api.http",
			Results: []*Result{
				{
					Name:       "login",
					StatusCode: 200,
					Duration:   1500 * time.Millisecond,
					Tests: []script.TestResult{
						{Name: "ok"},
						{Name: "token", Failed: true, Message: "boom"},
					},
				},
				{Name: "ping", StatusCode: 204, Duration: 20 * time.Millisecond},
				{Name: "broken", Err: errors.New("connection refused")},
			},
		},
	}
}

func TestWriteJUnit(t *testing.T) {
	buf := new(bytes.Buffer)
	require.NoError(t, writeJUnit(buf, reportSuites()))

	out := buf.String()
	assert.Equal(t, `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="4" failures="1" errors="1">
  <testsuite name="api.http" tests="4" failures="1" errors="1">
    <testcase name="login / ok" classname="api.http" time="1.500"></testcase>
    <testcase name="login / token" classname="api.http" time="1.500">
      <failure message="boom"></failure>
    </testcase>
    <testcase name="ping" classname="api.http" time="0.020"></testcase>
    <testcase name="broken" classname="api.http" time="0.000">
      <error message="connection refused"></error>
    </testcase>
  </testsuite>
</testsuites>
`, out)

	// must be parseable XML
	var parsed struct {
		XMLName xml.Name `xml:"testsuites"`
		Tests   int      `xml:"tests,attr"`
	}
	require.NoError(t, xml.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, 4, parsed.Tests)
}

func TestWriteJSONReport(t *testing.T) {
	buf := new(bytes.Buffer)
	require.NoError(t, writeJSONReport(buf, reportSuites(), buildReport(reportSuites()[0].Results, false)))

	var parsed struct {
		Files []struct {
			File     string `json:"file"`
			Requests []struct {
				Name       string `json:"name"`
				Status     int    `json:"status"`
				DurationMs int64  `json:"durationMs"`
				Error      string `json:"error"`
				Tests      []struct {
					Name    string `json:"name"`
					Failed  bool   `json:"failed"`
					Message string `json:"message"`
				} `json:"tests"`
			} `json:"requests"`
		} `json:"files"`
		Summary struct {
			Requests int `json:"requests"`
			Tests    int `json:"tests"`
			Failed   int `json:"failed"`
			Errors   int `json:"errors"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	require.Len(t, parsed.Files, 1)
	assert.Equal(t, "api.http", parsed.Files[0].File)
	require.Len(t, parsed.Files[0].Requests, 3)
	assert.Equal(t, "login", parsed.Files[0].Requests[0].Name)
	assert.Equal(t, int64(1500), parsed.Files[0].Requests[0].DurationMs)
	assert.Equal(t, "connection refused", parsed.Files[0].Requests[2].Error)
	assert.Equal(t, 3, parsed.Summary.Requests)
	assert.Equal(t, 2, parsed.Summary.Tests)
	assert.Equal(t, 1, parsed.Summary.Failed)
	assert.Equal(t, 1, parsed.Summary.Errors)
}

func TestWriteReportFiles(t *testing.T) {
	suites := reportSuites()
	report := buildReport(suites[0].Results, false)

	t.Run("writes both formats", func(t *testing.T) {
		dir := t.TempDir()
		junitPath := dir + "/junit.xml"
		jsonPath := dir + "/report.json"

		require.NoError(t, writeReportFiles(suites, report, junitPath, jsonPath))

		junitRaw, err := os.ReadFile(junitPath)
		require.NoError(t, err)
		assert.Contains(t, string(junitRaw), `<testsuite name="api.http"`)

		jsonRaw, err := os.ReadFile(jsonPath)
		require.NoError(t, err)
		assert.Contains(t, string(jsonRaw), `"file": "api.http"`)
	})

	t.Run("empty paths write nothing", func(t *testing.T) {
		require.NoError(t, writeReportFiles(suites, report, "", ""))
	})

	t.Run("unwritable destination errors", func(t *testing.T) {
		assert.Error(t, writeReportFiles(suites, report, "/definitely-missing-dir/x.xml", ""))
	})
}
