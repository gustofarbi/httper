package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	root, err := os.OpenRoot("../../testdata")
	assert.NoError(t, err)

	envs, err := Parse(root, "http-client.env.json")
	assert.NoError(t, err)

	env := envs["dev"]
	assert.NotNil(t, env)
}
