package main

import (
	"bytes"
	"httper/internal/echo/handler"
	"httper/pkg/request"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureHost is the literal every testdata/*.http fixture targets; e2e tests
// rewrite it to the in-process test server's URL.
const fixtureHost = "https://localhost:8080"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler.NewMux())
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// runContent rewrites the fixture host, parses the requests, and drives them
// through the real Runner against srv, returning the combined output.
func runContent(t *testing.T, srv *httptest.Server, content, wd string) string {
	t.Helper()

	content = strings.ReplaceAll(content, fixtureHost, srv.URL)

	reqs, err := request.Create(content, wd)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	runner := &Runner{
		Client: srv.Client(),
		Out:    buf,
		Config: Config{},
	}
	for _, req := range reqs {
		runner.Send(req)
	}
	return buf.String()
}

func runFixture(t *testing.T, srv *httptest.Server, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	// wd is testdata so any base-name include resolves there.
	return runContent(t, srv, string(raw), "testdata")
}

func TestE2EFixtures(t *testing.T) {
	srv := newTestServer(t)

	t.Run("get echoes query params", func(t *testing.T) {
		out := runFixture(t, srv, "get.http")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "param1")
	})

	t.Run("bearer authorized", func(t *testing.T) {
		out := runFixture(t, srv, "bearer.http")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "Authorized")
	})

	t.Run("basic auth authorized", func(t *testing.T) {
		out := runFixture(t, srv, "basic-auth.http")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "Authorized")
	})

	t.Run("json accepted", func(t *testing.T) {
		out := runFixture(t, srv, "json.http")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "Content-length:")
	})

	t.Run("image returns jpeg", func(t *testing.T) {
		out := runFixture(t, srv, "image.http")
		assert.Contains(t, out, "200 OK")
	})

	t.Run("form-data with file include", func(t *testing.T) {
		// Inline fixture: the committed formdata.http uses "< ../makefile"
		// which os.Root rejects; e2e uses a base-name include within testdata.
		content := "POST " + fixtureHost + "/form-data?headers\n" +
			"Content-Type: multipart/form-data; boundary=foo\n\n" +
			"--foo\n" +
			"Content-Disposition: form-data; name=\"file\"; filename=\"include.txt\"\n" +
			"Content-Type: text/plain\n\n" +
			"< include.txt\n" +
			"--foo\n" +
			"Content-Disposition: form-data; name=\"title\"\n" +
			"Content-Type: text/plain\n\n" +
			"hello\n" +
			"--foo--"
		out := runContent(t, srv, content, "testdata")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "Part: file, 'include.txt'")
		assert.Contains(t, out, "Part: title")
	})

	t.Run("http2 prior knowledge", func(t *testing.T) {
		// Real h2c prior-knowledge can't run against httptest's TLS server, and
		// Runner.Send injects a bare http2.Transport with no TLS config for the
		// HTTP/2 proto, so it cannot trust the test cert. Proto parsing itself
		// is covered by request.TestCreate ("explicit HTTP/2 proto").
		t.Skip("HTTP/2 prior knowledge not supported by in-process httptest server")
	})
}

func TestE2ESavesResponse(t *testing.T) {
	srv := newTestServer(t)

	tmp := t.TempDir()
	root, err := os.OpenRoot(tmp)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	content := strings.ReplaceAll("GET "+fixtureHost+"/bearer\nAuthorization: Bearer 42069", fixtureHost, srv.URL)
	reqs, err := request.Create(content, "")
	require.NoError(t, err)

	runner := &Runner{
		Client:   srv.Client(),
		Out:      new(bytes.Buffer),
		Config:   Config{Save: true},
		SaveRoot: root,
	}
	runner.Send(reqs[0])

	entries, err := os.ReadDir(tmp + "/.idea/httpRequests")
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "expected a saved response file")
}
