package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"github.com/BurntSushi/toml"
	"golang.org/x/sync/errgroup"
)

func printUsage() {
	fmt.Fprint(os.Stderr, "usage: httpserver <conf.toml>\n")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx := context.Background()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

type Conf struct {
	CertFile string
	KeyFile  string
	Hosts    map[string]string
}

func run(ctx context.Context) error {
	flag.Parse()

	if flag.NArg() != 1 {
		printUsage()
		os.Exit(2)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	var c Conf
	if _, err := toml.DecodeReader(f, &c); err != nil {
		return fmt.Errorf("decode conf: %s", err)
	}

	http.HandleFunc("/503", temporarilyUnavailable)

	revProxy := httputil.ReverseProxy{
		Director: multiHostDirector(c.Hosts),
	}

	var g errgroup.Group

	g.Go(func() error {
		log.Printf("listening on :80")
		return http.ListenAndServe(":80", redirectHTTPS(http.DefaultServeMux))
	})

	g.Go(func() error {
		log.Printf("listening on :443")
		return http.ListenAndServeTLS(":443", c.CertFile, c.KeyFile, &revProxy)
	})

	return g.Wait()
}

func redirectHTTPS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/503" {
			h.ServeHTTP(w, r)
			return
		}
		u := *r.URL
		u.Scheme = "https"
		u.Host = r.Host
		http.Redirect(w, r, u.String(), http.StatusFound)
	})

}

func multiHostDirector(hosts map[string]string) func(r *http.Request) {
	return func(r *http.Request) {
		var target url.URL

		if v, ok := hosts[r.Host]; ok {
			target = url.URL{
				Scheme:   "http",
				Host:     v,
				Path:     r.URL.Path,
				RawQuery: r.URL.RawQuery,
			}
		} else {
			target = *r.URL
			target.Scheme = "http"
			target.Host = r.Host
			target.Path = "/503"
		}

		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.URL.Path = target.Path
		r.URL.RawQuery = target.RawQuery
	}
}

func temporarilyUnavailable(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(503), 503)
}
