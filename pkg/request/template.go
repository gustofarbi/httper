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
		part = strings.TrimSpace(part)
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
