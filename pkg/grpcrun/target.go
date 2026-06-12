// Package grpcrun executes GRPC request templates: it parses the request-line
// target, resolves the service schema via server reflection, and invokes unary
// or server-streaming methods with JSON request bodies.
package grpcrun

import (
	"fmt"
	"net"
	"strings"
)

// Target is a parsed GRPC request-line URL: where to dial and which method to
// call.
type Target struct {
	Address    string // host:port for dialing
	Service    string // fully-qualified "package.Service"
	Method     string // method name
	FullMethod string // "/package.Service/Method" for Invoke/NewStream
	PlainText  bool
}

// ParseTarget parses a GRPC request-line URL of the form
// [grpc://|grpcs://]host[:port]/package.Service/Method. grpc:// forces
// plaintext and grpcs:// forces TLS; a bare host defaults to TLS except for
// loopback hosts (localhost, 127.0.0.1, ::1). A missing port defaults to 443
// for TLS and 80 for plaintext.
func ParseTarget(raw string) (Target, error) {
	rest := raw
	scheme := ""
	if i := strings.Index(raw, "://"); i >= 0 {
		scheme, rest = raw[:i], raw[i+len("://"):]
	}

	slash := strings.Index(rest, "/")
	if slash < 0 {
		return Target{}, fmt.Errorf("grpc target %q: missing /package.Service/Method path", raw)
	}
	hostport, path := rest[:slash], rest[slash+1:]
	if hostport == "" {
		return Target{}, fmt.Errorf("grpc target %q: missing host", raw)
	}

	segments := strings.Split(path, "/")
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
		return Target{}, fmt.Errorf("grpc target %q: path must be /package.Service/Method", raw)
	}
	service, method := segments[0], segments[1]

	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = strings.Trim(hostport, "[]")
		hostport = "" // no port given; filled in below
	}

	var plaintext bool
	switch scheme {
	case "grpc":
		plaintext = true
	case "grpcs":
		plaintext = false
	case "":
		plaintext = isLoopback(host)
	default:
		return Target{}, fmt.Errorf("grpc target %q: unsupported scheme %q (use grpc:// or grpcs://)", raw, scheme)
	}

	if hostport == "" {
		port := "443"
		if plaintext {
			port = "80"
		}
		hostport = net.JoinHostPort(host, port)
	}

	return Target{
		Address:    hostport,
		Service:    service,
		Method:     method,
		FullMethod: "/" + service + "/" + method,
		PlainText:  plaintext,
	}, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}

	return false
}
