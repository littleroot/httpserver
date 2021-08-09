# httpserver

```
go get github.com/littleroot/httpserver
```

The program runs a Go server that listens for HTTP (port 80) and HTTPS (port
443) requests. The server redirects any non-HTTPS requests to HTTPS, and it
terminates TLS. Based on the incoming request's Host, the server proxies the
incoming request to its appropriate local server address. The mapping from
incoming request hosts to local server addresses is configured in `conf.toml`.

If a request for a host that is not configured in `conf.toml` is received,
the server responds with a 503 over HTTP.

## Usage

```
httpserver <conf.toml>
```

See `conf.toml.example` for an example config file.

## Test

```
go test -race
```
