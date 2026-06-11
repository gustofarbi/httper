package request

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func identity(s string) string { return s }

func TestParseFile(t *testing.T) {
	t.Run("splits on ### and keeps raw placeholder text", func(t *testing.T) {
		file, err := ParseFile(
			"GET https://{{host}}/a\n" +
				"###\n" +
				"POST https://{{host}}/b\n" +
				"Content-Type: application/json\n\n" +
				`{"x":"{{y}}"}`,
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 2)

		assert.Equal(t, "GET https://{{host}}/a", file.Templates[0].Essentials)
		assert.Empty(t, file.Templates[0].HeadersRaw)
		assert.Empty(t, file.Templates[0].BodyRaw)

		tpl := file.Templates[1]
		assert.Equal(t, "POST https://{{host}}/b", tpl.Essentials)
		assert.Equal(t, "Content-Type: application/json", tpl.HeadersRaw)
		assert.Equal(t, `{"x":"{{y}}"}`, tpl.BodyRaw)
	})

	t.Run("joins multiline url continuations", func(t *testing.T) {
		file, err := ParseFile(
			"GET https://localhost:8080/?\n" +
				"    foo=bar&\n" +
				"    baz=foo&\n" +
				"    query",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)
		assert.Equal(t, "GET https://localhost:8080/?foo=bar&baz=foo&query", file.Templates[0].Essentials)
	})

	t.Run("collects in-file @variables", func(t *testing.T) {
		file, err := ParseFile(
			"@host = https://localhost:8080\n" +
				"@token abc\n" +
				"GET {{host}}/a\n" +
				"###\n" +
				"@late = value\n" +
				"GET {{host}}/b",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 2)

		assert.Equal(t, map[string]string{
			"host":  "https://localhost:8080",
			"token": "abc",
			"late":  "value",
		}, file.Vars)
		assert.Equal(t, "GET {{host}}/a", file.Templates[0].Essentials)
		assert.Equal(t, "GET {{host}}/b", file.Templates[1].Essentials)
	})

	t.Run("file with only @variables yields no templates", func(t *testing.T) {
		file, err := ParseFile("@host = https://localhost:8080")
		require.NoError(t, err)
		assert.Empty(t, file.Templates)
		assert.Equal(t, "https://localhost:8080", file.Vars["host"])
	})

	t.Run("named requests via @name directive", func(t *testing.T) {
		file, err := ParseFile(
			"# @name Login\n" +
				"GET https://localhost:8080/a\n" +
				"###\n" +
				"// @name Fetch\n" +
				"GET https://localhost:8080/b",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 2)
		assert.Equal(t, "Login", file.Templates[0].Name)
		assert.Equal(t, "Fetch", file.Templates[1].Name)
	})

	t.Run("separator title names request", func(t *testing.T) {
		file, err := ParseFile(
			"GET https://localhost:8080/a\n" +
				"### Fetch the thing\n" +
				"GET https://localhost:8080/b",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 2)
		assert.Equal(t, "#1", file.Templates[0].Name)
		assert.Equal(t, "Fetch the thing", file.Templates[1].Name)
	})

	t.Run("directives parsed", func(t *testing.T) {
		file, err := ParseFile(
			"# @no-redirect\n" +
				"# @no-cookie-jar\n" +
				"# @no-log\n" +
				"# @timeout 5\n" +
				"GET https://localhost:8080/a",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		d := file.Templates[0].Directives
		assert.True(t, d.NoRedirect)
		assert.True(t, d.NoCookieJar)
		assert.True(t, d.NoLog)
		assert.Equal(t, 5*time.Second, d.Timeout)
	})

	t.Run("comments skipped outside body", func(t *testing.T) {
		file, err := ParseFile(
			"# leading comment\n" +
				"GET https://localhost:8080/a\n" +
				"// between headers\n" +
				"Accept: application/json\n" +
				"# another\n" +
				"X-Other: 1\n\n" +
				"body # stays intact\n" +
				"// also body",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		tpl := file.Templates[0]
		assert.Equal(t, "GET https://localhost:8080/a", tpl.Essentials)
		assert.Equal(t, "Accept: application/json\nX-Other: 1", tpl.HeadersRaw)
		assert.Equal(t, "body # stays intact\n// also body", tpl.BodyRaw)
	})

	t.Run("pre-request script extracted", func(t *testing.T) {
		file, err := ParseFile(
			"< {%\n" +
				"    request.variables.set(\"id\", \"42\")\n" +
				"%}\n" +
				"GET https://localhost:8080/a",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		tpl := file.Templates[0]
		assert.Contains(t, tpl.PreScript.Code, `request.variables.set("id", "42")`)
		assert.Equal(t, "GET https://localhost:8080/a", tpl.Essentials)
	})

	t.Run("response handler script extracted after body", func(t *testing.T) {
		file, err := ParseFile(
			"POST https://localhost:8080/json\n" +
				"Content-Type: application/json\n\n" +
				`{"a":1}` + "\n\n" +
				"> {%\n" +
				"    client.test(\"ok\", function() {})\n" +
				"%}",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		tpl := file.Templates[0]
		assert.Equal(t, `{"a":1}`, tpl.BodyRaw)
		assert.Contains(t, tpl.PostScript.Code, `client.test("ok", function() {})`)
	})

	t.Run("response handler script without body", func(t *testing.T) {
		file, err := ParseFile(
			"GET https://localhost:8080/a\n\n" +
				"> {% client.test(\"inline\", function() {}) %}",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		tpl := file.Templates[0]
		assert.Empty(t, tpl.BodyRaw)
		assert.Contains(t, tpl.PostScript.Code, `client.test("inline", function() {})`)
	})

	t.Run("response handler script file reference", func(t *testing.T) {
		file, err := ParseFile(
			"GET https://localhost:8080/a\n\n" +
				"> check.js",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)
		assert.Equal(t, "check.js", file.Templates[0].PostScript.Path)
	})

	t.Run("response redirect >> is ignored", func(t *testing.T) {
		file, err := ParseFile(
			"GET https://localhost:8080/a\n\n" +
				">> out.json",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)
		assert.Empty(t, file.Templates[0].PostScript.Code)
		assert.Empty(t, file.Templates[0].PostScript.Path)
		assert.Empty(t, file.Templates[0].BodyRaw)
	})

	t.Run("empty content yields no templates", func(t *testing.T) {
		file, err := ParseFile("   \n  \n")
		require.NoError(t, err)
		assert.Empty(t, file.Templates)
	})
}

func TestTemplateBuild(t *testing.T) {
	t.Run("resolution happens at build time", func(t *testing.T) {
		file, err := ParseFile("GET https://{{host}}/path")
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)
		tpl := file.Templates[0]

		first, err := tpl.Build(func(s string) string {
			return strings.ReplaceAll(s, "{{host}}", "one.example")
		}, "")
		require.NoError(t, err)
		assert.Equal(t, "one.example", first.URL.Host)

		second, err := tpl.Build(func(s string) string {
			return strings.ReplaceAll(s, "{{host}}", "two.example")
		}, "")
		require.NoError(t, err)
		assert.Equal(t, "two.example", second.URL.Host)
	})

	t.Run("identity build matches Create output", func(t *testing.T) {
		content := "POST https://localhost:8080/json\n" +
			"Content-Type: application/json\n\n" +
			`{"name":"John Doe","age":25}`

		file, err := ParseFile(content)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		built, err := file.Templates[0].Build(identity, "")
		require.NoError(t, err)

		assert.Equal(t, "POST", built.Method)
		assert.Equal(t, "application/json", built.Header.Get("Content-Type"))
		body, err := io.ReadAll(built.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"name":"John Doe","age":25}`, string(body))
	})

	t.Run("resolver substitutes in headers and body", func(t *testing.T) {
		file, err := ParseFile(
			"POST https://localhost:8080/json\n" +
				"Authorization: Bearer {{token}}\n" +
				"Content-Type: application/json\n\n" +
				`{"id":"{{id}}"}`,
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		resolve := func(s string) string {
			s = strings.ReplaceAll(s, "{{token}}", "tok-1")
			return strings.ReplaceAll(s, "{{id}}", "42")
		}
		built, err := file.Templates[0].Build(resolve, "")
		require.NoError(t, err)

		assert.Equal(t, "Bearer tok-1", built.Header.Get("Authorization"))
		body, err := io.ReadAll(built.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"id":"42"}`, string(body))
	})
}
