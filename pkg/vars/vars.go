// Package vars resolves {{placeholder}} variables in request text from
// layered sources. Precedence, highest first: request-local (pre-request
// scripts) > globals (client.global) > in-file @vars > env file. Keys with a
// $ prefix are dynamic variables computed per occurrence.
package vars

import (
	"io"
	"log/slog"
	"regexp"
	"time"
)

var placeholderRegex = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*}}`)

// Globals holds variables set by response handler scripts via
// client.global.set; they persist across requests within one run.
type Globals struct {
	values map[string]string
}

func NewGlobals() *Globals {
	return &Globals{values: make(map[string]string)}
}

func (g *Globals) Set(key, value string) {
	g.values[key] = value
}

func (g *Globals) Get(key string) (string, bool) {
	v, ok := g.values[key]
	return v, ok
}

// Store resolves placeholders against the layered variable sources. Now and
// Rand back the dynamic variables and are injectable for deterministic tests.
type Store struct {
	Now  func() time.Time
	Rand io.Reader

	local   map[string]string
	globals *Globals
	file    map[string]string
	env     map[string]string
}

func NewStore(env, file map[string]string, globals *Globals) *Store {
	return &Store{
		Now:     time.Now,
		Rand:    defaultRand,
		local:   make(map[string]string),
		globals: globals,
		file:    file,
		env:     env,
	}
}

// SetLocal sets a request-local variable (highest precedence). Local
// variables are scoped to one request; call ClearLocal between requests.
func (s *Store) SetLocal(key, value string) {
	s.local[key] = value
}

func (s *Store) ClearLocal() {
	s.local = make(map[string]string)
}

// Resolve expands every {{key}} in text. Unknown keys are left verbatim.
func (s *Store) Resolve(text string) string {
	return placeholderRegex.ReplaceAllStringFunc(text, func(match string) string {
		key := placeholderRegex.FindStringSubmatch(match)[1]

		if value, ok := s.lookup(key); ok {
			return value
		}

		slog.Warn("unknown placeholder left as-is", "key", key)
		return match
	})
}

func (s *Store) lookup(key string) (string, bool) {
	if len(key) > 0 && key[0] == '$' {
		return s.dynamic(key)
	}

	if v, ok := s.local[key]; ok {
		return v, true
	}
	if v, ok := s.globals.Get(key); ok {
		return v, true
	}
	if v, ok := s.file[key]; ok {
		return v, true
	}
	if v, ok := s.env[key]; ok {
		return v, true
	}

	return "", false
}
