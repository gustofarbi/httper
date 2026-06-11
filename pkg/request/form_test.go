package request

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

func TestGetFormDataBody(t *testing.T) {
	//reader := getFormDataBody("foo", formContent)
}

func TestGetFiles(t *testing.T) {
	// Includes are resolved relative to wd and sandboxed to it via os.Root,
	// so reference the fixtures by base name with wd set to the testdata dir.
	bodyRaw := `< bearer.http
< get.http`

	r, err := getFiles(bodyRaw, "../../testdata")
	assert.NoError(t, err)

	actual := new(bytes.Buffer)

	if _, err = io.Copy(actual, r); err != nil {
		t.Fatal(err)
	}

	expected := `GET https://localhost:8080/bearer
Authorization: Bearer 42069

GET https://localhost:8080/?{{param}}&query&param1=foobar
`

	assert.Equal(t, expected, actual.String())
}
