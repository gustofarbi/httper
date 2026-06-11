package request

import (
	"net/http"
	"strings"
)

// File is the parsed form of one .http file: in-file variables plus one
// request template per `###`-separated block.
type File struct {
	Vars      map[string]string
	Templates []*Template
}

// Template is a single request block with its raw sections still containing
// {{placeholders}}. Resolution happens at Build time, just before sending, so
// later requests can use variables produced by earlier responses.
type Template struct {
	Essentials string
	HeadersRaw string
	BodyRaw    string
}

// ParseFile splits .http content into request templates without resolving any
// placeholders.
func ParseFile(content string) (*File, error) {
	file := &File{Vars: make(map[string]string)}

	for _, part := range splitRequests(content) {
		part = extractVars(part, file.Vars)
		if part == "" {
			continue
		}

		essentials, headers, body := splitRequest(part)
		file.Templates = append(file.Templates, &Template{
			Essentials: essentials,
			HeadersRaw: headers,
			BodyRaw:    body,
		})
	}

	return file, nil
}

// extractVars consumes leading `@name = value` (or `@name value`) lines of a
// request block into vars and returns the remaining trimmed block text.
func extractVars(part string, vars map[string]string) string {
	lines := strings.Split(strings.TrimSpace(part), "\n")

	rest := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "@") {
			break
		}

		key, value, found := strings.Cut(trimmed[1:], "=")
		if !found {
			key, value, _ = strings.Cut(trimmed[1:], " ")
		}

		key = strings.TrimSpace(key)
		if key != "" {
			vars[key] = strings.TrimSpace(value)
		}
		rest++
	}

	return strings.TrimSpace(strings.Join(lines[rest:], "\n"))
}

// Build resolves placeholders in the template's sections and constructs the
// *http.Request. wd is the directory file includes resolve against.
func (t *Template) Build(resolve func(string) string, wd string) (*http.Request, error) {
	return buildRequest(
		resolve(t.Essentials),
		resolve(t.HeadersRaw),
		resolve(t.BodyRaw),
		wd,
	)
}
