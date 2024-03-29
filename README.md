# httpserver

## Install

Install the latest:

```
go install github.com/littleroot/httpserver@latest
```

The command's interface isn't stable yet. To avoid breaking changes, you
may want to use a specific commit:

```
go install github.com/littleroot/httpserver@<commit>
```

## Overview

The command `httpserver` runs a server that listens on ports 80 and 443 for
HTTP and HTTPS requests respectively.

The server redirects HTTP requests, except HTTP requests to the
`/.well-known/acme-challenge/` paths, to their equivalent HTTPS URLs. For HTTPS
requests the server terminates TLS; then based on the incoming request's Host
header it forwards the request to a corresponding destination server address.
The mapping from incoming request hosts to destination server addresses is
configured in `conf.json`.

If a request is received for a host not configured in `conf.json`, or if
the destination server for a request is unreachable, the server responds
with a 502.

The command can optionally manage TLS certificates for the specified domains
automatically. See the `certs.auto` field in the config. Certificate renewals
are attempted roughly 30 days before expiry.

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
	// proxy is a map from incoming host to the destination server
	// base URL for that host.
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
		auto: false, // see documentation above
		// certFile and keyFile specify paths to the certificate file
		// and the matching private key file for the domains handled
		// by this server. They should satisfy all the domains.
		certFile: string,
		keyFile: string
	},
	// acmeChallenge specifies an optional directory to serve over HTTP at
	// at the path /.well-known/acme-challenge/.
	acmeChallenge: string
}
```

## Test

```
go test -race
```
