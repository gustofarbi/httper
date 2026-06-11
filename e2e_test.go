package main

import (
	"bytes"
	"httper/internal/echo/handler"
	"httper/pkg/request"
	"httper/pkg/script"
	"httper/pkg/vars"
	"io"
	"net/http/cookiejar"
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
	_, out := runResults(t, srv, content, wd)
	return out
}

// runResults is runContent but also returns the per-request results for
// report assertions.
func runResults(t *testing.T, srv *httptest.Server, content, wd string) ([]*Result, string) {
	t.Helper()

	content = strings.ReplaceAll(content, fixtureHost, srv.URL)

	httpFile, err := request.ParseFile(content)
	require.NoError(t, err)

	globals := vars.NewGlobals()
	store := vars.NewStore(nil, httpFile.Vars, globals)

	// Same default as main.run: a cookie jar so chained requests share cookies.
	client := srv.Client()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client.Jar = jar

	buf := new(bytes.Buffer)
	runner := &Runner{
		Client: client,
		Out:    buf,
		Config: Config{},
	}
	engine := &script.Engine{Globals: globals, Out: buf}

	// Mirrors main.run's loader: handler script paths resolve inside wd.
	var loadScript func(path string) (string, error)
	if wd != "" {
		loadScript = func(path string) (string, error) {
			root, err := os.OpenRoot(wd)
			if err != nil {
				return "", err
			}
			defer func() { _ = root.Close() }()

			code, err := root.Open(path)
			if err != nil {
				return "", err
			}
			defer func() { _ = code.Close() }()

			raw, err := io.ReadAll(code)
			return string(raw), err
		}
	}

	results := executeTemplates(runner, httpFile.Templates, store, engine, wd, loadScript)
	return results, buf.String()
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

	t.Run("in-file vars and dynamic uuid", func(t *testing.T) {
		content := "@base = " + fixtureHost + "\n" +
			"GET {{base}}/?id={{$uuid}}"
		out := runContent(t, srv, content, "")
		assert.Contains(t, out, "200 OK")
		assert.Regexp(t, `id=[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`, out)
	})

	t.Run("redirect followed by default", func(t *testing.T) {
		out := runContent(t, srv, "GET "+fixtureHost+"/redirect", "")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "Redirected OK")
	})

	t.Run("no-redirect directive stops at 302", func(t *testing.T) {
		out := runContent(t, srv, "# @no-redirect\nGET "+fixtureHost+"/redirect", "")
		assert.Contains(t, out, "302")
		assert.NotContains(t, out, "Redirected OK")
	})

	t.Run("cookie jar carries cookies across requests", func(t *testing.T) {
		content := "GET " + fixtureHost + "/set-cookie\n" +
			"###\n" +
			"GET " + fixtureHost + "/need-cookie"
		out := runContent(t, srv, content, "")
		assert.Contains(t, out, "Cookie OK")
	})

	t.Run("no-cookie-jar directive drops cookies", func(t *testing.T) {
		content := "GET " + fixtureHost + "/set-cookie\n" +
			"###\n" +
			"# @no-cookie-jar\n" +
			"GET " + fixtureHost + "/need-cookie"
		out := runContent(t, srv, content, "")
		assert.Contains(t, out, "401")
		assert.NotContains(t, out, "Cookie OK")
	})

	t.Run("no-log directive prints status only", func(t *testing.T) {
		out := runContent(t, srv, "# @no-log\nGET "+fixtureHost+"/redirect", "")
		assert.Contains(t, out, "Status 200")
		assert.NotContains(t, out, "Redirected OK")
	})

	t.Run("response handler chains token to next request", func(t *testing.T) {
		content := "POST " + fixtureHost + "/token\n\n" +
			"> {%\n" +
			"    client.test(\"token issued\", function() {\n" +
			"        client.assert(response.status === 200, \"expected 200\");\n" +
			"    });\n" +
			"    client.global.set(\"token\", response.body.token);\n" +
			"%}\n" +
			"###\n" +
			"GET " + fixtureHost + "/bearer\n" +
			"Authorization: Bearer {{token}}"

		out := runContent(t, srv, content, "")
		assert.Contains(t, out, "Authorized")
	})

	t.Run("pre-request script sets request variables", func(t *testing.T) {
		content := "< {% request.variables.set(\"p\", \"param1=foobar\") %}\n" +
			"GET " + fixtureHost + "/?query&{{p}}"

		out := runContent(t, srv, content, "")
		assert.Contains(t, out, "param1")
	})

	t.Run("handler script loaded from file", func(t *testing.T) {
		content := "GET " + fixtureHost + "/redirect\n\n" +
			"> check.js"

		out := runContent(t, srv, content, "testdata")
		assert.Contains(t, out, "file handler ran")
	})

	t.Run("form-urlencoded body", func(t *testing.T) {
		content := "POST " + fixtureHost + "/urlencoded\n" +
			"Content-Type: application/x-www-form-urlencoded\n\n" +
			"a=1&\n" +
			"b=two"

		out := runContent(t, srv, content, "")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "a=[1]")
		assert.Contains(t, out, "b=[two]")
	})

	t.Run("raw body sent verbatim for unknown content type", func(t *testing.T) {
		content := "POST " + fixtureHost + "/raw\n" +
			"Content-Type: text/plain\n\n" +
			"hello raw"

		out := runContent(t, srv, content, "")
		assert.Contains(t, out, "200 OK")
		assert.Contains(t, out, "text/plain: hello raw")
	})

	t.Run("http2 prior knowledge", func(t *testing.T) {
		// Real h2c prior-knowledge can't run against httptest's TLS server, and
		// Runner.Send injects a bare http2.Transport with no TLS config for the
		// HTTP/2 proto, so it cannot trust the test cert. Proto parsing itself
		// is covered by request.TestCreate ("explicit HTTP/2 proto").
		t.Skip("HTTP/2 prior knowledge not supported by in-process httptest server")
	})
}

func TestE2EReport(t *testing.T) {
	srv := newTestServer(t)

	t.Run("failing client.test reaches the report", func(t *testing.T) {
		content := "GET " + fixtureHost + "/redirect\n\n" +
			"> {% client.test(\"must fail\", function() { client.assert(false, \"nope\"); }); %}"

		results, _ := runResults(t, srv, content, "")
		report := buildReport(results, false)

		assert.Equal(t, 1, report.Requests)
		assert.Equal(t, 1, report.Tests)
		assert.Equal(t, 1, report.FailedTests)
		assert.True(t, report.Failed())
	})

	t.Run("strict flags non-2xx", func(t *testing.T) {
		results, _ := runResults(t, srv, "GET "+fixtureHost+"/need-cookie", "")

		assert.False(t, buildReport(results, false).Failed())
		assert.True(t, buildReport(results, true).Failed())
	})
}

func TestE2ESavesResponse(t *testing.T) {
	srv := newTestServer(t)

	tmp := t.TempDir()
	root, err := os.OpenRoot(tmp)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	content := strings.ReplaceAll("GET "+fixtureHost+"/bearer\nAuthorization: Bearer 42069", fixtureHost, srv.URL)
	httpFile, err := request.ParseFile(content)
	require.NoError(t, err)
	require.Len(t, httpFile.Templates, 1)

	req, err := httpFile.Templates[0].Build(func(s string) string { return s }, "")
	require.NoError(t, err)

	runner := &Runner{
		Client:   srv.Client(),
		Out:      new(bytes.Buffer),
		Config:   Config{Save: true},
		SaveRoot: root,
	}
	runner.Send(httpFile.Templates[0], req)

	entries, err := os.ReadDir(tmp + "/.idea/httpRequests")
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "expected a saved response file")
}
