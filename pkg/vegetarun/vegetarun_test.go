package vegetarun

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"httper/pkg/request"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func directive(freq int, duration time.Duration) *request.VegetaDirective {
	return &request.VegetaDirective{
		Rate:     request.Rate{Freq: freq, Per: time.Second},
		Duration: duration,
		MaxBody:  -1,
	}
}

func TestRun(t *testing.T) {
	t.Run("attacks target and reports success", func(t *testing.T) {
		var hits atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			_, _ = w.Write([]byte("ok"))
		}))
		defer srv.Close()

		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		require.NoError(t, err)

		outcome, err := Run(req, directive(20, 250*time.Millisecond), Options{})
		require.NoError(t, err)

		assert.Positive(t, outcome.Metrics.Requests)
		assert.Equal(t, int64(outcome.Metrics.Requests), hits.Load())
		assert.InDelta(t, 1.0, outcome.Metrics.Success, 0.001)
		assert.Contains(t, outcome.Report, "Requests")
	})

	t.Run("non-2xx lowers success ratio", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		require.NoError(t, err)

		outcome, err := Run(req, directive(20, 250*time.Millisecond), Options{})
		require.NoError(t, err)

		assert.Positive(t, outcome.Metrics.Requests)
		assert.Zero(t, outcome.Metrics.Success)
		assert.Contains(t, outcome.Metrics.StatusCodes, "500")
	})

	t.Run("sends method, headers and body", func(t *testing.T) {
		var lastBody atomic.Value
		var lastHeader atomic.Value
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			lastBody.Store(string(body))
			lastHeader.Store(r.Header.Get("X-Custom"))
		}))
		defer srv.Close()

		req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"a":1}`))
		require.NoError(t, err)
		req.Header.Set("X-Custom", "yes")

		outcome, err := Run(req, directive(10, 200*time.Millisecond), Options{})
		require.NoError(t, err)

		assert.Positive(t, outcome.Metrics.Requests)
		assert.Equal(t, `{"a":1}`, lastBody.Load())
		assert.Equal(t, "yes", lastHeader.Load())
	})

	t.Run("tls config applied", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
		defer srv.Close()

		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		require.NoError(t, err)

		pool := x509.NewCertPool()
		pool.AddCert(srv.Certificate())

		outcome, err := Run(req, directive(10, 200*time.Millisecond), Options{
			TLSConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		})
		require.NoError(t, err)
		assert.InDelta(t, 1.0, outcome.Metrics.Success, 0.001)
	})
}

func TestOutcome(t *testing.T) {
	t.Run("full success not failed", func(t *testing.T) {
		o := &Outcome{}
		o.Metrics.Success = 1
		assert.False(t, o.Failed())
	})

	t.Run("partial success failed", func(t *testing.T) {
		o := &Outcome{}
		o.Metrics.Success = 0.99
		assert.True(t, o.Failed())
	})

	t.Run("summary digest", func(t *testing.T) {
		o := &Outcome{}
		o.Metrics.Success = 0.5
		o.Metrics.Requests = 10
		o.Metrics.StatusCodes = map[string]int{"500": 5, "200": 5}
		assert.Equal(t, "success 50.00% (10 requests) [200:5 500:5]", o.Summary())
	})
}
