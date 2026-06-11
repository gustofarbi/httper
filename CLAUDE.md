# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`httper` is a CLI runner for `.http` files (JetBrains HTTP Client format). It parses a `.http` file into one or more requests, optionally substitutes `{{placeholders}}` from a JetBrains-style `http-client.env.json`, sends them, and prints/saves the responses.

```bash
go build && ./httper [-save] [-env-file <env.json> -env <name>] [-v] <file.http>
```

## Commands

- Build: `go build`
- Run all fixtures: `make run-all` (builds, then runs every `testdata/*.http` — requires the echo server below)
- All tests: `make test` (`go test -v ./...`)
- Single test: `go test -run TestSplitRequests ./pkg/request/`
- Full QA gate: `make qa` → `sec fmt lint vet test run-all` (needs `gosec` and `golangci-lint` installed)

### Local TLS test server (`echo/`)

The `testdata/*.http` fixtures all target `https://localhost:8080`, served by the echo server. It is a **separate Go module** (`echo/go.mod`).

- One-time cert setup: `make mkcert` (needs [mkcert](https://github.com/FiloSottile/mkcert))
- Run it: `make echo-server`

`echo/internal/handler` has one handler per fixture scenario (Bearer, BasicAuth, JsonBody, FormData, Http2, Image, CatchAll).

## Architecture

Pipeline in `main.go` `run()`: read input file → optional env substitution → `request.Create` → loop `sendRequest` → `finalize.Response`.

- **`pkg/request`** — turns raw `.http` text into `[]*http.Request`. `Create` calls `splitRequests` (splits on `###`), then per request `splitRequest` separates the essentials line / headers / body, and `parse` builds the `*http.Request`. `parseBody` (`request.go` + `form.go`) handles `multipart/form-data`, including `< filename` file-include lines, and JSON bodies. The `wd` argument threaded through these functions is the directory file includes resolve against.
- **`pkg/env`** — `Parse` reads the JetBrains env JSON into `EnvironmentMap`; `Environment.Replace` substitutes `{{key}}` placeholders in the request text.
- **`pkg/finalize`** — `Response` reads the body once, optionally saves it (`save.go`), then prints status/duration/body (`print.go` pretty-prints JSON) and headers under `-v`. Saved responses go to `.idea/httpRequests/<timestamp>.<status>.<ext>`; extension is sniffed from the body via `mimetype`.

### os.Root sandboxing invariant

All filesystem access goes through Go 1.24's `os.Root` (`request.getFiles`, `env.Parse`, `finalize.saveResponse`, `main.run`). **Paths passed to `os.Root` methods must be relative to the root, and `..` traversal is rejected by design** — this is the intended security model, not a bug. When opening a file, open the root at its *directory* and pass the *base name* (see `main.run` and `loadEnv`). File includes are sandboxed to their `.http` file's directory.
