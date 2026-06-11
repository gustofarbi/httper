package finalize

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newResponse(status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode:    status,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

func TestResponse(t *testing.T) {
	t.Run("status, duration and content-length", func(t *testing.T) {
		buf := new(bytes.Buffer)
		resp := newResponse(http.StatusOK, nil, "hi")

		err := Response(buf, resp, time.Second, false, false, nil)
		require.NoError(t, err)

		out := buf.String()
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

		quiet := new(bytes.Buffer)
		require.NoError(t, Response(quiet, newResponse(http.StatusOK, header, ""), 0, false, false, nil))
		assert.NotContains(t, quiet.String(), "X-Custom")

		verbose := new(bytes.Buffer)
		require.NoError(t, Response(verbose, newResponse(http.StatusOK, header, ""), 0, false, true, nil))
		assert.Contains(t, verbose.String(), "X-Custom")
		assert.Contains(t, verbose.String(), "abc")
	})

	t.Run("json body pretty printed", func(t *testing.T) {
		buf := new(bytes.Buffer)
		header := http.Header{"Content-Type": []string{"application/json"}}
		resp := newResponse(http.StatusOK, header, `{"a":1}`)

		require.NoError(t, Response(buf, resp, 0, false, false, nil))
		// MarshalIndent uses two-space indentation.
		assert.Contains(t, buf.String(), "\"a\": 1")
	})

	t.Run("non-json body printed raw", func(t *testing.T) {
		buf := new(bytes.Buffer)
		header := http.Header{"Content-Type": []string{"text/plain"}}
		resp := newResponse(http.StatusOK, header, "plain text")

		require.NoError(t, Response(buf, resp, 0, false, false, nil))
		assert.Contains(t, buf.String(), "plain text")
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
