package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplace(t *testing.T) {
	tests := []struct {
		name    string
		env     Environment
		content string
		want    string
	}{
		{
			name:    "string value",
			env:     Environment{"host": "localhost"},
			content: "GET https://{{host}}/",
			want:    "GET https://localhost/",
		},
		{
			name:    "numeric value stringified",
			env:     Environment{"foo": 1},
			content: "id={{foo}}",
			want:    "id=1",
		},
		{
			name:    "missing key left intact",
			env:     Environment{"host": "localhost"},
			content: "{{unknown}}",
			want:    "{{unknown}}",
		},
		{
			name:    "multiple occurrences",
			env:     Environment{"x": "v"},
			content: "{{x}}-{{x}}",
			want:    "v-v",
		},
		{
			name:    "empty env returns content unchanged",
			env:     Environment{},
			content: "{{x}}",
			want:    "{{x}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.env.Replace(tt.content))
		})
	}
}

func TestParse(t *testing.T) {
	root, err := os.OpenRoot("../../testdata")
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	envs, err := Parse(root, "http-client.env.json")
	require.NoError(t, err)

	dev := envs.Get("dev")
	require.NotNil(t, dev)
	assert.Equal(t, "param1=foobar", dev.Replace("{{param}}"))
}

func TestParseErrors(t *testing.T) {
	root, err := os.OpenRoot("../../testdata")
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	t.Run("empty path", func(t *testing.T) {
		_, err := Parse(root, "")
		assert.Error(t, err)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := Parse(root, "does-not-exist.json")
		assert.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		_, err := Parse(root, "bad-env.json")
		assert.Error(t, err)
	})
}
