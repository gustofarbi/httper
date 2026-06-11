package request

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataDir is where file-include fixtures resolve. Includes must reference
// files by base name within this dir: os.Root rejects ".." traversal.
const testdataDir = "../../testdata"

// buildAll parses content and builds every template with an identity
// resolver, mirroring what the old eager Create did.
func buildAll(content, wd string) ([]*http.Request, error) {
	file, err := ParseFile(content)
	if err != nil {
		return nil, err
	}

	requests := make([]*http.Request, 0, len(file.Templates))
	for _, template := range file.Templates {
		built, err := template.Build(func(s string) string { return s }, wd)
		if err != nil {
			return nil, err
		}
		requests = append(requests, built)
	}

	return requests, nil
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wd      string
		wantErr bool
		check   func(t *testing.T, reqs []*http.Request)
	}{
		{
			name:    "get with query",
			content: "GET https://localhost:8080/?query&param1=foobar",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				assert.Equal(t, http.MethodGet, reqs[0].Method)
				assert.Equal(t, "query&param1=foobar", reqs[0].URL.RawQuery)
			},
		},
		{
			name: "bearer auth header preserved",
			content: "GET https://localhost:8080/bearer\n" +
				"Authorization: Bearer 42069",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				assert.Equal(t, "Bearer 42069", reqs[0].Header.Get("Authorization"))
			},
		},
		{
			name: "basic auth sets credentials and no raw header",
			content: "GET https://localhost:8080/basic-auth\n" +
				"Authorization: Basic foo bar",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				user, pass, ok := reqs[0].BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "foo", user)
				assert.Equal(t, "bar", pass)
				// transferHeaders skips re-adding the raw Authorization header,
				// then SetBasicAuth writes the encoded one — assert it is the
				// encoded form, not the literal "Basic foo bar".
				assert.NotEqual(t, "Basic foo bar", reqs[0].Header.Get("Authorization"))
			},
		},
		{
			name: "json body",
			content: "POST https://localhost:8080/json\n" +
				"Content-Type: application/json\n\n" +
				`{"name":"John Doe","age":25}`,
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				assert.Equal(t, http.MethodPost, reqs[0].Method)
				assert.Equal(t, "application/json", reqs[0].Header.Get("Content-Type"))
				body, err := io.ReadAll(reqs[0].Body)
				require.NoError(t, err)
				assert.Equal(t, `{"name":"John Doe","age":25}`, string(body))
			},
		},
		{
			name: "multipart form-data with file include",
			wd:   testdataDir,
			content: "POST https://localhost:8080/form-data\n" +
				"Content-Type: multipart/form-data; boundary=foo\n\n" +
				"--foo\n" +
				"Content-Disposition: form-data; name=\"file\"; filename=\"include.txt\"\n" +
				"Content-Type: text/plain\n\n" +
				"< include.txt\n" +
				"--foo\n" +
				"Content-Disposition: form-data; name=\"title\"\n" +
				"Content-Type: text/plain\n\n" +
				"hello\n" +
				"--foo--",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				body, err := io.ReadAll(reqs[0].Body)
				require.NoError(t, err)
				s := string(body)
				assert.Contains(t, s, "included file contents")
				assert.Contains(t, s, `name="title"`)
				assert.Contains(t, s, "hello")
			},
		},
		{
			name: "multiline url joins continuation lines",
			content: "GET https://localhost:8080/?\n" +
				"    foo=bar&\n" +
				"    baz=foo&\n" +
				"    query",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				assert.Equal(t, "foo=bar&baz=foo&query", reqs[0].URL.RawQuery)
			},
		},
		{
			name: "three requests split on ###",
			content: "GET https://localhost:8080/a\n###\n" +
				"GET https://localhost:8080/b\n###\n" +
				"POST https://localhost:8080/c",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 3)
				assert.Equal(t, "/a", reqs[0].URL.Path)
				assert.Equal(t, "/b", reqs[1].URL.Path)
				assert.Equal(t, http.MethodPost, reqs[2].Method)
			},
		},
		{
			name:    "explicit HTTP/2 proto",
			content: "GET https://localhost:8080/http2 HTTP/2",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				assert.Equal(t, "HTTP/2", reqs[0].Proto)
			},
		},
		{
			name:    "no method defaults to GET",
			content: "https://localhost:8080/",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				assert.Equal(t, http.MethodGet, reqs[0].Method)
			},
		},
		{
			name:    "unknown content-type sends body verbatim",
			content: "POST https://localhost:8080/x\nContent-Type: text/plain\n\nhello",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				body, err := io.ReadAll(reqs[0].Body)
				require.NoError(t, err)
				assert.Equal(t, "hello", string(body))
			},
		},
		{
			name:    "missing content-type sends body verbatim",
			content: "POST https://localhost:8080/x\n\n<note>hi</note>",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				body, err := io.ReadAll(reqs[0].Body)
				require.NoError(t, err)
				assert.Equal(t, "<note>hi</note>", string(body))
			},
		},
		{
			name:    "no body stays nil",
			content: "GET https://localhost:8080/x",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				assert.Nil(t, reqs[0].Body)
			},
		},
		{
			name: "form-urlencoded body joins lines with ampersand",
			content: "POST https://localhost:8080/urlencoded\n" +
				"Content-Type: application/x-www-form-urlencoded\n\n" +
				"a=1&\n" +
				"b=2&\n" +
				"c=hello+world",
			check: func(t *testing.T, reqs []*http.Request) {
				require.Len(t, reqs, 1)
				body, err := io.ReadAll(reqs[0].Body)
				require.NoError(t, err)
				assert.Equal(t, "a=1&b=2&c=hello+world", string(body))
			},
		},
		{
			name:    "empty content yields no requests",
			content: "   \n  \n",
			check: func(t *testing.T, reqs []*http.Request) {
				assert.Empty(t, reqs)
			},
		},
		{
			name: "missing include file errors",
			wd:   testdataDir,
			content: "POST https://localhost:8080/form-data\n" +
				"Content-Type: multipart/form-data; boundary=foo\n\n" +
				"--foo\n" +
				"Content-Disposition: form-data; name=\"file\"; filename=\"nope.txt\"\n\n" +
				"< nope.txt\n" +
				"--foo--",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs, err := buildAll(tt.content, tt.wd)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, reqs)
		})
	}
}
