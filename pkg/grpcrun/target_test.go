package grpcrun

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Target
	}{
		{
			name: "plaintext scheme",
			raw:  "grpc://example.com:9090/pkg.Service/Method",
			want: Target{
				Address:    "example.com:9090",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  true,
			},
		},
		{
			name: "tls scheme",
			raw:  "grpcs://example.com:9090/pkg.Service/Method",
			want: Target{
				Address:    "example.com:9090",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  false,
			},
		},
		{
			name: "bare remote host defaults to tls",
			raw:  "example.com:9090/pkg.Service/Method",
			want: Target{
				Address:    "example.com:9090",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  false,
			},
		},
		{
			name: "bare localhost defaults to plaintext",
			raw:  "localhost:8081/pkg.Service/Method",
			want: Target{
				Address:    "localhost:8081",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  true,
			},
		},
		{
			name: "bare loopback ip defaults to plaintext",
			raw:  "127.0.0.1:8081/pkg.Service/Method",
			want: Target{
				Address:    "127.0.0.1:8081",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  true,
			},
		},
		{
			name: "bare ipv6 loopback defaults to plaintext",
			raw:  "[::1]:8081/pkg.Service/Method",
			want: Target{
				Address:    "[::1]:8081",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  true,
			},
		},
		{
			name: "grpcs on localhost stays tls",
			raw:  "grpcs://localhost:8081/pkg.Service/Method",
			want: Target{
				Address:    "localhost:8081",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  false,
			},
		},
		{
			name: "no port defaults to 443 for tls",
			raw:  "example.com/pkg.Service/Method",
			want: Target{
				Address:    "example.com:443",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  false,
			},
		},
		{
			name: "no port defaults to 80 for plaintext",
			raw:  "grpc://example.com/pkg.Service/Method",
			want: Target{
				Address:    "example.com:80",
				Service:    "pkg.Service",
				Method:     "Method",
				FullMethod: "/pkg.Service/Method",
				PlainText:  true,
			},
		},
		{
			name: "nested package",
			raw:  "grpc://localhost:8081/a.b.c.Service/Do",
			want: Target{
				Address:    "localhost:8081",
				Service:    "a.b.c.Service",
				Method:     "Do",
				FullMethod: "/a.b.c.Service/Do",
				PlainText:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTargetErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"missing method", "localhost:8081/pkg.Service"},
		{"missing service", "localhost:8081//Method"},
		{"missing path", "localhost:8081"},
		{"too many segments", "localhost:8081/pkg.Service/Method/extra"},
		{"unsupported scheme", "http://localhost:8081/pkg.Service/Method"},
		{"missing host", "grpc:///pkg.Service/Method"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTarget(tt.raw)
			assert.Error(t, err)
		})
	}
}
