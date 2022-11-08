# httpserver

## Install

```
go install github.com/littleroot/httpserver@latest
```

## Overview

The command `httpserver` runs a server that listens on ports 80 and 443 for
HTTP and HTTPS requests respectively.

The server redirects HTTP requests, except HTTP requests to the
`/.well-known/` paths, to their equivalent HTTPS URLs.

For HTTPS requests the server terminates TLS; then based on the incoming
request's Host header it forwards the request to a corresponding internal
server address. The mapping from incoming request hosts to internal server
addresses is configured in `conf.toml`.

If a request is received for a host not configured in `conf.toml`, or if the
internal server for a request is unreachable, the server responds with a 502.

## Usage

```
httpserver <conf.toml>
```

See `conf.toml.example` for a documented example config file.

## Test

```
go test -race
```
