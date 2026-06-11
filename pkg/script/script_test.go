package script

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"httper/pkg/vars"

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
			func(k, v string) { set[k] = v },
		)
		require.NoError(t, err)
		assert.Equal(t, "s-1", set["from-global"])
	})
}
