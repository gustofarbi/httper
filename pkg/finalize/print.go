package finalize

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// PrettyPrintJSON formats and prints JSON content
func PrettyPrintJSON(content string) {
	var jsonData interface{}

	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	err := json.Unmarshal([]byte(content), &jsonData)
	if err != nil {
		// Not valid JSON, just print it as is
		fmt.Println(content)
		return
	}

	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		slog.Error("formatting JSON", "err", err)
		fmt.Println(content)
		return
	}

	fmt.Println(string(prettyJSON))
}
