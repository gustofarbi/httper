package finalize

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintSummary(t *testing.T) {
	t.Run("grpc-style status line with json body", func(t *testing.T) {
		buf := new(bytes.Buffer)
		summary := Summary{
			StatusLine:    "0 OK",
			StatusCode:    0,
			Duration:      time.Second,
			ContentLength: 13,
			Header:        http.Header{"X-Echo-Header": []string{"header-value"}},
			ContentType:   "application/json",
		}

		require.NoError(t, Print(buf, summary, []byte(`{"a": 1}`), Options{}, nil))
		out := buf.String()

		assert.Contains(t, out, "Status")
		assert.Contains(t, out, "0 OK")
		assert.Contains(t, out, "Duration")
		assert.Contains(t, out, "Response body:")
		assert.Contains(t, out, `"a": 1`)
		assert.NotContains(t, out, "X-Echo-Header")
	})

	t.Run("verbose prints headers", func(t *testing.T) {
		buf := new(bytes.Buffer)
		summary := Summary{
			StatusLine: "14 Unavailable",
			StatusCode: 14,
			Header:     http.Header{"X-Echo-Header": []string{"header-value"}},
		}

		require.NoError(t, Print(buf, summary, nil, Options{Verbose: true}, nil))
		out := buf.String()

		assert.Contains(t, out, "14 Unavailable")
		assert.Contains(t, out, "X-Echo-Header")
	})

	t.Run("quiet prints only status line", func(t *testing.T) {
		buf := new(bytes.Buffer)
		summary := Summary{StatusLine: "0 OK", Duration: time.Second}

		require.NoError(t, Print(buf, summary, []byte("body text"), Options{Quiet: true}, nil))
		out := buf.String()

		assert.Contains(t, out, "Status 0 OK")
		assert.NotContains(t, out, "body text")
		assert.NotContains(t, out, "Duration")
	})
}
