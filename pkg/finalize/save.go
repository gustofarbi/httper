package finalize

import (
	"errors"
	"fmt"
	"github.com/gabriel-vasile/mimetype"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func saveResponse(root *os.Root, response *http.Response, body []byte) error {
	prefix, err := getFilePrefix(root)
	if err != nil {
		return fmt.Errorf("getting file prefix: %w", err)
	}

	extension := getExtension(response, body)

	filePath := filepath.Join(prefix, getFilename(response.StatusCode, extension))
	file, err := root.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}

	defer func() {
		_ = file.Close()
	}()

	if _, err = file.Write(body); err != nil {
		return fmt.Errorf("writing response body: %w", err)
	}

	return nil
}

func getFilePrefix(root *os.Root) (string, error) {
	const (
		ideaDir   = ".idea"
		prefixDir = ".idea/httpRequests"
	)

	// root.Mkdir is non-recursive, so create each level, tolerating dirs that
	// already exist (e.g. across multiple requests in the same run).
	for _, dir := range []string{ideaDir, prefixDir} {
		if err := root.Mkdir(dir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("creating dir %s: %w", dir, err)
		}
	}

	return prefixDir, nil
}

func getExtension(response *http.Response, body []byte) string {
	if ext := mimetype.Detect(body).Extension(); ext != "" {
		return ext
	}

	exts, err := mime.ExtensionsByType(response.Header.Get("Content-Type"))
	if err == nil && len(exts) > 0 {
		sort.Sort(sort.Reverse(sort.StringSlice(exts)))
		return exts[0]
	}

	return ".txt"
}

// now is overridable in tests so saved filenames are deterministic.
var now = time.Now

func getFilename(statusCode int, ext string) string {
	// Include sub-second precision so multiple responses saved within the same
	// second don't collide and overwrite each other.
	return fmt.Sprintf(
		"%s.%d%s",
		now().Format("2006-01-02T150405.000000000"),
		statusCode,
		ext,
	)
}
