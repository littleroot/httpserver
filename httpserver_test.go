package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

var noFollowRedirect = func(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

func mustToURLs(proxy map[string]string) map[string]url.URL {
	m, err := toURLs(proxy)
	if err != nil {
		panic(err)
	}
	return m
}

func TestHandler(t *testing.T) {
	proxy := map[string]string{
		"littleroot.org": "http://:" + getFreePort(),
		"foo.com":        "http://:" + getFreePort(),
		"sub.foo.com":    "http://:" + getFreePort(),
	}

	// Prepare local servers.
	for host, baseURL := range proxy {
		_, baseURL := host, baseURL // capture for closure in HTTP handler

		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			u := *r.URL
			u.Scheme = "http"
			u.Host = r.Host
			rw.Write([]byte(fmt.Sprintf("response from %s for incoming request %s", baseURL, u.String())))
		}))

		u, err := url.Parse(baseURL)
		if err != nil {
			t.Fatalf("bad baseURL %s: %s", baseURL, err)
		}

		ts.Config.Addr = u.Host

		// It is not sufficient to just replace s.Config.Addr, though the docs
		// seems to indicate so:
		//
		//     Config may be changed after calling NewUnstartedServer and before
		//     Start or StartTLS.
		//
		//     Addr optionally specifies the TCP address for the server to listen on [...].
		//
		// Changing the ts.Config.Addr at this point has no effect at all
		// (except for setting the value of ts.URL) when ts.Start() is called.
		// The ts.Listener would have already been set up to listen at
		// 127.0.0.1:0/:::1:0 by NewUnstartedServer(), and the new ts.Config.Addr
		// is not used. Worse still, the value of ts.URL would seem to indicate
		// that the test server is listening at the modified ts.Config.Addr,
		// when, in fact, it's not.
		//
		// TODO: File an issue with the Go project.
		l, err := net.Listen("tcp", u.Host)
		if err != nil {
			t.Errorf("failed to listen tcp on %s: %s", u.Host, err)
			return
		}
		ts.Listener = l

		ts.Start()
		defer ts.Close()
	}

	h80 := httpHandler(mustToURLs(proxy))
	h443 := httpsHandler(mustToURLs(proxy))

	// load certificate for hosts used in the test
	cert, err := tls.LoadX509KeyPair(filepath.Join("testdata", "cert.pem"), filepath.Join("testdata", "key.pem"))
	if err != nil {
		t.Errorf("failed to load certificate: %s", err)
		return
	}

	// create servers. used for the happy path test so that the test is
	// similar to the real world usage.
	s80 := httptest.NewServer(h80)
	defer s80.Close()
	s443 := httptest.NewUnstartedServer(h443)
	s443.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	s443.StartTLS()
	defer s443.Close()

	s443Client := s443.Client() // usable with both s80 and s443 servers
	// the requests should go to the test servers (not e.g. to
	// actual host of littleroot.org).
	//
	// only modify DialContext on the Transport. the field TLSClientConfig, in particular,
	// has to be preserved since it holds the root CA cert pool
	// for the self-signed certificates being used.
	s443Client.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		var s *httptest.Server
		if strings.HasSuffix(addr, ":443") {
			s = s443
		} else {
			s = s80
		}
		var d net.Dialer
		return d.DialContext(ctx, s.Listener.Addr().Network(), s.Listener.Addr().String())
	}

	t.Run("unknown request host", func(t *testing.T) {
		t.Run("http", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "http://unknown.org", nil)
			h80.ServeHTTP(w, r)
			if w.Code != 502 {
				t.Errorf("status code: want 502, got %d", w.Code)
				return
			}
		})

		t.Run("https", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "https://unknown.org", nil)
			h443.ServeHTTP(w, r)
			if w.Code != 502 {
				t.Errorf("status code: want 502, got %d", w.Code)
				return
			}
		})
	})

	t.Run("http -> https redirect", func(t *testing.T) {
		t.Run("basic", func(t *testing.T) {
			s443Client.CheckRedirect = noFollowRedirect
			defer func() {
				s443Client.CheckRedirect = nil // undo
			}()

			req, _ := http.NewRequest("GET", "http://littleroot.org", nil)
			rsp, _ := s443Client.Do(req)
			defer rsp.Body.Close()

			want := "https://littleroot.org/"
			got := rsp.Header.Get("location")

			if want != got {
				t.Errorf("location: want %q, got %q", want, got)
				return
			}
			if rsp.StatusCode != http.StatusFound {
				t.Errorf("status code: want %d, got %d", http.StatusFound, rsp.StatusCode)
				return
			}
		})

		t.Run("preserves remainder of url", func(t *testing.T) {
			s443Client.CheckRedirect = noFollowRedirect
			defer func() {
				s443Client.CheckRedirect = nil // undo
			}()

			req, _ := http.NewRequest("GET", "http://sub.foo.com/path/?key=val", nil)
			rsp, _ := s443Client.Do(req)
			defer rsp.Body.Close()

			want := "https://sub.foo.com/path/?key=val"
			got := rsp.Header.Get("location")

			if want != got {
				t.Errorf("location: want %q, got %q", want, got)
				return
			}
			if rsp.StatusCode != http.StatusFound {
				t.Errorf("status code: want %d, got %d", http.StatusFound, rsp.StatusCode)
				return
			}
		})
	})

	t.Run("happy path", func(t *testing.T) {
		for host, baseURL := range mustToURLs(proxy) {
			t.Run(host, func(t *testing.T) {
				// NOTE: http.Get follows redirects.
				reqPath := "/path/?key=val"
				reqURL := "http://" + host + reqPath

				rsp, err := s443Client.Get(reqURL)
				if err != nil {
					t.Errorf("want nil error, got %v", err)
					return
				}
				defer rsp.Body.Close()
				if rsp.StatusCode != 200 {
					t.Errorf("status code: want 200, got %d", rsp.StatusCode)
					return
				}
				b, err := io.ReadAll(rsp.Body)
				if err != nil {
					t.Errorf("unexpected error reading body: %s", err)
					return
				}

				got := string(b)
				want := fmt.Sprintf("response from %s for incoming request %s", baseURL.String(), reqURL)
				if got != want {
					t.Errorf("body: want %q, got %q", want, got)
					return
				}
			})
		}
	})
}

// https://github.com/facebookarchive/freeport/blob/master/freeport.go
func getFreePort() (port string) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	addr := listener.Addr().String()
	_, portString, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}

	return portString
}
