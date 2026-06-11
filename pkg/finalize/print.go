package finalize

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// PrettyPrintJSON formats and writes JSON content to w.
func PrettyPrintJSON(w io.Writer, content string) {
	var jsonData interface{}

	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	err := json.Unmarshal([]byte(content), &jsonData)
	if err != nil {
		// Not valid JSON, just print it as is
		_, _ = fmt.Fprintln(w, content)
		return
	}

	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		slog.Error("formatting JSON", "err", err)
		_, _ = fmt.Fprintln(w, content)
		return
	}

	_, _ = fmt.Fprintln(w, string(prettyJSON))
}
