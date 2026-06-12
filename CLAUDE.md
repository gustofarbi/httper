# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`httper` is a CLI runner for `.http` files (JetBrains HTTP Client format). It parses a `.http` file into request templates, resolves `{{placeholders}}` per request just before sending (env file, in-file `@vars`, `client.global`, dynamic `$vars`), runs pre-request/response handler scripts on an embedded JS engine (goja), sends the requests, and prints/saves the responses plus a test report.

```bash
go build && ./httper [-save] [-env-file <env.json> -env <name>] [-name <a,b>] [-strict] [-v] <file.http>
```

Exit codes: `0` success, `1` usage/parse/IO error, `2` failing `client.test`s or send errors (with `-strict`, also non-2xx).

## Commands

- Build: `go build`
- All tests: `make test` (`go test -v ./...`)
- Single test: `go test -run TestParseFile ./pkg/request/`
- Coverage gate: `make cover` (`-coverpkg=./...`, fails under `COVERAGE_THRESHOLD`, default 60)
- Full QA gate: `make qa` → `sec fmt lint vet cover` (needs `gosec` and `golangci-lint` installed)
- Run all fixtures manually: `make run-all` (builds, then runs every `testdata/*.http` — requires the echo server below; not part of `qa`, since `e2e_test.go` covers the same handlers in-process)

### Local TLS test server

The `testdata/*.http` fixtures all target `https://localhost:8080`, served by the echo server (`cmd/echo`, run from the repo root; certs live in `echo/certs`).

- One-time cert setup: `make mkcert` (needs [mkcert](https://github.com/FiloSottile/mkcert))
- Run it: `make echo-server`

`internal/echo/handler` has one handler per fixture scenario (Bearer, BasicAuth, JsonBody, FormData, Http2, Image, Token, Redirect, SetCookie/NeedCookie, CatchAll) plus `NewMux()` wiring the routes. `NewMux()` is the single source of truth shared by `cmd/echo` and the in-process end-to-end tests (`e2e_test.go`), so `make run-all` (manual server) and `go test` exercise the same handlers.

`cmd/echo` also serves a plaintext gRPC echo service on `:8081` (`internal/grpcecho`, reflection enabled — Echo/Fail/Countdown, one method per `testdata/grpc.http` scenario). `grpcecho.New()` is shared with the e2e tests the same way `NewMux()` is. Regenerate its protobuf code with `make proto` (runs buf via `go run`; needs `protoc-gen-go`/`protoc-gen-go-grpc` on PATH).

## Architecture

Pipeline in `main.go` `run()`: read input file → `request.ParseFile` → `filterTemplates` (`-name`) → per template in `executeTemplates` (`execute.go`): pre-request script → `vars.Store.Resolve` → `Template.Build` → `Runner.Send` → response handler script → collect `Result`s → `buildReport`/`printReport` (`report.go`) → exit code. `GRPC` templates branch to `GRPCRunner.Send` (`grpcrunner.go`) instead of Build/Send; everything around them (scripts, vars, report) is shared, with `Result.GRPC` marking that `StatusCode` is a gRPC code (0 = OK, which is what `-strict` checks).

- **`pkg/request`** — `ParseFile` scans `.http` text line by line into a `File` (in-file `@vars` + `[]*Template`). A `Template` keeps raw essentials/headers/body **with placeholders intact** plus `Name`, `Directives` (`@no-redirect`, `@timeout`, `@no-cookie-jar`, `@no-log`), and `PreScript`/`PostScript` (`< {% %}`, `> {% %}`, `> file.js`). `Template.Build(resolve, wd)` substitutes and constructs the `*http.Request` at send time — this lazy split is what makes request chaining possible. `parseBody` (`request.go` + `form.go`) handles JSON, `multipart/form-data` (with `< filename` includes), and `x-www-form-urlencoded`.
- **`pkg/vars`** — layered `Store`; precedence: request-local (pre-script) > `Globals` (`client.global`) > in-file `@vars` > env file. `$`-prefixed keys are dynamic (`$uuid`, `$timestamp`, `$isoTimestamp`, `$randomInt`); `Now`/`Rand` are injectable for deterministic tests. Unknown keys stay verbatim.
- **`pkg/script`** — goja engine. Fresh runtime per script, only `client`/`request`/`response` registered (no fs/net), wall-clock interrupt guard (default 5s). `client.test` results feed the run report; `client.global` writes through to `vars.Globals`.
- **`pkg/env`** — `Parse` reads the JetBrains env JSON into `EnvironmentMap` (values feed `vars.Store`).
- **`pkg/grpcrun`** — gRPC execution: `ParseTarget` (scheme/TLS/port rules: `grpc://` plaintext, `grpcs://` TLS, bare host TLS except loopback) and `Invoke` (server-reflection schema via `grpcreflect`, JSON↔protobuf via `dynamicpb`/`protojson`, unary + server-streaming; non-OK statuses are Outcomes, not errors — only dial/reflect/marshal failures error).
- **`pkg/finalize`** — `Response` takes the pre-read body, optionally saves it (`save.go`), then prints status/duration/body (`print.go` pretty-prints JSON); headers under `-v`, status line only under `@no-log` (`Options.Quiet`). Saved responses go to `.idea/httpRequests/<timestamp>.<status>.<ext>`; extension is sniffed from the body via `mimetype`.
- **`runner.go`** — `Runner.Send(template, req)` returns a `Result` (status, duration, headers, body, tests, err). `clientFor` makes a per-request shallow client copy to apply directives and the h2 transport without mutating the shared client. Cookie jar is on by default (set in `main.run`).

## Conventions

- TDD: failing test first, then implementation (`make test` must stay green per commit).
- The e2e harness (`runResults`/`runContent` in `e2e_test.go`) mirrors `main.run`'s wiring; when changing the pipeline, update both.

### os.Root sandboxing invariant

All filesystem access goes through Go 1.24's `os.Root` (`request.getFiles`, `env.Parse`, `finalize.saveResponse`, `main.run`, handler-script loading). **Paths passed to `os.Root` methods must be relative to the root, and `..` traversal is rejected by design** — this is the intended security model, not a bug. When opening a file, open the root at its *directory* and pass the *base name* (see `main.run` and `loadEnv`). File includes and `> file.js` handler scripts are sandboxed to their `.http` file's directory.
