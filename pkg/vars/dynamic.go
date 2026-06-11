package vars

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// defaultRand sources dynamic-variable randomness from crypto/rand.
var defaultRand = rand.Reader

// callRegex matches dynamic variables with arguments: $name(args).
var callRegex = regexp.MustCompile(`^(\$[\w.]+)\((.*)\)$`)

// dynamic computes a $-prefixed variable, matching the JetBrains HTTP Client
// built-ins. Unknown names (and malformed arguments) report not-found so they
// stay verbatim in the text.
func (s *Store) dynamic(key string) (string, bool) {
	if name, ok := strings.CutPrefix(key, "$env."); ok {
		value := s.Getenv(name)
		if value == "" {
			// Empty beats sending the literal {{$env.X}} to a server.
			slog.Warn("environment variable unset, resolving to empty", "name", name)
		}
		return value, true
	}

	if m := callRegex.FindStringSubmatch(key); m != nil {
		return s.dynamicCall(m[1], m[2])
	}

	switch key {
	case "$uuid", "$random.uuid":
		return s.uuid(), true
	case "$timestamp":
		return strconv.FormatInt(s.Now().Unix(), 10), true
	case "$isoTimestamp":
		return s.Now().UTC().Format(time.RFC3339), true
	case "$randomInt":
		return s.randomInt(), true
	case "$random.email":
		return s.randomAlphabetic(8) + "@" + s.randomAlphabetic(6) + ".example", true
	default:
		return "", false
	}
}

// dynamicCall evaluates argument-taking dynamic variables.
func (s *Store) dynamicCall(name, argsRaw string) (string, bool) {
	args := strings.Split(argsRaw, ",")
	for i := range args {
		args[i] = strings.TrimSpace(args[i])
	}

	switch name {
	case "$random.integer":
		if len(args) != 2 {
			return "", false
		}
		from, err1 := strconv.Atoi(args[0])
		to, err2 := strconv.Atoi(args[1])
		if err1 != nil || err2 != nil || from >= to {
			return "", false
		}
		// from inclusive, to exclusive — JetBrains semantics.
		// #nosec G115 -- from < to is validated above, so to-from is positive
		// and the modulo result fits an int.
		offset := int(s.randomUint64() % uint64(to-from))
		return strconv.Itoa(from + offset), true
	case "$random.alphabetic":
		if len(args) != 1 {
			return "", false
		}
		length, err := strconv.Atoi(args[0])
		if err != nil || length < 0 {
			return "", false
		}
		return s.randomAlphabetic(length), true
	default:
		return "", false
	}
}

// uuid generates a random (version 4, variant 1) UUID from s.Rand.
func (s *Store) uuid() string {
	var b [16]byte
	if _, err := s.Rand.Read(b[:]); err != nil {
		return ""
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// randomInt returns 0–1000 inclusive, the JetBrains $randomInt range.
func (s *Store) randomInt() string {
	return strconv.FormatUint(s.randomUint64()%1001, 10)
}

// randomUint64 draws 8 bytes from s.Rand; zero on read failure.
func (s *Store) randomUint64() uint64 {
	var b [8]byte
	if _, err := s.Rand.Read(b[:]); err != nil {
		return 0
	}

	return binary.BigEndian.Uint64(b[:])
}

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// randomAlphabetic returns length random letters from s.Rand.
func (s *Store) randomAlphabetic(length int) string {
	b := make([]byte, length)
	if _, err := s.Rand.Read(b); err != nil {
		return ""
	}

	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}

	return string(b)
}
