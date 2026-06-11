# httper

[![CI](https://github.com/gustofarbi/httper/actions/workflows/ci.yml/badge.svg)](https://github.com/gustofarbi/httper/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/gustofarbi/httper/branch/master/graph/badge.svg)](https://codecov.io/gh/gustofarbi/httper)

This is a cli runner for .http files.

## Usage

```bash
httper <file.http>
```

## Installation

Prebuilt binaries for linux/macos/windows (amd64, arm64) are on the
[releases page](https://github.com/gustofarbi/httper/releases).

Or build from source:

```bash
go build
./httper -version
```

## Supported features

- [x] Basic HTTP methods
- [x] Basic authentication
- [x] Basic request headers
- [x] Basic request body
- [x] Basic response body
- [x] Basic response status
- [x] Basic response headers
- [x] Basic response time
- [x] .env file support
- [ ] Grpc support
- [ ] Graphql support
- [ ] Websocket support
