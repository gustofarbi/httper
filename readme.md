# httper

[![CI](https://github.com/gustofarbi/httper/actions/workflows/ci.yml/badge.svg)](https://github.com/gustofarbi/httper/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/gustofarbi/httper/branch/master/graph/badge.svg)](https://codecov.io/gh/gustofarbi/httper)

A CLI runner for `.http` files (JetBrains HTTP Client format).

## Usage

```bash
httper [flags] <file.http> [more.http ...]
```

Multiple files (or shell/quoted globs like `'requests/*.http'`) run in one
invocation with an aggregated report and exit code. Files are isolated —
fresh cookie jar and `client.global` state per file — so results never depend
on argument order.

| Flag | Description |
|------|-------------|
| `-env-file <path>` | JetBrains-style `http-client.env.json` |
| `-env <name>` | Environment to use from the env file |
| `-name <a,b>` | Run only the named requests (comma-separated) |
| `-save` | Save responses to `.idea/httpRequests/` |
| `-strict` | Treat non-2xx responses as failures |
| `-insecure` | Skip TLS certificate verification (self-signed certs) |
| `-timeout <seconds>` | Request timeout, default 30 (`# @timeout` wins per request) |
| `-var key=value` | Set a variable (repeatable; overrides `@vars` and env file) |
| `-report-junit <path>` | Write a JUnit XML report (CI test integration) |
| `-report-json <path>` | Write a JSON report |
| `-vegeta` | Run `# @vegeta`-marked requests as load tests |
| `-v` | Verbose output (response headers, PASS lines, debug logs) |
| `-version` | Print version and exit |

### Exit codes

- `0` — all requests sent, all tests passed
- `1` — usage, parse, or I/O error
- `2` — failing `client.test` assertions or request send errors (with `-strict`, also any non-2xx response)

## File format

Requests are separated by `###` lines; text after the hashes becomes the
request title. Comments use `#` or `//`.

```http
### Login
# @name login
POST https://example.com/api/login HTTP/2
Content-Type: application/json

{"user": "admin", "password": "{{password}}"}

> {%
    client.test("logged in", function () {
        client.assert(response.status === 200, "expected 200");
    });
    client.global.set("token", response.body.token);
%}

### Profile
GET https://example.com/api/profile
    ?fields=name,email
Authorization: Bearer {{token}}
```

- The request line is `[METHOD] URL [PROTO]`; method defaults to `GET`,
  protocol may be `HTTP/1.1`, `HTTP/2`, or `HTTP/2 (Prior Knowledge)`.
- Lines indented with four spaces continue the previous line (multi-line
  URLs/query strings and header values).
- Headers run until the first blank line; everything after is the body.

## Supported features

### Requests

- All standard HTTP methods, headers, and bodies: JSON,
  `multipart/form-data` (with `< filename` file includes),
  `application/x-www-form-urlencoded` (pairs may span multiple lines), and
  any other content type sent verbatim (`text/plain`, XML, …)
- Whole-body file include: a body of just `< ./payload.json` sends that
  file's contents (any content type; path sandboxed to the `.http` file's
  directory, placeholders in the path resolve first)
- HTTP/2 and HTTP/2 (Prior Knowledge)
- Bearer auth (header passthrough) and Basic auth
  (`Authorization: Basic user password` is base64-encoded automatically)
- Request naming via `# @name` or `### title` (unnamed requests get `#1`,
  `#2`, …), selection via `-name`
- Default request timeout of 30s, overridable per request with `# @timeout`

### Per-request directives

| Directive | Effect |
|-----------|--------|
| `# @name <name>` | Name the request |
| `# @no-redirect` | Don't follow redirects |
| `# @timeout <seconds>` | Override the 30s default timeout |
| `# @no-cookie-jar` | Opt this request out of the shared cookie jar |
| `# @no-log` | Print only the status line for this response |
| `# @vegeta [params]` | Declare a load profile for this request (see Load testing) |

### Variables

`{{placeholders}}` are resolved per request just before sending. Precedence
(highest first):

1. Request-local variables set in a pre-request script (`request.variables.set`)
2. `client.global` values set by handler scripts
3. `-var key=value` CLI flags (above file-declared values, below runtime ones
   so chaining keeps working)
4. In-file `@name = value` definitions
5. Env file values (`-env-file` + `-env`); a private sibling file
   (`http-client.private.env.json` next to `http-client.env.json`) overlays
   the public one key-wise — keep secrets there, out of version control

Unknown placeholders stay verbatim. Dynamic variables are computed fresh on
each use:

| Variable | Value |
|----------|-------|
| `{{$uuid}}` / `{{$random.uuid}}` | Random UUID v4 |
| `{{$timestamp}}` | Unix timestamp (seconds) |
| `{{$isoTimestamp}}` | ISO-8601 / RFC 3339 UTC timestamp |
| `{{$randomInt}}` | Random integer 0–1000 |
| `{{$random.integer(from, to)}}` | Random integer, `from` inclusive to `to` exclusive |
| `{{$random.alphabetic(length)}}` | Random letters |
| `{{$random.email}}` | Random email address |
| `{{$env.NAME}}` | OS environment variable (empty + warning when unset) |

### Scripts

Pre-request scripts (`< {% ... %}`) and response handlers (`> {% ... %}`
inline, or `> path/to/file.js`) run on an embedded JavaScript engine
([goja](https://github.com/dop251/goja)). Each script gets a fresh, sandboxed
runtime — no filesystem, network, or process access — with a 5s wall-clock
timeout.

Available API:

| Object | Available in | Members |
|--------|--------------|---------|
| `client` | both | `test(name, fn)` (response handlers only), `assert(cond, message)`, `log(...args)`, `global.set(name, value)`, `global.get(name)` |
| `crypto` | both | `sha256(s)`, `sha1(s)`, `md5(s)`, `hmac.sha256(key, data)` / `.sha1` / `.md5` — hex strings |
| `request` | pre-request | `variables.set/get`, `method()`, `url()` / `body()` (each with `getRaw()` and `tryGetSubstituted()`), `headers.all()` / `headers.findByName(name)` (header objects with `name()`, `getRawValue()`, `tryGetSubstituted()`), `environment.get(name)` (env-file values) |
| `response` | response handlers | `status`, `body` (JSON bodies parsed into objects), `headers.valueOf(name)`, `headers.valuesOf(name)`, `contentType.mimeType`, `contentType.charset` |

Pre-request scripts see the request *as written* (placeholders intact) via
`getRaw()`; `tryGetSubstituted()` resolves with the variables known at call
time.

`client.test` results feed the run report and exit code; `client.global.set`
in one request resolves `{{placeholders}}` in later ones (request chaining).

### gRPC

`GRPC` request lines execute real gRPC calls (unary and server-streaming):

```http
### say-hello
GRPC grpc://localhost:8081/helloworld.Greeter/SayHello
X-Token: secret

{"name": "world"}

> {%
client.test("greets", function() {
  client.assert(response.status === 0, "expected OK");
});
%}
```

- Target syntax: `[grpc://|grpcs://]host[:port]/package.Service/Method`.
  `grpc://` forces plaintext, `grpcs://` TLS; a bare host defaults to TLS
  except loopback hosts (`localhost`, `127.0.0.1`, `::1`). A missing port
  defaults to 443 (TLS) / 80 (plaintext). `-insecure` skips TLS verification.
- The message schema is resolved via [server
  reflection](https://grpc.io/docs/guides/reflection/) — the server must have
  it enabled; no `.proto` files are read.
- The JSON body becomes the request message; headers become metadata verbatim
  (no Basic-auth encoding; HTTP-only names like `Content-Type` and reserved
  `grpc-*` names are dropped).
- `response.status` is the gRPC status code (`0` = OK), `response.body` the
  response message as JSON (an array of messages for server streams), and
  `response.headers` the merged header + trailer metadata. `-strict` treats
  any non-OK status as a failure.
- `# @timeout` caps the whole call including reflection; `# @no-log` and
  `-save` work as for HTTP.
- Not supported: client/bidirectional streaming and `< file` body includes.

### Load testing (vegeta)

With the `-vegeta` flag, requests marked `# @vegeta` are attacked via the
[vegeta](https://github.com/tsenart/vegeta) library instead of being sent
once; without the flag they run as normal single requests, so a file can be
exercised in CI and load-tested with the same content.

```http
### login runs once; its token feeds the attack
# @name login
POST https://example.com/api/login
Content-Type: application/json

{"user": "admin", "password": "{{password}}"}

> {% client.global.set("token", response.body.token); %}

### attacked at 100 req/s for 30s
# @vegeta rate=100/s duration=30s
GET https://example.com/api/profile
Authorization: Bearer {{token}}
```

- Params (all optional): `rate=N/s|N/m` (default `50/s`), `duration` (Go
  duration, default `10s`), `workers`, `max-workers`, `connections`,
  `max-body` (response bytes read per shot). Invalid values are warned about
  and fall back to the defaults.
- The pre-request script runs once and placeholders resolve once; the frozen
  request is attacked. Response handler scripts and `client.test` are skipped
  (there is no single response).
- Output is vegeta's text report (latencies p50/p95/p99, success ratio,
  status code histogram); `# @no-log` prints a one-line summary instead.
- Any failing shot (non-2xx or transport error) marks the request as errored
  → exit code 2. `-strict` adds nothing for attacked requests.
- `# @timeout` and `# @no-redirect` apply to every shot; `-insecure` works as
  for HTTP. Requests run through the shared HTTP client's cookie jar only for
  normal sends — the attacker has no jar.
- `GRPC` requests cannot be attacked.

### Runs

- Cookie jar shared across all requests in one run (login → authenticated
  follow-up works out of the box); opt out per request with `# @no-cookie-jar`
- Test report summary with CI-friendly exit codes; `-report-junit` /
  `-report-json` write machine-readable reports (one testcase per
  `client.test`, send errors as `<error>` entries)
- `-save` writes each response body to
  `.idea/httpRequests/<timestamp>.<status>.<ext>` under the working directory,
  with the extension sniffed from the body content
- Sandboxed file access: body file includes and `> file.js` handler scripts
  are restricted to the `.http` file's directory; saved responses to the
  working directory (no `..` traversal)

### Not supported

- WebSocket and GraphQL execution (request lines parse, but nothing is sent)
- gRPC client/bidirectional streaming
- `>> file` response redirects (ignored with a warning)

## Installation

Prebuilt binaries for linux/macos/windows (amd64, arm64) are on the
[releases page](https://github.com/gustofarbi/httper/releases).

Or install with Go:

```bash
go install github.com/gustofarbi/httper@latest
```

Or build from source:

```bash
go build
./httper -version
```
