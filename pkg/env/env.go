package env

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Environment map[string]interface{}

func (e Environment) Replace(content string) string {
	oldNew := make([]string, 0, len(e)*2)
	for search, replace := range e {
		oldNew = append(
			oldNew,
			fmt.Sprintf("{{%s}}", search),
			fmt.Sprint(replace),
		)
	}

	return strings.NewReplacer(oldNew...).Replace(content)
}

type EnvironmentMap map[string]Environment

func (m EnvironmentMap) Get(name string) Environment {
	return m[name]
}

func Parse(root *os.Root, path string) (EnvironmentMap, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	f, err := root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open env file: %w", err)
	}

	defer func() {
		_ = f.Close()
	}()

	var result EnvironmentMap
	if err = json.NewDecoder(f).Decode(&result); err != nil {
		return nil, fmt.Errorf("cannot decode env file: %w", err)
	}

	return result, nil
}
