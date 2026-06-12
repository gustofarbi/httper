// Package vegetarun executes a single built HTTP request under load via the
// vegeta library and aggregates the shots into metrics plus a rendered text
// report. The request is frozen before the attack: placeholders, scripts and
// body includes have already been resolved by the caller.
package vegetarun

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"httper/pkg/request"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

// Options carries per-run attacker settings derived from existing httper
// knobs (directives and CLI flags). Zero values keep vegeta defaults.
type Options struct {
	Timeout    time.Duration
	NoRedirect bool
	HTTP2      bool
	TLSConfig  *tls.Config
}

// Outcome is the aggregate of one attack.
type Outcome struct {
	Metrics vegeta.Metrics
	Report  string
}

// Failed reports whether any shot failed (success ratio below 100%).
func (o *Outcome) Failed() bool {
	return o.Metrics.Success < 1
}

// Summary is a one-line digest: ratio, request count and status histogram.
func (o *Outcome) Summary() string {
	codes := make([]string, 0, len(o.Metrics.StatusCodes))
	for code, n := range o.Metrics.StatusCodes {
		codes = append(codes, fmt.Sprintf("%s:%d", code, n))
	}
	sort.Strings(codes)

	return fmt.Sprintf("success %.2f%% (%d requests) [%s]",
		o.Metrics.Success*100, o.Metrics.Requests, strings.Join(codes, " "))
}

// Run attacks the request per the directive's load profile and blocks until
// the attack completes. Only setup problems (e.g. unreadable body) are
// errors; failing shots are reflected in the metrics.
func Run(req *http.Request, d *request.VegetaDirective, opts Options) (*Outcome, error) {
	target, err := toTarget(req)
	if err != nil {
		return nil, err
	}

	attacker := vegeta.NewAttacker(attackerOptions(d, opts)...)
	pacer := vegeta.ConstantPacer{Freq: d.Rate.Freq, Per: d.Rate.Per}

	var metrics vegeta.Metrics
	for res := range attacker.Attack(vegeta.NewStaticTargeter(target), pacer, d.Duration, req.URL.Path) {
		metrics.Add(res)
	}
	metrics.Close()

	var report strings.Builder
	if err := vegeta.NewTextReporter(&metrics).Report(&report); err != nil {
		return nil, fmt.Errorf("rendering vegeta report: %w", err)
	}

	return &Outcome{Metrics: metrics, Report: report.String()}, nil
}

// toTarget converts the built request into a static vegeta target, reading
// the body once.
func toTarget(req *http.Request) (vegeta.Target, error) {
	target := vegeta.Target{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: req.Header,
	}

	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return vegeta.Target{}, fmt.Errorf("reading request body: %w", err)
		}
		_ = req.Body.Close()
		target.Body = body
	}

	return target, nil
}

func attackerOptions(d *request.VegetaDirective, opts Options) []func(*vegeta.Attacker) {
	attackerOpts := []func(*vegeta.Attacker){
		vegeta.MaxBody(d.MaxBody),
		vegeta.HTTP2(opts.HTTP2),
	}

	if d.Workers > 0 {
		attackerOpts = append(attackerOpts, vegeta.Workers(d.Workers))
	}
	if d.MaxWorkers > 0 {
		attackerOpts = append(attackerOpts, vegeta.MaxWorkers(d.MaxWorkers))
	}
	if d.Connections > 0 {
		attackerOpts = append(attackerOpts, vegeta.Connections(d.Connections))
	}
	if opts.Timeout > 0 {
		attackerOpts = append(attackerOpts, vegeta.Timeout(opts.Timeout))
	}
	if opts.NoRedirect {
		attackerOpts = append(attackerOpts, vegeta.Redirects(vegeta.NoFollow))
	}
	if opts.TLSConfig != nil {
		attackerOpts = append(attackerOpts, vegeta.TLSConfig(opts.TLSConfig))
	}

	return attackerOpts
}
