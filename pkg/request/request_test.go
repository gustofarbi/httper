package request

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
