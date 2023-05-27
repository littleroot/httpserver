// Command httpserver implements a multiple-host HTTP and HTTPS reverse proxy.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/sync/errgroup"
)

const renewBefore = 30 * 24 * time.Hour

func printUsage() {
	fmt.Fprintf(os.Stderr, "usage: %s <conf.json>\n", programName)
}

const programName = "httpserver"

func main() {
	log.SetFlags(0)

	ctx := context.Background()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func parseConf(path string) (Conf, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Conf{}, err
	}

	var c Conf
	err = json.Unmarshal(data, &c)
	return c, err
}

func checkConf(c Conf) error {
	if c.Certs.Auto && c.Certs.CertDir == "" {
		return errors.New("require certs.certDir when certs.auto == true")
	}
	if !c.Certs.Auto && c.Certs.CertFile == "" {
		return errors.New("require certs.certFile when certs.auto == false")
	}
	if !c.Certs.Auto && c.Certs.KeyFile == "" {
		return errors.New("require certs.keyFile when certs.auto == false")
	}
	if _, err := toURLs(c.Proxy); err != nil {
		return err
	}
	return nil
}

// Conf is the configuration for the program.
type Conf struct {
	Domains   []string          `json:"domains"`
	Proxy     map[string]string `json:"proxy"`
	Certs     Certs             `json:"certs"`
	WellKnown string            `json:"wellKnown"`
}

func toURLs(proxy map[string]string) (map[string]url.URL, error) {
	m := make(map[string]url.URL)
	for k, v := range proxy {
		u, err := url.Parse(v)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %s", v, err)
		}
		m[k] = *u
	}
	return m, nil
}

type Certs struct {
	Auto     bool   `json:"auto"`
	CertDir  string `json:"certDir"`
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
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
	if err := checkConf(c); err != nil {
		return fmt.Errorf("check conf: %s", err)
	}

	proxyURLs, err := toURLs(c.Proxy)
	if err != nil {
		// should be nil; should have been handled earlier in checkConf.
		panic(err)
	}

	var g errgroup.Group

	g.Go(func() error {
		mux := http.NewServeMux()
		mux.Handle("/", httpHandler(proxyURLs))
		if c.WellKnown != "" {
			mux.Handle("/.well-known/", http.StripPrefix("/.well-known/", http.FileServer(http.Dir(c.WellKnown))))
		}
		log.Printf("listening http on :80")
		return http.ListenAndServe(":80", mux)
	})

	g.Go(func() error {
		var cert, key string
		var s *http.Server

		if c.Certs.Auto {
			m := &autocert.Manager{
				Prompt:      autocert.AcceptTOS,
				Cache:       autocert.DirCache(c.Certs.CertDir),
				HostPolicy:  autocert.HostWhitelist(c.Domains...),
				RenewBefore: renewBefore,
			}
			s = &http.Server{
				Addr:      ":443",
				Handler:   httpsHandler(proxyURLs),
				TLSConfig: m.TLSConfig(),
			}
		} else {
			s = &http.Server{
				Addr:    ":443",
				Handler: httpsHandler(proxyURLs),
			}
			cert = c.Certs.CertFile
			key = c.Certs.KeyFile
		}

		log.Printf("listening https on %s", s.Addr)
		return s.ListenAndServeTLS(cert, key)
	})

	return g.Wait()
}

func httpHandler(proxy map[string]url.URL) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// if no mapping exists reject with a 502.
		if _, ok := proxy[r.Host]; !ok {
			http.Error(w, http.StatusText(502), 502)
			return
		}

		// redirect to https
		u := *r.URL
		u.Scheme = "https"
		// explicitly set Host on the URL, otherwise only Path and
		// RawQuery will be present.
		u.Host = r.Host
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
}

func httpsHandler(proxy map[string]url.URL) http.Handler {
	revproxy := &httputil.ReverseProxy{
		Rewrite: rewriter(proxy),
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			log.Printf("proxy error: %v", err)
			http.Error(rw, http.StatusText(502), 502)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// if no mapping exists reject with a 502.
		if _, ok := proxy[r.Host]; !ok {
			http.Error(w, http.StatusText(502), 502)
			return
		}
		revproxy.ServeHTTP(w, r)
	})
}

// rewriter returns a function that is suitable for use as the
// Rewriter field of httputil.ReverseProxy. The proxy parameter is a map from
// known request hosts to destination server base URLs. The returned function
// modifies the request such that a request to a known host is redirected to
// the appropriate destination server base URL, based on the proxy map.
//
// The returned function must be used only with a request whose Host exists in
// the proxy map. Otherwise the returned function panics.
func rewriter(proxy map[string]url.URL) func(*httputil.ProxyRequest) {
	return func(pr *httputil.ProxyRequest) {
		dest, ok := proxy[pr.In.Host]
		if !ok {
			panic("unknown host " + pr.In.Host)
		}
		pr.SetURL(&dest)
		pr.Out.Host = pr.In.Host
		pr.SetXForwarded()
	}
}
