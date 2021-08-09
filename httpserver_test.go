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
		"littleroot.org":           "localhost:8000",
		"passwords.littleroot.org": "localhost:52849",
		"birthdays.littleroot.org": "localhost:6000",
	}

	// Prepare local servers.
	for host, addr := range hosts {
		host := host // capture for closure in HTTP handler
		s := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte(host))
		}))
		s.Config.Addr = addr
		s.Start()
		defer s.Close()
	}

	h80 := httpHandler(hosts)
	h443 := httpsHandler(hosts)

	cert, err := tls.LoadX509KeyPair(filepath.Join("testdata", "cert.pem"), filepath.Join("testdata", "key.pem"))
	if err != nil {
		t.Errorf("failed to load certificate: %s", err)
		return
	}

	// No need server for http, where we can invoke the handler directly.
	// For https the server is necessary to obtain a client that makes
	// proper TLS requests.
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
				t.Errorf("location: want: %s, got: %s", want, got)
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
				t.Errorf("location: want: %s, got: %s", want, got)
				return
			}

			if w.Code != http.StatusFound {
				t.Errorf("status code: want %d, got %d", http.StatusFound, w.Code)
				return
			}
		})
	})

	t.Run("happy path", func(t *testing.T) {
		for host := range hosts {
			t.Run(host, func(t *testing.T) {
				c := s443.Client()
				// only modify DialContext. the field TLSClientConfig, in particular,
				// has to be preserved since it holds the root CA cert pool
				// for the self-signed certificates being used.
				c.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					var d net.Dialer
					addr = strings.TrimPrefix(s443.URL, "https://") // a bit hacky. s443.URL is like "https://127.0.0.1:51678"
					return d.DialContext(ctx, network, addr)
				}

				rsp, err := c.Get("https://user:pass@" + host + "/path/?key=val#frag")
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
				if got := strings.TrimSpace(string(b)); got != host {
					t.Errorf("body: want: %s, got: %s", host, got)
					return
				}
			})
		}
	})
}
