# httper

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
| `-v` | Verbose output (response headers, PASS lines) |

### Exit codes

- `0` — all requests sent, all tests passed
- `1` — usage, parse, or I/O error
- `2` — failing `client.test` assertions or request send errors (with `-strict`, also any non-2xx response)

## Supported features

- HTTP methods, headers, JSON / multipart (`< file` includes) / `application/x-www-form-urlencoded` bodies
- HTTP/2 and HTTP/2 (Prior Knowledge)
- Bearer and Basic authentication
- `{{placeholders}}` from env files, in-file `@variables`, and dynamic `{{$uuid}}`, `{{$timestamp}}`, `{{$isoTimestamp}}`, `{{$randomInt}}`
- Request naming via `# @name` or `### title`, selection via `-name`
- Per-request directives: `# @no-redirect`, `# @timeout <seconds>`, `# @no-cookie-jar`, `# @no-log`
- Pre-request scripts `< {% ... %}` and response handlers `> {% ... %}` / `> file.js` (JavaScript via goja): `client.test`, `client.assert`, `client.log`, `client.global`, `response.status/body/headers/contentType`, `request.variables`
- Request chaining: `client.global.set(...)` in one request resolves `{{placeholders}}` in later ones
- Cookie jar across requests in one run (opt out per request with `# @no-cookie-jar`)
- Test report summary with CI-friendly exit codes

Not supported: gRPC, WebSocket, GraphQL execution, `>> file` response redirects.

## Installation

```bash
go install github.com/matej-karolcik/httper@latest
```
