package networkproxy

import (
	"context"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseProxyURL(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:7890", "https://user:p%40ss@proxy.example:443/", "http://[::1]:8080", "http://proxy.example"} {
		if _, err := Parse(raw); err != nil {
			t.Errorf("valid proxy rejected: %v", err)
		}
	}
	for _, raw := range []string{"", "socks5://host:1080", "ftp://host", "http:///path", "http://host:0", "http://host:65536", "http://host/path", "http://host?token=secret", "http://host#secret", "http://user:secret@host:bad"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("invalid proxy accepted: %q", raw)
		} else if strings.Contains(err.Error(), "secret") {
			t.Fatal("validation error leaked proxy credentials")
		}
	}
}

func TestTransportRoutesConcurrentRequestsByContext(t *testing.T) {
	makeProxy := func(label string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
				t.Error("missing proxy credentials")
			}
			if r.URL.Host != "upstream.invalid" {
				t.Errorf("unexpected destination: %s", r.URL.Host)
			}
			io.WriteString(w, label)
		}))
	}
	a, b := makeProxy("a"), makeProxy("b")
	defer a.Close()
	defer b.Close()
	transport := NewTransport()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	var calls sync.WaitGroup
	for i := 0; i < 12; i++ {
		for _, target := range []struct{ address, want string }{{a.URL, "a"}, {b.URL, "b"}} {
			calls.Add(1)
			go func(address, want string) {
				defer calls.Done()
				u, _ := Parse(strings.Replace(address, "http://", "http://user:pass@", 1))
				req, _ := http.NewRequestWithContext(WithURL(context.Background(), u), "GET", "http://upstream.invalid/check", nil)
				resp, err := client.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				if string(body) != want {
					t.Errorf("response=%q want=%q", body, want)
				}
			}(target.address, target.want)
		}
	}
	calls.Wait()
}

func TestTransportHTTPSProxyAndConnectTunnel(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "secure-origin") }))
	defer origin.Close()
	var tunnels atomic.Int32
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
			t.Error("missing CONNECT authentication")
		}
		if r.Method != http.MethodConnect {
			io.WriteString(w, "secure-proxy")
			return
		}
		if r.Host != origin.Listener.Addr().String() {
			t.Error("unexpected tunnel destination")
			http.Error(w, "bad destination", 400)
			return
		}
		upstream, err := net.DialTimeout("tcp", r.Host, time.Second)
		if err != nil {
			t.Error(err)
			return
		}
		client, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			t.Error(err)
			return
		}
		tunnels.Add(1)
		io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { io.Copy(upstream, client); upstream.Close() }()
		io.Copy(client, upstream)
		client.Close()
	}))
	defer proxy.Close()
	transport := NewTransport()
	transport.TLSClientConfig = proxy.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	pool := x509.NewCertPool()
	pool.AddCert(proxy.Certificate())
	pool.AddCert(origin.Certificate())
	transport.TLSClientConfig.RootCAs = pool
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	u, _ := Parse(strings.Replace(proxy.URL, "https://", "https://user:pass@", 1))
	for _, item := range []struct{ address, want string }{{"http://upstream.invalid/", "secure-proxy"}, {origin.URL, "secure-origin"}} {
		req, _ := http.NewRequestWithContext(WithURL(context.Background(), u), "GET", item.address, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != item.want {
			t.Fatalf("response=%q want=%q", body, item.want)
		}
	}
	if tunnels.Load() != 1 {
		t.Fatal("HTTPS upstream bypassed CONNECT")
	}
}

func TestUnavailableProxyNeverFallsBackToDirect(t *testing.T) {
	var direct atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct.Add(1) }))
	defer origin.Close()
	proxy := httptest.NewServer(http.NotFoundHandler())
	u, _ := Parse(proxy.URL)
	proxy.Close()
	transport := NewTransport()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	req, _ := http.NewRequestWithContext(WithURL(context.Background(), u), "GET", origin.URL, nil)
	if _, err := client.Do(req); err == nil {
		t.Fatal("unavailable proxy unexpectedly succeeded")
	}
	if direct.Load() != 0 {
		t.Fatal("request bypassed configured proxy")
	}
	req, _ = http.NewRequest("GET", origin.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if direct.Load() != 1 {
		t.Fatal("channel without proxy failed to use its direct connection")
	}
}
