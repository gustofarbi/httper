package finalize

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedClock(t *testing.T) {
	t.Helper()
	orig := now
	now = func() time.Time {
		return time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC)
	}
	t.Cleanup(func() { now = orig })
}

func TestSaveResponse(t *testing.T) {
	fixedClock(t)

	tmp := t.TempDir()
	root, err := os.OpenRoot(tmp)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	require.NoError(t, saveResponse(root, http.StatusOK, "application/json", []byte(`{"a":1}`)))

	want := filepath.Join(tmp, ".idea", "httpRequests", "2024-01-02T150405.000000000.200.json")
	data, err := os.ReadFile(want)
	require.NoError(t, err, "expected saved file at %s", want)
	assert.Equal(t, `{"a":1}`, string(data))
}

func TestGetExtension(t *testing.T) {
	t.Run("sniffed from body", func(t *testing.T) {
		assert.Equal(t, ".json", getExtension("", []byte(`{"a":1}`)))
	})

	t.Run("fallback to content-type header", func(t *testing.T) {
		// A body mimetype cannot detect falls back to the Content-Type header
		// (reverse-sorted, so text/html resolves to .shtml).
		ext := getExtension("text/html", []byte("\x00\x01\x02not-detectable"))
		assert.True(t, strings.HasSuffix(ext, "html"), "got %q", ext)
	})

	t.Run("fallback to .txt when nothing matches", func(t *testing.T) {
		assert.Equal(t, ".txt", getExtension("", []byte("\x00\x01\x02not-detectable")))
	})
}
