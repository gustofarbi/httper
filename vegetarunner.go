package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"httper/pkg/request"
	"httper/pkg/vegetarun"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// VegetaRunner is the load-test sibling of Runner: it attacks an already
// built request per its `# @vegeta` profile. A nil VegetaRunner means the
// -vegeta flag is off and marked requests run as normal single requests.
type VegetaRunner struct {
	Out    io.Writer
	Config Config
	// Timeout is the global -timeout default; a per-request @timeout wins.
	Timeout time.Duration
	// TLSConfig overrides the attacker's TLS settings; nil keeps the secure
	// default (main wires the -insecure config here, tests their cert pool).
	TLSConfig *tls.Config
}

// Send runs the attack and maps the outcome onto a Result. StatusCode stays
// zero — Result.Vegeta marks it meaningless; failure is encoded in Err when
// any shot failed.
func (r *VegetaRunner) Send(template *request.Template, httpRequest *http.Request) *Result {
	result := &Result{Name: template.Name, Vegeta: true}

	d := template.Directives.Vegeta

	_, _ = fmt.Fprintf(r.Out, "%s (vegeta %d/%s for %s)\n",
		httpRequest.URL, d.Rate.Freq, rateUnit(d.Rate.Per), d.Duration)

	timeout := r.Timeout
	if template.Directives.Timeout > 0 {
		timeout = template.Directives.Timeout
	}

	outcome, err := vegetarun.Run(httpRequest, d, vegetarun.Options{
		Timeout:    timeout,
		NoRedirect: template.Directives.NoRedirect,
		HTTP2:      true,
		TLSConfig:  r.attackTLSConfig(),
	})
	if err != nil {
		slog.Error("running vegeta attack", "err", err, "request", template.Name)
		result.Err = err
		return result
	}

	result.Duration = outcome.Metrics.Duration + outcome.Metrics.Wait

	if template.Directives.NoLog {
		_, _ = fmt.Fprintln(r.Out, outcome.Summary())
	} else {
		_, _ = fmt.Fprintln(r.Out, strings.TrimRight(outcome.Report, "\n"))
	}

	if outcome.Failed() {
		result.Err = errors.New(outcome.Summary())
	}

	return result
}

func (r *VegetaRunner) attackTLSConfig() *tls.Config {
	if r.TLSConfig != nil {
		return r.TLSConfig
	}
	return insecureTLSConfig(r.Config.Insecure)
}

func rateUnit(per time.Duration) string {
	if per == time.Minute {
		return "m"
	}
	return "s"
}
