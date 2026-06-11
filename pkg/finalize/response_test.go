package finalize

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newResponse(status int, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
	}
}

func run(t *testing.T, resp *http.Response, body string, duration time.Duration, opts Options) string {
	t.Helper()
	resp.ContentLength = int64(len(body))
	buf := new(bytes.Buffer)
	require.NoError(t, Response(buf, resp, []byte(body), duration, opts, nil))
	return buf.String()
}

func TestResponse(t *testing.T) {
	t.Run("status, duration and content-length", func(t *testing.T) {
		out := run(t, newResponse(http.StatusOK, nil), "hi", time.Second, Options{})

		assert.Contains(t, out, "Status")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "Duration")
		assert.Contains(t, out, "1s")
		assert.Contains(t, out, "Content-Length")
		assert.Contains(t, out, "Response body:")
		assert.Contains(t, out, "hi")
	})

	t.Run("headers only under verbose", func(t *testing.T) {
		header := http.Header{"X-Custom": []string{"abc"}}

		quiet := run(t, newResponse(http.StatusOK, header), "", 0, Options{})
		assert.NotContains(t, quiet, "X-Custom")

		verbose := run(t, newResponse(http.StatusOK, header), "", 0, Options{Verbose: true})
		assert.Contains(t, verbose, "X-Custom")
		assert.Contains(t, verbose, "abc")
	})

	t.Run("quiet prints only the status line", func(t *testing.T) {
		out := run(t, newResponse(http.StatusTeapot, nil), "body text", time.Second, Options{Quiet: true})

		assert.Contains(t, out, "Status 418")
		assert.NotContains(t, out, "body text")
		assert.NotContains(t, out, "Duration")
	})

	t.Run("json body pretty printed", func(t *testing.T) {
		header := http.Header{"Content-Type": []string{"application/json"}}
		out := run(t, newResponse(http.StatusOK, header), `{"a":1}`, 0, Options{})
		// MarshalIndent uses two-space indentation.
		assert.Contains(t, out, "\"a\": 1")
	})

	t.Run("non-json body printed raw", func(t *testing.T) {
		header := http.Header{"Content-Type": []string{"text/plain"}}
		out := run(t, newResponse(http.StatusOK, header), "plain text", 0, Options{})
		assert.Contains(t, out, "plain text")
	})
}

func TestPrettyPrintJSON(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantContain string
	}{
		{name: "valid json indented", content: `{"a":1}`, wantContain: "\"a\": 1"},
		{name: "invalid json passthrough", content: "not json", wantContain: "not json"},
		{name: "empty produces nothing", content: "   ", wantContain: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			PrettyPrintJSON(buf, tt.content)
			if tt.wantContain == "" {
				assert.Empty(t, buf.String())
				return
			}
			assert.Contains(t, buf.String(), tt.wantContain)
		})
	}
}
