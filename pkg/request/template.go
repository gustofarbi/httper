package request

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// File is the parsed form of one .http file: in-file variables plus one
// request template per `###`-separated block.
type File struct {
	Vars      map[string]string
	Templates []*Template
}

// Directives are per-request execution options set via `# @...` comments.
type Directives struct {
	NoRedirect  bool
	NoCookieJar bool
	NoLog       bool
	Timeout     time.Duration
	Vegeta      *VegetaDirective
}

// Script is a pre-request or response handler script: inline `{% ... %}`
// code, or a path to a .js file relative to the .http file.
type Script struct {
	Code string
	Path string
}

func (s Script) Empty() bool {
	return s.Code == "" && s.Path == ""
}

// Template is a single request block with its raw sections still containing
// {{placeholders}}. Resolution happens at Build time, just before sending, so
// later requests can use variables produced by earlier responses. Scripts are
// never resolved — {{ }} inside script source stays untouched.
type Template struct {
	Name       string
	Directives Directives
	Essentials string
	HeadersRaw string
	BodyRaw    string
	PreScript  Script
	PostScript Script
}

// ParseFile splits .http content into request templates without resolving any
// placeholders. `@name = value` lines outside request blocks become file
// variables; `# @...` comment lines before the request line become the
// template's name and directives.
func ParseFile(content string) (*File, error) {
	file := &File{Vars: make(map[string]string)}

	for _, b := range splitBlocks(content) {
		template := parseBlock(b.lines, file.Vars)
		if template == nil {
			continue
		}

		if template.Name == "" {
			template.Name = b.title
		}
		if template.Name == "" {
			template.Name = fmt.Sprintf("#%d", len(file.Templates)+1)
		}

		file.Templates = append(file.Templates, template)
	}

	return file, nil
}

type block struct {
	title string
	lines []string
}

// splitBlocks separates content on `###` lines; text after the separator
// hashes becomes the next block's title.
func splitBlocks(content string) []block {
	blocks := make([]block, 0, 1)
	current := block{}

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "###") {
			blocks = append(blocks, current)
			current = block{title: strings.TrimSpace(strings.TrimLeft(line, "#"))}
			continue
		}
		current.lines = append(current.lines, line)
	}

	return append(blocks, current)
}

// parseBlock scans one request block line by line. Before the request line it
// collects `@var` definitions and `#`/`//` comments (directives included);
// after it, headers run until the first blank line and everything else is the
// body, left untouched. Returns nil when the block holds no request line.
func parseBlock(lines []string, fileVars map[string]string) *Template {
	const (
		sectionPre = iota
		sectionHead
		sectionBody
	)

	template := &Template{}
	section := sectionPre
	var headers, body []string

	appendContinuation := func(line string) {
		joined := line[len("    "):]
		if len(headers) > 0 {
			headers[len(headers)-1] += joined
		} else {
			template.Essentials += joined
		}
	}

	// Script collection: openScript starts an inline `{% ... %}` block (which
	// may close on the same line); while open, lines accumulate until `%}`.
	var scriptLines []string
	var scriptDone func(code string)

	openScript := func(rest string, done func(string)) {
		if before, _, closed := strings.Cut(rest, "%}"); closed {
			done(strings.TrimSpace(before))
			return
		}
		scriptLines = scriptLines[:0]
		if strings.TrimSpace(rest) != "" {
			scriptLines = append(scriptLines, strings.TrimSpace(rest))
		}
		scriptDone = done
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if scriptDone != nil {
			if before, _, closed := strings.Cut(line, "%}"); closed {
				scriptLines = append(scriptLines, before)
				scriptDone(strings.TrimSpace(strings.Join(scriptLines, "\n")))
				scriptDone = nil
				continue
			}
			scriptLines = append(scriptLines, line)
			continue
		}

		switch section {
		case sectionPre:
			switch {
			case trimmed == "":
			case strings.HasPrefix(trimmed, "< {%"):
				openScript(scriptRest(trimmed), func(code string) { template.PreScript.Code = code })
			case strings.HasPrefix(trimmed, "@"):
				key, value := parseVarLine(trimmed)
				if key != "" {
					fileVars[key] = value
				}
			case isComment(trimmed):
				applyDirective(trimmed, template)
			default:
				template.Essentials = trimmed
				section = sectionHead
			}
		case sectionHead:
			switch {
			case trimmed == "":
				section = sectionBody
			case strings.HasPrefix(line, "    "):
				appendContinuation(line)
			case isComment(trimmed):
			default:
				headers = append(headers, trimmed)
			}
		case sectionBody:
			switch {
			case strings.HasPrefix(trimmed, "> {%"):
				openScript(scriptRest(trimmed), func(code string) { template.PostScript.Code = code })
			case strings.HasPrefix(trimmed, ">>"):
				slog.Warn("response redirect '>>' is not supported, ignoring", "line", trimmed)
			case strings.HasPrefix(trimmed, "> "):
				template.PostScript.Path = strings.TrimSpace(trimmed[len("> "):])
			default:
				body = append(body, line)
			}
		}
	}

	if template.Essentials == "" {
		return nil
	}

	template.HeadersRaw = strings.Join(headers, "\n")
	template.BodyRaw = strings.TrimSpace(strings.Join(body, "\n"))

	return template
}

// scriptRest strips the leading `<`/`>` marker and the `{%` opener, returning
// any code that follows on the same line.
func scriptRest(trimmed string) string {
	rest := strings.TrimSpace(trimmed[1:])
	return strings.TrimPrefix(rest, "{%")
}

func isComment(trimmed string) bool {
	return strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//")
}

// parseVarLine reads `@name = value` or `@name value` into a key/value pair.
func parseVarLine(trimmed string) (key, value string) {
	key, value, found := strings.Cut(trimmed[1:], "=")
	if !found {
		key, value, _ = strings.Cut(trimmed[1:], " ")
	}

	return strings.TrimSpace(key), strings.TrimSpace(value)
}

// applyDirective interprets a pre-request comment as `@name` or an execution
// directive; non-directive comments are ignored.
func applyDirective(comment string, template *Template) {
	text := strings.TrimPrefix(comment, "//")
	text = strings.TrimLeft(text, "#")
	text = strings.TrimSpace(text)

	if !strings.HasPrefix(text, "@") {
		return
	}

	directive, arg, _ := strings.Cut(text[1:], " ")
	arg = strings.TrimSpace(arg)

	switch directive {
	case "name":
		template.Name = arg
	case "no-redirect":
		template.Directives.NoRedirect = true
	case "no-cookie-jar":
		template.Directives.NoCookieJar = true
	case "no-log":
		template.Directives.NoLog = true
	case "timeout":
		seconds, err := strconv.Atoi(arg)
		if err != nil {
			return
		}
		template.Directives.Timeout = time.Duration(seconds) * time.Second
	case "vegeta":
		template.Directives.Vegeta = parseVegetaDirective(arg)
	}
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
