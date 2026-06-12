package script

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gustofarbi/httper/pkg/vars"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEngine(out *bytes.Buffer) (*Engine, *vars.Globals) {
	globals := vars.NewGlobals()
	return &Engine{Globals: globals, Out: out}, globals
}

func jsonResponse(body string) *Response {
	return &Response{
		Status:      200,
		Headers:     http.Header{"Content-Type": []string{"application/json"}},
		ContentType: "application/json",
		Body:        []byte(body),
	}
}

func TestRunPost(t *testing.T) {
	t.Run("passing and failing tests captured", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))

		results, err := engine.RunPost(
			`client.test("passes", function() {});
			 client.test("fails", function() { throw new Error("boom"); });`,
			jsonResponse(`{}`),
		)
		require.NoError(t, err)
		require.Len(t, results, 2)

		assert.Equal(t, "passes", results[0].Name)
		assert.False(t, results[0].Failed)
		assert.Equal(t, "fails", results[1].Name)
		assert.True(t, results[1].Failed)
		assert.Contains(t, results[1].Message, "boom")
	})

	t.Run("client.assert throws on falsy", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))

		results, err := engine.RunPost(
			`client.test("status", function() {
				client.assert(response.status === 404, "expected 404");
			});`,
			jsonResponse(`{}`),
		)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.True(t, results[0].Failed)
		assert.Contains(t, results[0].Message, "expected 404")
	})

	t.Run("response.body parsed as json", func(t *testing.T) {
		engine, globals := newEngine(new(bytes.Buffer))

		_, err := engine.RunPost(
			`client.global.set("token", response.body.token);`,
			jsonResponse(`{"token":"abc123"}`),
		)
		require.NoError(t, err)

		token, ok := globals.Get("token")
		assert.True(t, ok)
		assert.Equal(t, "abc123", token)
	})

	t.Run("non-json body is a string", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))

		results, err := engine.RunPost(
			`client.test("raw", function() {
				client.assert(response.body === "plain", "body was: " + response.body);
			});`,
			&Response{Status: 200, ContentType: "text/plain", Body: []byte("plain"), Headers: http.Header{}},
		)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.False(t, results[0].Failed)
	})

	t.Run("headers valueOf is case-insensitive", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))

		resp := jsonResponse(`{}`)
		resp.Headers.Set("X-Request-Id", "rid-1")

		results, err := engine.RunPost(
			`client.test("header", function() {
				client.assert(response.headers.valueOf("x-request-id") === "rid-1", "missing header");
			});`,
			resp,
		)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.False(t, results[0].Failed, results[0].Message)
	})

	t.Run("contentType exposes mimeType", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))

		resp := jsonResponse(`{}`)
		resp.ContentType = "application/json; charset=utf-8"

		results, err := engine.RunPost(
			`client.test("ct", function() {
				client.assert(response.contentType.mimeType === "application/json", response.contentType.mimeType);
				client.assert(response.contentType.charset === "utf-8", response.contentType.charset);
			});`,
			resp,
		)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.False(t, results[0].Failed, results[0].Message)
	})

	t.Run("client.log writes to out", func(t *testing.T) {
		out := new(bytes.Buffer)
		engine, _ := newEngine(out)

		_, err := engine.RunPost(`client.log("hello", 42);`, jsonResponse(`{}`))
		require.NoError(t, err)
		assert.Contains(t, out.String(), "hello 42")
	})

	t.Run("syntax error reported", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))
		_, err := engine.RunPost(`this is not js`, jsonResponse(`{}`))
		assert.Error(t, err)
	})

	t.Run("runaway script interrupted", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))
		engine.Timeout = 50 * time.Millisecond

		_, err := engine.RunPost(`while (true) {}`, jsonResponse(`{}`))
		assert.Error(t, err)
	})
}

func TestRunPre(t *testing.T) {
	t.Run("request.variables.set reaches callback", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))

		set := map[string]string{}
		err := engine.RunPre(
			`request.variables.set("id", "42");
			 request.variables.set("n", 7);`,
			nil,
			func(k, v string) { set[k] = v },
		)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"id": "42", "n": "7"}, set)
	})

	t.Run("client.global available in pre-script", func(t *testing.T) {
		engine, globals := newEngine(new(bytes.Buffer))
		globals.Set("seed", "s-1")

		set := map[string]string{}
		err := engine.RunPre(
			`request.variables.set("from-global", client.global.get("seed"));`,
			nil,
			func(k, v string) { set[k] = v },
		)
		require.NoError(t, err)
		assert.Equal(t, "s-1", set["from-global"])
	})
}

func preRequest() *PreRequest {
	return &PreRequest{
		Method: "POST",
		URL:    "https://{{host}}/api",
		Body:   `{"id": "{{id}}"}`,
		Headers: [][2]string{
			{"Content-Type", "application/json"},
			{"X-Token", "{{token}}"},
		},
		Environment: map[string]string{"host": "env.example"},
		Resolve: func(s string) string {
			return strings.NewReplacer("{{host}}", "env.example", "{{token}}", "tok-1").Replace(s)
		},
	}
}

func TestRunPreRequestObject(t *testing.T) {
	run := func(t *testing.T, code string) map[string]string {
		t.Helper()
		engine, _ := newEngine(new(bytes.Buffer))
		set := map[string]string{}
		err := engine.RunPre(code, preRequest(), func(k, v string) { set[k] = v })
		require.NoError(t, err)
		return set
	}

	t.Run("method and raw url with placeholders intact", func(t *testing.T) {
		set := run(t,
			`request.variables.set("m", request.method());
			 request.variables.set("u", request.url().getRaw());`)
		assert.Equal(t, "POST", set["m"])
		assert.Equal(t, "https://{{host}}/api", set["u"])
	})

	t.Run("url tryGetSubstituted resolves", func(t *testing.T) {
		set := run(t, `request.variables.set("u", request.url().tryGetSubstituted());`)
		assert.Equal(t, "https://env.example/api", set["u"])
	})

	t.Run("raw body", func(t *testing.T) {
		set := run(t, `request.variables.set("b", request.body().getRaw());`)
		assert.Equal(t, `{"id": "{{id}}"}`, set["b"])
	})

	t.Run("headers all and findByName", func(t *testing.T) {
		set := run(t,
			`request.variables.set("count", request.headers.all().length);
			 request.variables.set("first", request.headers.all()[0].name());
			 request.variables.set("raw", request.headers.findByName("x-token").getRawValue());
			 request.variables.set("sub", request.headers.findByName("X-Token").tryGetSubstituted());`)
		assert.Equal(t, "2", set["count"])
		assert.Equal(t, "Content-Type", set["first"])
		assert.Equal(t, "{{token}}", set["raw"])
		assert.Equal(t, "tok-1", set["sub"])
	})

	t.Run("findByName misses with null", func(t *testing.T) {
		set := run(t, `request.variables.set("miss", request.headers.findByName("nope") === null);`)
		assert.Equal(t, "true", set["miss"])
	})

	t.Run("environment get", func(t *testing.T) {
		set := run(t,
			`request.variables.set("host", request.environment.get("host"));
			 request.variables.set("miss", request.environment.get("nope") === null);`)
		assert.Equal(t, "env.example", set["host"])
		assert.Equal(t, "true", set["miss"])
	})

	t.Run("nil request still runs", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))
		set := map[string]string{}
		err := engine.RunPre(`request.variables.set("x", "1");`, nil, func(k, v string) { set[k] = v })
		require.NoError(t, err)
		assert.Equal(t, "1", set["x"])
	})
}

func TestCrypto(t *testing.T) {
	t.Run("digests in pre-request scripts", func(t *testing.T) {
		engine, _ := newEngine(new(bytes.Buffer))
		set := map[string]string{}
		err := engine.RunPre(
			`request.variables.set("sha256", crypto.sha256("abc"));
			 request.variables.set("sha1", crypto.sha1("abc"));
			 request.variables.set("md5", crypto.md5("abc"));
			 request.variables.set("hmac", crypto.hmac.sha256("key", "The quick brown fox jumps over the lazy dog"));`,
			nil,
			func(k, v string) { set[k] = v },
		)
		require.NoError(t, err)
		assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", set["sha256"])
		assert.Equal(t, "a9993e364706816aba3e25717850c26c9cd0d89d", set["sha1"])
		assert.Equal(t, "900150983cd24fb0d6963f7d28e17f72", set["md5"])
		assert.Equal(t, "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8", set["hmac"])
	})

	t.Run("available in response handlers", func(t *testing.T) {
		engine, globals := newEngine(new(bytes.Buffer))
		_, err := engine.RunPost(`client.global.set("sig", crypto.sha256("abc"));`, jsonResponse(`{}`))
		require.NoError(t, err)
		sig, _ := globals.Get("sig")
		assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", sig)
	})
}
