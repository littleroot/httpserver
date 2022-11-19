// Command httpserver implements a multiple-host HTTP and HTTPS reverse proxy.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/BurntSushi/toml"
	"golang.org/x/sync/errgroup"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, "usage: %s <conf.toml>\n", programName)
}

const programName = "httpserver"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix(programName + ": ")

	ctx := context.Background()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

// Conf is the configuration for the program.
// See conf.toml.example in the repository for details.
type Conf struct {
	CertFile  string
	KeyFile   string
	WellKnown string
	Hosts     map[string]string
}

func (c Conf) String() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "CertFile: %s\n", c.CertFile)
	fmt.Fprintf(&buf, "KeyFile: %s\n", c.KeyFile)
	fmt.Fprintf(&buf, "WellKnown: %s\n", c.WellKnown)
	for k, v := range c.Hosts {
		fmt.Fprintf(&buf, "Hosts: %s -> %s\n", k, v)
	}
	return buf.String()
}

func run(_ context.Context) error {
	flag.Usage = printUsage
	flag.Parse()

	if flag.NArg() != 1 {
		printUsage()
		os.Exit(2)
	}

	c, err := parseConf(flag.Arg(0))
	if err != nil {
		return fmt.Errorf("parse conf: %s", err)
	}
	log.Printf("using conf:")
	fmt.Fprintf(os.Stderr, "%s", c)

	var g errgroup.Group

	g.Go(func() error {
		mux := http.NewServeMux()
		mux.Handle("/", httpHandler(c.Hosts))
		if c.WellKnown != "" {
			mux.Handle("/.well-known/", http.StripPrefix("/.well-known/", http.FileServer(http.Dir(c.WellKnown))))
		}

		log.Printf("listening http on :80")
		return http.ListenAndServe(":80", mux)
	})

	g.Go(func() error {
		log.Printf("listening https on :443")
		return http.ListenAndServeTLS(":443", c.CertFile, c.KeyFile, httpsHandler(c.Hosts))
	})

	return g.Wait()
}

func httpHandler(hosts map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// if no mapping exists reject with a 502.
		if _, ok := hosts[r.Host]; !ok {
			http.Error(w, http.StatusText(502), 502)
			return
		}

		// redirect to https
		u := *r.URL
		u.Scheme = "https"
		// explicitly copy host from the request's Host header,
		// since the Host field of r.URL is typically empty.
		u.Host = r.Host
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
}

func httpsHandler(hosts map[string]string) http.Handler {
	revproxy := &httputil.ReverseProxy{
		Director: director(hosts),
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			log.Printf("proxy error: %v", err)
			http.Error(rw, http.StatusText(502), 502)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// if no mapping exists reject with a 502.
		if _, ok := hosts[r.Host]; !ok {
			http.Error(w, http.StatusText(502), 502)
			return
		}

		revproxy.ServeHTTP(w, r)
	})
}

// director returns a function that is suitable for use as the
// Director field of httputil.ReverseProxy. The hosts parameter is a map from
// known request hosts to internal server addresses. The returned function
// modifies the request such that a request to a known host is redirected to
// the appropriate internal server address, based on the hosts map.
//
// The returned function must be used only with a request whose Host exists in
// the hosts map. Otherwise the returned function panics.
func director(hosts map[string]string) func(r *http.Request) {
	return func(r *http.Request) {
		internalHost, ok := hosts[r.Host]
		if !ok {
			panic("unknown host " + r.Host)
		}

		r.URL.Scheme = "http"
		r.URL.Host = internalHost

		// copied from NewSingleHostReverseProxy.
		// https://golang.org/src/net/http/httputil/reverseproxy.go:
		if _, ok := r.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to default value
			r.Header.Set("User-Agent", "")
		}
	}
}

func parseConf(path string) (Conf, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Conf{}, err
	}

	var c Conf
	err = toml.Unmarshal(data, &c)
	return c, err
}
