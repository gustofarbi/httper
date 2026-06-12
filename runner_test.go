package main

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/gustofarbi/httper/pkg/request"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

func TestClientFor(t *testing.T) {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	base := &http.Client{
		Timeout:   30 * time.Second,
		Jar:       jar,
		Transport: http.DefaultTransport,
	}

	runner := &Runner{Client: base}

	t.Run("no directives keeps base behavior", func(t *testing.T) {
		c := runner.clientFor(request.Directives{}, "")
		assert.Equal(t, base.Timeout, c.Timeout)
		assert.Equal(t, base.Jar, c.Jar)
		assert.Equal(t, base.Transport, c.Transport)
		assert.Nil(t, c.CheckRedirect)
	})

	t.Run("timeout directive overrides", func(t *testing.T) {
		c := runner.clientFor(request.Directives{Timeout: 5 * time.Second}, "")
		assert.Equal(t, 5*time.Second, c.Timeout)
		assert.Equal(t, 30*time.Second, base.Timeout, "base must not be mutated")
	})

	t.Run("no-redirect stops following", func(t *testing.T) {
		c := runner.clientFor(request.Directives{NoRedirect: true}, "")
		require.NotNil(t, c.CheckRedirect)
		assert.Equal(t, http.ErrUseLastResponse, c.CheckRedirect(nil, nil))
	})

	t.Run("no-cookie-jar clears jar", func(t *testing.T) {
		c := runner.clientFor(request.Directives{NoCookieJar: true}, "")
		assert.Nil(t, c.Jar)
		assert.NotNil(t, base.Jar, "base must not be mutated")
	})

	t.Run("http2 proto swaps transport on the copy only", func(t *testing.T) {
		c := runner.clientFor(request.Directives{}, "HTTP/2")
		assert.IsType(t, &http2.Transport{}, c.Transport)
		assert.Equal(t, http.DefaultTransport, base.Transport, "base must not be mutated")
	})
}

func TestClientForInsecure(t *testing.T) {
	base := &http.Client{Transport: http.DefaultTransport}

	t.Run("insecure h2 transport skips verification", func(t *testing.T) {
		r := &Runner{Client: base, Config: Config{Insecure: true}}
		c := r.clientFor(request.Directives{}, "HTTP/2")

		transport, ok := c.Transport.(*http2.Transport)
		require.True(t, ok)
		require.NotNil(t, transport.TLSClientConfig)
		assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	})

	t.Run("secure h2 transport verifies", func(t *testing.T) {
		r := &Runner{Client: base, Config: Config{}}
		c := r.clientFor(request.Directives{}, "HTTP/2")

		transport, ok := c.Transport.(*http2.Transport)
		require.True(t, ok)
		assert.Nil(t, transport.TLSClientConfig)
	})
}

func TestNewHTTPClient(t *testing.T) {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	t.Run("secure has no custom transport", func(t *testing.T) {
		c := newHTTPClient(jar, false, 30*time.Second)
		assert.Nil(t, c.Transport)
	})

	t.Run("insecure skips tls verification", func(t *testing.T) {
		c := newHTTPClient(jar, true, 30*time.Second)
		transport, ok := c.Transport.(*http.Transport)
		require.True(t, ok)
		require.NotNil(t, transport.TLSClientConfig)
		assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	})
}

func TestNewHTTPClientTimeout(t *testing.T) {
	c := newHTTPClient(nil, false, 5*time.Second)
	assert.Equal(t, 5*time.Second, c.Timeout)
}
