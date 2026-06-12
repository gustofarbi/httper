package request

import (
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Rate is a request frequency per time unit, e.g. 50/s.
type Rate struct {
	Freq int
	Per  time.Duration
}

// VegetaDirective is the load profile declared by `# @vegeta` on a request
// block. Zero Workers/MaxWorkers/Connections leave the attacker defaults.
type VegetaDirective struct {
	Rate        Rate
	Duration    time.Duration
	Workers     uint64
	MaxWorkers  uint64
	Connections int
	MaxBody     int64
}

// parseVegetaDirective reads `@vegeta key=value ...` args. Invalid or unknown
// pairs are warned about and skipped, keeping the defaults.
func parseVegetaDirective(arg string) *VegetaDirective {
	v := &VegetaDirective{
		Rate:     Rate{Freq: 50, Per: time.Second},
		Duration: 10 * time.Second,
		MaxBody:  -1,
	}

	for _, pair := range strings.Fields(arg) {
		key, value, found := strings.Cut(pair, "=")
		if !found || !applyVegetaParam(v, key, value) {
			slog.Warn("ignoring invalid @vegeta parameter", "param", pair)
		}
	}

	return v
}

func applyVegetaParam(v *VegetaDirective, key, value string) bool {
	switch key {
	case "rate":
		rate, ok := parseRate(value)
		if ok {
			v.Rate = rate
		}
		return ok
	case "duration":
		d, ok := parsePositiveDuration(value)
		if ok {
			v.Duration = d
		}
		return ok
	case "workers":
		n, ok := parseUint(value)
		if ok {
			v.Workers = n
		}
		return ok
	case "max-workers":
		n, ok := parseUint(value)
		if ok {
			v.MaxWorkers = n
		}
		return ok
	case "connections":
		n, ok := parseInt(value, 32)
		if ok {
			v.Connections = int(n)
		}
		return ok
	case "max-body":
		n, ok := parseInt(value, 64)
		if ok {
			v.MaxBody = n
		}
		return ok
	default:
		return false
	}
}

// parseRate reads `N/unit` where unit is s or m.
func parseRate(value string) (Rate, bool) {
	freq, unit, found := strings.Cut(value, "/")
	if !found {
		return Rate{}, false
	}

	n, err := strconv.Atoi(freq)
	if err != nil || n <= 0 {
		return Rate{}, false
	}

	switch unit {
	case "s":
		return Rate{Freq: n, Per: time.Second}, true
	case "m":
		return Rate{Freq: n, Per: time.Minute}, true
	default:
		return Rate{}, false
	}
}

func parsePositiveDuration(value string) (time.Duration, bool) {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

func parseUint(value string) (uint64, bool) {
	n, err := strconv.ParseUint(value, 10, 64)
	return n, err == nil
}

func parseInt(value string, bitSize int) (int64, bool) {
	n, err := strconv.ParseInt(value, 10, bitSize)
	return n, err == nil && n > 0
}
