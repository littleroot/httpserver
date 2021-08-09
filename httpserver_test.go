package main

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	hosts := map[string]string{
		"littleroot.org":           ":" + getFreePort(),
		"passwords.littleroot.org": ":" + getFreePort(),
		"birthdays.littleroot.org": ":" + getFreePort(),
	}

	// Prepare local servers.
	for host, addr := range hosts {
		host, addr := host, addr // capture for closure in HTTP handler
		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte("response from " + addr + " for " + host))
		}))

		ts.Config.Addr = addr
		// It is not sufficient to just replace s.Config.Addr, though the Go doc
		// seems to indicate so:
		//
		//     Config may be changed after calling NewUnstartedServer and before
		//     Start or StartTLS.
		//
		// Changing the ts.Config.Addr at this point has no effect at all
		// (except for setting the value of ts.URL) when ts.Start() is called.
		// The ts.Listener would have already been set up to listen at
		// 127.0.0.1:0/:::1:0 by NewUnstartedServer(), and the new ts.Config.Addr
		// is not used. Worse still, the value of ts.URL would indicate
		// that the test server is listening at the modified ts.Config.Addr,
		// when, in fact, it's not.
		//
		// File an issue with the Go project.
		l, err := net.Listen("tcp", addr)
		if err != nil {
			t.Errorf("failed to listen on %s: %s", addr, err)
			return
		}
		ts.Listener = l

		ts.Start()
		defer ts.Close()
	}

	h80 := httpHandler(hosts)
	h443 := httpsHandler(hosts)

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

	t.Run("unknown request host", func(t *testing.T) {
		t.Run("http", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "http://unknown.org", nil)
			h80.ServeHTTP(w, r)
			if w.Code != 503 {
				t.Errorf("status code: want 503, got %d", w.Code)
				return
			}
		})

		t.Run("https", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "https://unknown.org", nil)
			h443.ServeHTTP(w, r)
			if w.Code != 503 {
				t.Errorf("status code: want 503, got %d", w.Code)
				return
			}
		})
	})

	t.Run("http -> https redirect", func(t *testing.T) {
		t.Run("basic", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "http://littleroot.org", nil)
			h80.ServeHTTP(w, r)

			want := "https://littleroot.org"
			got := w.Header().Get("location")

			if want != got {
				t.Errorf("location: want %q, got %q", want, got)
				return
			}

			if w.Code != http.StatusFound {
				t.Errorf("status code: want %d, got %d", http.StatusFound, w.Code)
				return
			}
		})

		t.Run("preserves remainder of url", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "http://user:pass@littleroot.org/path/?key=val#frag", nil)
			h80.ServeHTTP(w, r)

			want := "https://user:pass@littleroot.org/path/?key=val#frag"
			got := w.Header().Get("location")

			if want != got {
				t.Errorf("location: want %q, got %q", want, got)
				return
			}

			if w.Code != http.StatusFound {
				t.Errorf("status code: want %d, got %d", http.StatusFound, w.Code)
				return
			}
		})
	})

	t.Run("happy path", func(t *testing.T) {
		c := s443.Client()

		// the requests should go to the test servers (not e.g. to
		// actual host of littleroot.org).
		//
		// only modify DialContext on the Transport. the field TLSClientConfig, in particular,
		// has to be preserved since it holds the root CA cert pool
		// for the self-signed certificates being used.
		c.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var s *httptest.Server
			if strings.HasSuffix(addr, ":443") {
				s = s443
			} else {
				s = s80
			}
			var d net.Dialer
			return d.DialContext(ctx, s.Listener.Addr().Network(), s.Listener.Addr().String())
		}

		for host, localAddr := range hosts {
			t.Run(host, func(t *testing.T) {
				// NOTE: c.Get() follows redirects.
				rsp, err := c.Get("http://user:pass@" + host + "/path/?key=val#frag")
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
				want := "response from " + localAddr + " for " + host
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
