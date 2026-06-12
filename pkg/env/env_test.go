package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	root, err := os.OpenRoot("../../testdata")
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	envs, err := Parse(root, "http-client.env.json")
	require.NoError(t, err)

	dev := envs.Get("dev")
	require.NotNil(t, dev)
	assert.Equal(t, "param1=foobar", dev["param"])
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

func TestMerge(t *testing.T) {
	public := EnvironmentMap{
		"dev":  {"host": "dev.example", "token": "public"},
		"prod": {"host": "prod.example"},
	}
	private := EnvironmentMap{
		"dev":   {"token": "secret"},
		"stage": {"host": "stage.example"},
	}

	merged := Merge(public, private)

	assert.Equal(t, "dev.example", merged.Get("dev")["host"])
	assert.Equal(t, "secret", merged.Get("dev")["token"], "private overrides public")
	assert.Equal(t, "prod.example", merged.Get("prod")["host"])
	assert.Equal(t, "stage.example", merged.Get("stage")["host"], "private-only env survives")

	assert.Equal(t, "public", public.Get("dev")["token"], "inputs unchanged")

	assert.Nil(t, Merge(nil, nil))
	assert.Equal(t, public, Merge(public, nil))
	assert.Equal(t, private, Merge(nil, private))
}
