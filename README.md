# httpserver

## Install

Install the latest:

```
go install github.com/littleroot/httpserver@latest
```

The command's interface isn't stable yet. To avoid breaking changes, you
may want to use a specific commit. For example:

```
go install github.com/littleroot/httpserver@0fc181a
```

## Overview

The command `httpserver` runs a server that listens on ports 80 and 443 for
HTTP and HTTPS requests respectively.

The server redirects HTTP requests, except HTTP requests to the
`/.well-known/` paths, to their equivalent HTTPS URLs.

For HTTPS requests the server terminates TLS; then based on the incoming
request's Host header it forwards the request to a corresponding internal
server address. The mapping from incoming request hosts to internal server
addresses is configured in `conf.json`.

If a request is received for a host not configured in `conf.json`, or if
the internal server for a request is unreachable, the server responds
with a 502.

The command can optionally manage TLS certificates for the specified domains
automatically. See the `certs.auto` field in the config. Certificate renewals
are attempted 30 days before expiry.

## Usage

```
httpserver <conf.json>
```

## Config

See `conf.json.example` for an example.

The config file must contain a JSON object with the following structure.

```ts
{

	// domains is the set of domains served by the command.
	domains: [string],
	// proxy is a map from incoming host to the internal server
	// address serving that host.
	proxy: { [string]: string },
	// certs specifies details for TLS certificate.
	certs: {
		// auto specifies whether the command should automatically create
		// and renew TLS certificates for the domains via Let's Encrypt.
		auto: true,
		// certDir is the path to store automatically created
		// certificates and keyfiles.
		certDir: string
	} | {
		// auto specifies whether the command should automatically create
		// and renew TLS certificates for the domains via Let's Encrypt.
		auto: false,
		// certFile and keyFile specify paths to the certificate file
		// and the matching private key file for the domains handled
		// by this server. They should satisfy all the domains.
		certFile: string,
		keyFile: string
	},
	// wellKnown specifies an optional directory to serve over HTTP at
	// at the URL /.well-known/. Typically one of its subdirectories contains
	// ACME challenges to serve; this tends to be useful when manually
	// creating certificates.
	wellKnown: string
}
```

## Test

```
go test -race
```
