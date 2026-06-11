package request

import (
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"os"
	"testing"
)

const formContent = `POST https://localhost:8080/form-data
Content-Type: multipart/form-data; boundary=foo

--foo
Content-Disposition: form-data; name="image"; filename="Cargo.lock"
Content-Type: application/octet-stream

< ../Cargo.lock
--foo
content-Disposition: form-data; name="title"
Content-Type: text/plain

test text

foobar
--foo--`

func TestSplitRequests(t *testing.T) {
	file, err := os.OpenFile("../../testdata/three.http", os.O_RDONLY, 0)
	assert.NoError(t, err)
	defer file.Close()

	content, err := io.ReadAll(file)
	assert.NoError(t, err)

	parts := splitRequests(string(content))

	assert.Len(t, parts, 3)
}

func TestSplitRequest(t *testing.T) {
	essentials, headers, body := splitRequest(formContent)

	assert.Equal(t, "POST https://localhost:8080/form-data", essentials)
	assert.Equal(t, "Content-Type: multipart/form-data; boundary=foo", headers)
	assert.Equal(t, `--foo
Content-Disposition: form-data; name="image"; filename="Cargo.lock"
Content-Type: application/octet-stream

< ../Cargo.lock
--foo
content-Disposition: form-data; name="title"
Content-Type: text/plain

test text

foobar
--foo--`, body)
}

func TestParseEssentials(t *testing.T) {
	method, parsedUrl, proto := parseEssentials("GET https://localhost:8080/http2?{{param}}&query&param1=foobar HTTP/2")

	assert.Equal(t, http.MethodGet, method)
	assert.NotNil(t, parsedUrl)
	assert.Equal(t, "HTTP/2", proto)
}
