# httpserver

```
go install github.com/littleroot/httpserver@latest
```

The program runs a Go server that listens for HTTP (port 80) and HTTPS (port
443) requests. The server redirects all HTTP requests (except those to
`/.well-known/`) to HTTPS. For HTTPS requests the server terminates TLS, and
based on the incoming request's Host, forwards the incoming request to an
appropriate internal server address. The mapping from incoming request hosts
to internal server addresses is configured in `conf.toml`.

If a request is received for a host not configured in `conf.toml`,
the server responds with a 502.

## Usage

```
httpserver <conf.toml>
```

See `conf.toml.example` for an example config file.

## Test

```
go test -race
```
