package vars

import (
	"bytes"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResolvePrecedence(t *testing.T) {
	globals := NewGlobals()
	globals.Set("key", "from-globals")
	globals.Set("g", "global-only")

	store := NewStore(
		map[string]string{"key": "from-env", "e": "env-only"},
		map[string]string{"key": "from-file", "f": "file-only"},
		globals,
	)
	store.SetLocal("key", "from-local")
	store.SetLocal("l", "local-only")

	assert.Equal(t, "from-local", store.Resolve("{{key}}"))
	assert.Equal(t, "local-only", store.Resolve("{{l}}"))
	assert.Equal(t, "global-only", store.Resolve("{{g}}"))
	assert.Equal(t, "file-only", store.Resolve("{{f}}"))
	assert.Equal(t, "env-only", store.Resolve("{{e}}"))
}

func TestResolveLayerFallthrough(t *testing.T) {
	globals := NewGlobals()
	globals.Set("key", "from-globals")

	store := NewStore(
		map[string]string{"key": "from-env"},
		map[string]string{"key": "from-file"},
		globals,
	)

	// no local layer set: globals win over file vars, file vars over env
	assert.Equal(t, "from-globals", store.Resolve("{{key}}"))

	store2 := NewStore(
		map[string]string{"key": "from-env"},
		map[string]string{"key": "from-file"},
		NewGlobals(),
	)
	assert.Equal(t, "from-file", store2.Resolve("{{key}}"))
}

func TestResolveUnknownKeyLeftVerbatim(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	assert.Equal(t, "x {{missing}} y", store.Resolve("x {{missing}} y"))
}

func TestResolveWhitespaceInsideBraces(t *testing.T) {
	store := NewStore(map[string]string{"host": "example.com"}, nil, NewGlobals())
	assert.Equal(t, "https://example.com/", store.Resolve("https://{{ host }}/"))
}

func TestClearLocal(t *testing.T) {
	store := NewStore(map[string]string{"key": "from-env"}, nil, NewGlobals())
	store.SetLocal("key", "from-local")
	store.ClearLocal()
	assert.Equal(t, "from-env", store.Resolve("{{key}}"))
}

func TestDynamicUUID(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	// deterministic randomness: all zero bytes
	store.Rand = bytes.NewReader(make([]byte, 64))

	got := store.Resolve("{{$uuid}}")
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), got)
}

func TestDynamicTimestamps(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	fixed := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return fixed }

	assert.Equal(t, "1781179200", store.Resolve("{{$timestamp}}"))
	assert.Equal(t, "2026-06-11T12:00:00Z", store.Resolve("{{$isoTimestamp}}"))
}

func TestDynamicRandomInt(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	store.Rand = bytes.NewReader(make([]byte, 64))

	got := store.Resolve("{{$randomInt}}")
	assert.Regexp(t, regexp.MustCompile(`^\d{1,4}$`), got)
}

func TestGlobalsRoundTrip(t *testing.T) {
	g := NewGlobals()
	g.Set("token", "abc")
	v, ok := g.Get("token")
	assert.True(t, ok)
	assert.Equal(t, "abc", v)

	_, ok = g.Get("nope")
	assert.False(t, ok)
}

func TestDynamicEnv(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	store.Getenv = func(name string) string {
		if name == "API_TOKEN" {
			return "sekret"
		}
		return ""
	}

	assert.Equal(t, "sekret", store.Resolve("{{$env.API_TOKEN}}"))
	// Unset OS vars resolve to empty rather than leaking the literal
	// placeholder to the server.
	assert.Equal(t, "x  y", store.Resolve("x {{$env.MISSING}} y"))
}

func TestDynamicRandomInteger(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	store.Rand = bytes.NewReader(make([]byte, 64))

	// from inclusive, to exclusive; zero randomness hits from.
	assert.Equal(t, "5", store.Resolve("{{$random.integer(5, 10)}}"))

	// malformed args stay verbatim like any unknown placeholder
	store.Rand = bytes.NewReader(make([]byte, 64))
	assert.Equal(t, "{{$random.integer(abc)}}", store.Resolve("{{$random.integer(abc)}}"))
	assert.Equal(t, "{{$random.integer(10, 5)}}", store.Resolve("{{$random.integer(10, 5)}}"))
}

func TestDynamicRandomAlphabetic(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	store.Rand = bytes.NewReader(make([]byte, 64))

	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z]{8}$`), store.Resolve("{{$random.alphabetic(8)}}"))
	assert.Equal(t, "{{$random.alphabetic(-1)}}", store.Resolve("{{$random.alphabetic(-1)}}"))
}

func TestDynamicRandomEmail(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	store.Rand = bytes.NewReader(make([]byte, 64))

	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z]+@[A-Za-z]+\.example$`), store.Resolve("{{$random.email}}"))
}

func TestDynamicRandomUUIDAlias(t *testing.T) {
	store := NewStore(nil, nil, NewGlobals())
	store.Rand = bytes.NewReader(make([]byte, 64))

	got := store.Resolve("{{$random.uuid}}")
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), got)
}

func TestResolvePrecedenceWithCLI(t *testing.T) {
	globals := NewGlobals()
	globals.Set("key", "from-globals")

	store := NewStore(
		map[string]string{"key": "from-env"},
		map[string]string{"key": "from-file", "f": "file-only"},
		globals,
	)
	store.SetCLI(map[string]string{"key": "from-cli", "c": "cli-only", "f": "from-cli"})

	// runtime-produced values still win over CLI…
	store.SetLocal("key", "from-local")
	assert.Equal(t, "from-local", store.Resolve("{{key}}"))
	store.ClearLocal()
	assert.Equal(t, "from-globals", store.Resolve("{{key}}"))

	// …but CLI beats everything declared in files
	assert.Equal(t, "from-cli", store.Resolve("{{f}}"))
	assert.Equal(t, "cli-only", store.Resolve("{{c}}"))
}
