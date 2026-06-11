// Package script runs JetBrains HTTP Client handler scripts ({% ... %}) on a
// goja JavaScript runtime. Each execution gets a fresh runtime with only the
// client/request/response objects registered — scripts have no filesystem,
// network, or process access. State persists between requests solely through
// vars.Globals.
package script

import (
	"crypto/hmac"
	"crypto/md5"  // #nosec G501 -- scripting helper, not used for security
	"crypto/sha1" // #nosec G505 -- scripting helper, not used for security
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"httper/pkg/vars"

	"github.com/dop251/goja"
)

const defaultTimeout = 5 * time.Second

// TestResult is one client.test outcome.
type TestResult struct {
	Name    string
	Failed  bool
	Message string
}

// Response is the view of an HTTP response handed to response handler
// scripts.
type Response struct {
	Status      int
	Headers     http.Header
	ContentType string
	Body        []byte
}

// Engine executes pre-request and response handler scripts.
type Engine struct {
	Globals *vars.Globals
	Out     io.Writer
	// Timeout is the wall-clock guard per script; zero means 5s.
	Timeout time.Duration
}

// PreRequest is the raw (unresolved) view of the request a pre-request
// script is about to influence. Raw values keep their {{placeholders}};
// Resolve backs the tryGetSubstituted views with the variables known at call
// time.
type PreRequest struct {
	Method      string
	URL         string
	Body        string
	Headers     [][2]string
	Environment map[string]string
	Resolve     func(string) string
}

func (p *PreRequest) resolve(s string) string {
	if p.Resolve == nil {
		return s
	}
	return p.Resolve(s)
}

// RunPre executes a pre-request script. Variables set via
// request.variables.set are delivered through setVar. req may be nil.
func (e *Engine) RunPre(code string, req *PreRequest, setVar func(key, value string)) error {
	vm := goja.New()

	local := make(map[string]string)
	variables := vm.NewObject()
	_ = variables.Set("set", func(name string, value goja.Value) {
		local[name] = value.String()
		setVar(name, value.String())
	})
	_ = variables.Set("get", func(name string) goja.Value {
		if v, ok := local[name]; ok {
			return vm.ToValue(v)
		}
		return goja.Undefined()
	})

	requestObj := vm.NewObject()
	_ = requestObj.Set("variables", variables)
	if req != nil {
		_ = requestObj.Set("method", func() string { return req.Method })
		_ = requestObj.Set("url", func() *goja.Object {
			return substitutable(vm, req.URL, req.resolve)
		})
		_ = requestObj.Set("body", func() *goja.Object {
			return substitutable(vm, req.Body, req.resolve)
		})
		_ = requestObj.Set("headers", preHeadersObject(vm, req))
		_ = requestObj.Set("environment", environmentObject(vm, req.Environment))
	}
	_ = vm.Set("request", requestObj)

	_ = vm.Set("client", e.clientObject(vm, nil))
	_ = vm.Set("crypto", cryptoObject(vm))

	return e.run(vm, code)
}

// substitutable wraps a raw string in the JetBrains getRaw /
// tryGetSubstituted pair.
func substitutable(vm *goja.Runtime, raw string, resolve func(string) string) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("getRaw", func() string { return raw })
	_ = obj.Set("tryGetSubstituted", func() string { return resolve(raw) })
	return obj
}

// preHeadersObject exposes request.headers.all() / findByName(name); header
// name matching is case-insensitive, misses return null.
func preHeadersObject(vm *goja.Runtime, req *PreRequest) *goja.Object {
	headerObj := func(name, value string) *goja.Object {
		obj := vm.NewObject()
		_ = obj.Set("name", func() string { return name })
		_ = obj.Set("getRawValue", func() string { return value })
		_ = obj.Set("tryGetSubstituted", func() string { return req.resolve(value) })
		return obj
	}

	headers := vm.NewObject()
	_ = headers.Set("all", func() []*goja.Object {
		all := make([]*goja.Object, len(req.Headers))
		for i, h := range req.Headers {
			all[i] = headerObj(h[0], h[1])
		}
		return all
	})
	_ = headers.Set("findByName", func(name string) goja.Value {
		for _, h := range req.Headers {
			if strings.EqualFold(h[0], name) {
				return vm.ToValue(headerObj(h[0], h[1]))
			}
		}
		return goja.Null()
	})

	return headers
}

// environmentObject exposes request.environment.get(name) over the selected
// env-file values; misses return null.
func environmentObject(vm *goja.Runtime, env map[string]string) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("get", func(name string) goja.Value {
		if v, ok := env[name]; ok {
			return vm.ToValue(v)
		}
		return goja.Null()
	})
	return obj
}

// RunPost executes a response handler script and returns its client.test
// results.
func (e *Engine) RunPost(code string, response *Response) ([]TestResult, error) {
	vm := goja.New()

	var results []TestResult
	_ = vm.Set("client", e.clientObject(vm, &results))
	_ = vm.Set("response", responseObject(vm, response))
	_ = vm.Set("crypto", cryptoObject(vm))

	err := e.run(vm, code)
	return results, err
}

// run executes code with the wall-clock interrupt guard.
func (e *Engine) run(vm *goja.Runtime, code string) error {
	timeout := e.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	timer := time.AfterFunc(timeout, func() { vm.Interrupt("script timeout") })
	defer timer.Stop()

	if _, err := vm.RunString(code); err != nil {
		return fmt.Errorf("running script: %w", err)
	}

	return nil
}

// clientObject builds the `client` API. results may be nil (pre-request
// scripts have no client.test).
func (e *Engine) clientObject(vm *goja.Runtime, results *[]TestResult) *goja.Object {
	client := vm.NewObject()

	_ = client.Set("test", func(name string, fn goja.Callable) {
		if results == nil {
			return
		}
		result := TestResult{Name: name}
		if _, err := fn(goja.Undefined()); err != nil {
			result.Failed = true
			result.Message = err.Error()
		}
		*results = append(*results, result)
	})

	_ = client.Set("assert", func(cond goja.Value, message goja.Value) {
		if cond.ToBoolean() {
			return
		}
		msg := "assertion failed"
		if message != nil && !goja.IsUndefined(message) {
			msg = message.String()
		}
		panic(vm.ToValue(msg))
	})

	_ = client.Set("log", func(args ...goja.Value) {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = arg.String()
		}
		_, _ = fmt.Fprintln(e.Out, strings.Join(parts, " "))
	})

	global := vm.NewObject()
	_ = global.Set("set", func(name string, value goja.Value) {
		e.Globals.Set(name, value.String())
	})
	_ = global.Set("get", func(name string) goja.Value {
		if v, ok := e.Globals.Get(name); ok {
			return vm.ToValue(v)
		}
		return goja.Undefined()
	})
	_ = client.Set("global", global)

	return client
}

// cryptoObject builds the `crypto` API: hex digests plus crypto.hmac.*.
// md5/sha1 are exposed as scripting conveniences (legacy APIs still demand
// them), not as security primitives.
func cryptoObject(vm *goja.Runtime) *goja.Object {
	hexSum := func(h func() hash.Hash) func(string) string {
		return func(data string) string {
			sum := h()
			_, _ = sum.Write([]byte(data))
			return hex.EncodeToString(sum.Sum(nil))
		}
	}
	hexHmac := func(h func() hash.Hash) func(string, string) string {
		return func(key, data string) string {
			mac := hmac.New(h, []byte(key))
			_, _ = mac.Write([]byte(data))
			return hex.EncodeToString(mac.Sum(nil))
		}
	}

	// #nosec G401,G505 -- exposed as scripting helpers, not used for security
	obj := vm.NewObject()
	_ = obj.Set("sha256", hexSum(sha256.New))
	_ = obj.Set("sha1", hexSum(sha1.New))
	_ = obj.Set("md5", hexSum(md5.New))

	hmacObj := vm.NewObject()
	_ = hmacObj.Set("sha256", hexHmac(sha256.New))
	_ = hmacObj.Set("sha1", hexHmac(sha1.New))
	_ = hmacObj.Set("md5", hexHmac(md5.New))
	_ = obj.Set("hmac", hmacObj)

	return obj
}

// responseObject exposes status/body/headers/contentType. JSON bodies are
// unmarshalled so scripts can navigate them natively.
func responseObject(vm *goja.Runtime, response *Response) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("status", response.Status)
	_ = obj.Set("body", bodyValue(response))

	headers := vm.NewObject()
	_ = headers.Set("valueOf", func(name string) goja.Value {
		if v := response.Headers.Get(name); v != "" {
			return vm.ToValue(v)
		}
		return goja.Null()
	})
	_ = headers.Set("valuesOf", func(name string) []string {
		return response.Headers.Values(name)
	})
	_ = obj.Set("headers", headers)

	mimeType, params, err := mime.ParseMediaType(response.ContentType)
	if err != nil {
		mimeType = response.ContentType
	}
	contentType := vm.NewObject()
	_ = contentType.Set("mimeType", mimeType)
	_ = contentType.Set("charset", params["charset"])
	_ = obj.Set("contentType", contentType)

	return obj
}

func bodyValue(response *Response) interface{} {
	if strings.Contains(response.ContentType, "application/json") {
		var parsed interface{}
		if err := json.Unmarshal(response.Body, &parsed); err == nil {
			return parsed
		}
	}

	return string(response.Body)
}
