// Package script runs JetBrains HTTP Client handler scripts ({% ... %}) on a
// goja JavaScript runtime. Each execution gets a fresh runtime with only the
// client/request/response objects registered — scripts have no filesystem,
// network, or process access. State persists between requests solely through
// vars.Globals.
package script

import (
	"encoding/json"
	"fmt"
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

// RunPre executes a pre-request script. Variables set via
// request.variables.set are delivered through setVar.
func (e *Engine) RunPre(code string, setVar func(key, value string)) error {
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
	_ = vm.Set("request", requestObj)

	_ = vm.Set("client", e.clientObject(vm, nil))

	return e.run(vm, code)
}

// RunPost executes a response handler script and returns its client.test
// results.
func (e *Engine) RunPost(code string, response *Response) ([]TestResult, error) {
	vm := goja.New()

	var results []TestResult
	_ = vm.Set("client", e.clientObject(vm, &results))
	_ = vm.Set("response", responseObject(vm, response))

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
