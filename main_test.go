package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionString(t *testing.T) {
	assert.Equal(t, "httper dev", versionString())
}
