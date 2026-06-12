package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const vegetaContent = "# @vegeta rate=20/s duration=300ms\n" +
	"GET https://localhost:8080/anything\n"

func TestVegetaE2E(t *testing.T) {
	srv := newTestServer(t)

	t.Run("flag on attacks marked request", func(t *testing.T) {
		results, out := runResultsVegeta(t, srv, vegetaContent, "")
		require.Len(t, results, 1)

		result := results[0]
		assert.True(t, result.Vegeta)
		assert.NoError(t, result.Err)
		assert.Contains(t, out, "Requests")
		assert.Contains(t, out, "Success")
	})

	t.Run("flag off runs single request", func(t *testing.T) {
		results, out := runResults(t, srv, vegetaContent, "")
		require.Len(t, results, 1)

		result := results[0]
		assert.False(t, result.Vegeta)
		assert.NoError(t, result.Err)
		assert.Contains(t, out, "200 OK")
		assert.NotContains(t, out, "Latencies")
	})

	t.Run("unmarked request runs normally under flag", func(t *testing.T) {
		content := "GET https://localhost:8080/anything\n"
		results, out := runResultsVegeta(t, srv, content, "")
		require.Len(t, results, 1)

		assert.False(t, results[0].Vegeta)
		assert.Contains(t, out, "200 OK")
	})

	t.Run("failing shots set error", func(t *testing.T) {
		// /bearer without an Authorization header answers 401 on every shot.
		content := "# @vegeta rate=20/s duration=300ms\n" +
			"GET https://localhost:8080/bearer\n"
		results, _ := runResultsVegeta(t, srv, content, "")
		require.Len(t, results, 1)

		result := results[0]
		assert.True(t, result.Vegeta)
		require.Error(t, result.Err)
		assert.Contains(t, result.Err.Error(), "success")

		report := buildReport(results, false)
		assert.Equal(t, 1, report.Errors)
		assert.True(t, report.Failed())
	})

	t.Run("grpc request rejected", func(t *testing.T) {
		content := "# @vegeta\n" +
			"GRPC localhost:9999/test.Echo/Echo\n"
		results, _ := runResultsVegeta(t, srv, content, "")
		require.Len(t, results, 1)
		require.Error(t, results[0].Err)
		assert.Contains(t, results[0].Err.Error(), "GRPC")
	})

	t.Run("post script skipped for attacked request", func(t *testing.T) {
		content := "# @vegeta rate=20/s duration=300ms\n" +
			"GET https://localhost:8080/anything\n\n" +
			"> {% client.test(\"never runs\", function() {}); %}\n"
		results, _ := runResultsVegeta(t, srv, content, "")
		require.Len(t, results, 1)
		assert.Empty(t, results[0].Tests)
	})

	t.Run("no-log prints summary only", func(t *testing.T) {
		content := "# @vegeta rate=20/s duration=300ms\n" +
			"# @no-log\n" +
			"GET https://localhost:8080/anything\n"
		results, out := runResultsVegeta(t, srv, content, "")
		require.Len(t, results, 1)
		assert.NoError(t, results[0].Err)
		assert.Contains(t, out, "success")
		assert.NotContains(t, out, "Latencies")
	})

	t.Run("strict ignores vegeta status codes", func(t *testing.T) {
		results, _ := runResultsVegeta(t, srv, vegetaContent, "")
		report := buildReport(results, true)
		assert.Zero(t, report.Errors, "vegeta result must not trip strict status check")
	})
}

func TestVegetaFixture(t *testing.T) {
	srv := newTestServer(t)

	raw, err := os.ReadFile("testdata/vegeta.http")
	require.NoError(t, err)

	results, out := runResultsVegeta(t, srv, string(raw), "testdata")
	require.NotEmpty(t, results)
	for _, result := range results {
		assert.NoError(t, result.Err)
	}
	assert.Contains(t, out, "Success")
}
