package main

import (
	"bytes"
	"errors"
	"testing"

	"httper/pkg/request"
	"httper/pkg/script"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionString(t *testing.T) {
	assert.Equal(t, "httper dev", versionString())
}

func sampleResults() []*Result {
	return []*Result{
		{
			Name:       "a",
			StatusCode: 200,
			Tests: []script.TestResult{
				{Name: "t1"},
				{Name: "t2", Failed: true, Message: "boom"},
			},
		},
		{Name: "b", StatusCode: 404},
		{Name: "c", Err: errors.New("connection refused")},
	}
}

func TestBuildReport(t *testing.T) {
	t.Run("counts requests, tests, failures, errors", func(t *testing.T) {
		report := buildReport(sampleResults(), false)

		assert.Equal(t, 3, report.Requests)
		assert.Equal(t, 2, report.Tests)
		assert.Equal(t, 1, report.FailedTests)
		assert.Equal(t, 1, report.Errors)
		assert.True(t, report.Failed())
	})

	t.Run("clean run does not fail", func(t *testing.T) {
		report := buildReport([]*Result{{Name: "a", StatusCode: 200}}, false)
		assert.False(t, report.Failed())
	})

	t.Run("strict counts non-2xx as error", func(t *testing.T) {
		report := buildReport(sampleResults(), true)
		// the 404 joins; the errored request must not be double-counted
		assert.Equal(t, 2, report.Errors)
	})

	t.Run("non-2xx without strict is fine", func(t *testing.T) {
		report := buildReport([]*Result{{Name: "b", StatusCode: 404}}, false)
		assert.False(t, report.Failed())
	})
}

func TestPrintReport(t *testing.T) {
	t.Run("failures and summary always shown", func(t *testing.T) {
		buf := new(bytes.Buffer)
		printReport(buf, sampleResults(), buildReport(sampleResults(), false), false)

		out := buf.String()
		assert.Contains(t, out, "FAIL a / t2: boom")
		assert.Contains(t, out, "ERROR c: connection refused")
		assert.NotContains(t, out, "PASS")
		assert.Contains(t, out, "3 requests, 2 tests, 1 failed, 1 error")
	})

	t.Run("passes shown under verbose", func(t *testing.T) {
		buf := new(bytes.Buffer)
		printReport(buf, sampleResults(), buildReport(sampleResults(), false), true)
		assert.Contains(t, buf.String(), "PASS a / t1")
	})
}

func TestFilterTemplates(t *testing.T) {
	templates := []*request.Template{
		{Name: "Login"},
		{Name: "Fetch"},
		{Name: "#3"},
	}

	t.Run("empty filter keeps all", func(t *testing.T) {
		got, err := filterTemplates(templates, "")
		require.NoError(t, err)
		assert.Len(t, got, 3)
	})

	t.Run("single name", func(t *testing.T) {
		got, err := filterTemplates(templates, "Fetch")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "Fetch", got[0].Name)
	})

	t.Run("comma-separated names keep file order", func(t *testing.T) {
		got, err := filterTemplates(templates, "Fetch,Login")
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "Login", got[0].Name)
		assert.Equal(t, "Fetch", got[1].Name)
	})

	t.Run("no match errors", func(t *testing.T) {
		_, err := filterTemplates(templates, "nope")
		assert.Error(t, err)
	})
}
