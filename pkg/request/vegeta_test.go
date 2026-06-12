package request

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVegetaDirective(t *testing.T) {
	t.Run("absent by default", func(t *testing.T) {
		file, err := ParseFile("GET https://localhost:8080/a")
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)
		assert.Nil(t, file.Templates[0].Directives.Vegeta)
	})

	t.Run("bare directive uses defaults", func(t *testing.T) {
		file, err := ParseFile(
			"# @vegeta\n" +
				"GET https://localhost:8080/a",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		v := file.Templates[0].Directives.Vegeta
		require.NotNil(t, v)
		assert.Equal(t, Rate{Freq: 50, Per: time.Second}, v.Rate)
		assert.Equal(t, 10*time.Second, v.Duration)
		assert.Zero(t, v.Workers)
		assert.Zero(t, v.MaxWorkers)
		assert.Zero(t, v.Connections)
		assert.Equal(t, int64(-1), v.MaxBody)
	})

	t.Run("full params", func(t *testing.T) {
		file, err := ParseFile(
			"# @vegeta rate=100/m duration=30s workers=8 max-workers=16 connections=32 max-body=4096\n" +
				"GET https://localhost:8080/a",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		v := file.Templates[0].Directives.Vegeta
		require.NotNil(t, v)
		assert.Equal(t, Rate{Freq: 100, Per: time.Minute}, v.Rate)
		assert.Equal(t, 30*time.Second, v.Duration)
		assert.Equal(t, uint64(8), v.Workers)
		assert.Equal(t, uint64(16), v.MaxWorkers)
		assert.Equal(t, 32, v.Connections)
		assert.Equal(t, int64(4096), v.MaxBody)
	})

	t.Run("invalid values fall back to defaults", func(t *testing.T) {
		file, err := ParseFile(
			"# @vegeta rate=abc duration=xx workers=-1 max-body=zz unknown=1\n" +
				"GET https://localhost:8080/a",
		)
		require.NoError(t, err)
		require.Len(t, file.Templates, 1)

		v := file.Templates[0].Directives.Vegeta
		require.NotNil(t, v)
		assert.Equal(t, Rate{Freq: 50, Per: time.Second}, v.Rate)
		assert.Equal(t, 10*time.Second, v.Duration)
		assert.Zero(t, v.Workers)
		assert.Equal(t, int64(-1), v.MaxBody)
	})

	t.Run("rate per second shorthand", func(t *testing.T) {
		file, err := ParseFile(
			"# @vegeta rate=5/s duration=1s\n" +
				"GET https://localhost:8080/a",
		)
		require.NoError(t, err)

		v := file.Templates[0].Directives.Vegeta
		require.NotNil(t, v)
		assert.Equal(t, Rate{Freq: 5, Per: time.Second}, v.Rate)
		assert.Equal(t, time.Second, v.Duration)
	})
}
