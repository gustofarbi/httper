package env

import (
	"encoding/json"
	"fmt"
	"os"
)

type Environment map[string]interface{}

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

// Merge overlays private onto public per environment, key-wise: private
// values win (the JetBrains http-client.private.env.json convention). Inputs
// are left untouched.
func Merge(public, private EnvironmentMap) EnvironmentMap {
	if public == nil {
		return private
	}
	if private == nil {
		return public
	}

	merged := make(EnvironmentMap, len(public))
	for name, environment := range public {
		copied := make(Environment, len(environment))
		for k, v := range environment {
			copied[k] = v
		}
		merged[name] = copied
	}

	for name, environment := range private {
		if _, ok := merged[name]; !ok {
			merged[name] = make(Environment, len(environment))
		}
		for k, v := range environment {
			merged[name][k] = v
		}
	}

	return merged
}
