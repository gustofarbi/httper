# httper

[![CI](https://github.com/gustofarbi/httper/actions/workflows/ci.yml/badge.svg)](https://github.com/gustofarbi/httper/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/gustofarbi/httper/branch/master/graph/badge.svg)](https://codecov.io/gh/gustofarbi/httper)

A CLI runner for `.http` files (JetBrains HTTP Client format).

## Usage

```bash
httper [flags] <file.http>
```

| Flag | Description |
|------|-------------|
| `-env-file <path>` | JetBrains-style `http-client.env.json` |
| `-env <name>` | Environment to use from the env file |
| `-name <a,b>` | Run only the named requests (comma-separated) |
| `-save` | Save responses to `.idea/httpRequests/` |
| `-strict` | Treat non-2xx responses as failures |
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

### Variables

`{{placeholders}}` are resolved per request just before sending. Precedence
(highest first):

1. Request-local variables set in a pre-request script (`request.variables.set`)
2. `client.global` values set by handler scripts
3. In-file `@name = value` definitions
4. Env file values (`-env-file` + `-env`); a private sibling file
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
| `request.variables` | pre-request | `set(name, value)`, `get(name)` |
| `response` | response handlers | `status`, `body` (JSON bodies parsed into objects), `headers.valueOf(name)`, `headers.valuesOf(name)`, `contentType.mimeType`, `contentType.charset` |

`client.test` results feed the run report and exit code; `client.global.set`
in one request resolves `{{placeholders}}` in later ones (request chaining).

### Runs

- Cookie jar shared across all requests in one run (login → authenticated
  follow-up works out of the box); opt out per request with `# @no-cookie-jar`
- Test report summary with CI-friendly exit codes
- `-save` writes each response body to
  `.idea/httpRequests/<timestamp>.<status>.<ext>` under the working directory,
  with the extension sniffed from the body content
- Sandboxed file access: body file includes and `> file.js` handler scripts
  are restricted to the `.http` file's directory; saved responses to the
  working directory (no `..` traversal)

### Not supported

- gRPC, WebSocket, GraphQL execution (request lines parse, but nothing is sent)
- `>> file` response redirects (ignored with a warning)

## Installation

Prebuilt binaries for linux/macos/windows (amd64, arm64) are on the
[releases page](https://github.com/gustofarbi/httper/releases).

Or build from source:

```bash
go build
./httper -version
```
