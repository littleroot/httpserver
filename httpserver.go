package main

import (
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
	fmt.Fprint(os.Stderr, "usage: httpserver <conf.toml>\n")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx := context.Background()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

// Conf is the configuration for the program.
// See conf.toml.example in the repository for details.
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
	defer f.Close()

	var c Conf
	if _, err := toml.DecodeReader(f, &c); err != nil {
		return fmt.Errorf("decode conf: %s", err)
	}

	var g errgroup.Group

	g.Go(func() error {
		log.Printf("listening on :80")
		return http.ListenAndServe(":80", httpHandler(c.Hosts))
	})

	g.Go(func() error {
		log.Printf("listening on :443")
		return http.ListenAndServeTLS(":443", c.CertFile, c.KeyFile, httpsHandler(c.Hosts))
	})

	return g.Wait()
}

func httpHandler(hosts map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no mapping exists; reject with a 503.
		if _, ok := hosts[r.Host]; !ok {
			http.Error(w, http.StatusText(503), 503)
			return
		}

		u := *r.URL
		u.Scheme = "https"
		u.Host = r.Host // explicitly copy the Host from the Request
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
}

func httpsHandler(hosts map[string]string) http.Handler {
	rev := &httputil.ReverseProxy{
		Director: multiHostDirector(hosts),
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no mapping exists; reject with a 503.
		if _, ok := hosts[r.Host]; !ok {
			http.Error(w, http.StatusText(503), 503)
			return
		}

		rev.ServeHTTP(w, r)
	})
}

func multiHostDirector(hosts map[string]string) func(r *http.Request) {
	return func(r *http.Request) {
		if localHost, ok := hosts[r.Host]; ok {
			r.URL.Scheme = "https"
			r.URL.Host = localHost
		} else {
			panic("unknown host " + r.Host) // should not be reached: indicates code bug
		}
	}
}
