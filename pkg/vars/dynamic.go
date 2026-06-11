package vars

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strconv"
	"time"
)

// defaultRand sources dynamic-variable randomness from crypto/rand.
var defaultRand = rand.Reader

// dynamic computes a $-prefixed variable, matching the JetBrains HTTP Client
// built-ins. Unknown names report not-found so they stay verbatim in the text.
func (s *Store) dynamic(key string) (string, bool) {
	switch key {
	case "$uuid":
		return s.uuid(), true
	case "$timestamp":
		return strconv.FormatInt(s.Now().Unix(), 10), true
	case "$isoTimestamp":
		return s.Now().UTC().Format(time.RFC3339), true
	case "$randomInt":
		return s.randomInt(), true
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
	var b [8]byte
	if _, err := s.Rand.Read(b[:]); err != nil {
		return ""
	}

	return strconv.FormatUint(binary.BigEndian.Uint64(b[:])%1001, 10)
}
