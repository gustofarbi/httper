package main

import (
	"testing"

	"httper/pkg/request"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterTemplates(t *testing.T) {
	templates := []*request.Template{
		{Name: "Login"},
		{Name: "Fetch"},
		{Name: "#3"},
	}

	t.Run("empty filter keeps all", func(t *testing.T) {
		got, err := filterTemplates(templates, "")
		require.NoError(t, err)
		assert.Len(t, got, 3)
	})

	t.Run("single name", func(t *testing.T) {
		got, err := filterTemplates(templates, "Fetch")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "Fetch", got[0].Name)
	})

	t.Run("comma-separated names keep file order", func(t *testing.T) {
		got, err := filterTemplates(templates, "Fetch,Login")
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "Login", got[0].Name)
		assert.Equal(t, "Fetch", got[1].Name)
	})

	t.Run("no match errors", func(t *testing.T) {
		_, err := filterTemplates(templates, "nope")
		assert.Error(t, err)
	})
}
