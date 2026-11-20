//go:build with_lx_probe

package urltest

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

type probeCertificateStore struct {
	pool *x509.CertPool
}

func (s *probeCertificateStore) Name() string                                   { return "probe-test" }
func (s *probeCertificateStore) Start(adapter.StartStage, *adapter.Scope) error { return nil }
func (s *probeCertificateStore) Pool() *x509.CertPool                           { return s.pool }
func (s *probeCertificateStore) ExclusiveAnchors() bool                         { return true }

func trustedProbeContext(server *httptest.Server) context.Context {
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return service.ContextWith[adapter.CertificateStore](context.Background(), &probeCertificateStore{pool: pool})
}

type recordingProbeDialer struct {
	N.DefaultDialer
	calls atomic.Int32
}

func (d *recordingProbeDialer) DialContext(ctx context.Context, network string, address M.Socksaddr) (net.Conn, error) {
	d.calls.Add(1)
	return d.DefaultDialer.DialContext(ctx, network, address)
}

func TestHTTP204ProbeRequiresGET204WithoutRedirect_LX(t *testing.T) {
	var heads, redirects atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusNoContent)
		case "/ordinary":
			w.WriteHeader(http.StatusOK)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
		case "/redirect":
			w.Header().Set("Location", "/redirect-target")
			w.WriteHeader(http.StatusFound)
		case "/redirect-target":
			redirects.Add(1)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	ctx := trustedProbeContext(server)
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/ok", 204}, {"/ordinary", 200}, {"/forbidden", 403},
		{"/error", 500}, {"/redirect", 302},
	} {
		t.Run(tc.path, func(t *testing.T) {
			dialer := &recordingProbeDialer{}
			_, err := URLTestHTTP204(ctx, server.URL+tc.path, dialer)
			if tc.status == 204 {
				if err != nil {
					t.Fatalf("GET 204 failed: %v", err)
				}
			} else {
				var statusError *HTTP204StatusError
				if !errors.As(err, &statusError) || statusError.StatusCode != tc.status {
					t.Fatalf("status %d was not rejected: %v", tc.status, err)
				}
			}
			if dialer.calls.Load() != 1 {
				t.Fatalf("selected dialer calls: %d", dialer.calls.Load())
			}
		})
	}
	if heads.Load() != 0 || redirects.Load() != 0 {
		t.Fatalf("strict probe used HEAD or followed redirect: heads=%d redirects=%d", heads.Load(), redirects.Load())
	}
	if _, err := URLTest(ctx, server.URL+"/ok", &recordingProbeDialer{}); err != nil {
		t.Fatalf("legacy HEAD delay changed: %v", err)
	}
	if heads.Load() != 1 {
		t.Fatalf("legacy delay did not use HEAD: %d", heads.Load())
	}
}

func TestHTTP204ProbeRejectsInvalidTargetsBeforeDial_LX(t *testing.T) {
	for _, target := range []string{
		"", "http://example.invalid/generate_204", "/generate_204", "https:opaque",
		"https://user:password@example.invalid/", "https://example.invalid/#fragment",
		"https://example.invalid:0/", "https://example.invalid:65536/",
		"https://example.invalid/" + strings.Repeat("a", 4096),
	} {
		dialer := &recordingProbeDialer{}
		_, err := URLTestHTTP204(context.Background(), target, dialer)
		if !errors.Is(err, ErrInvalidHTTP204URL) {
			t.Fatalf("target was not rejected: %q: %v", target, err)
		}
		if dialer.calls.Load() != 0 {
			t.Fatalf("invalid target dialed: %q", target)
		}
	}
}

func TestHTTP204ProbeVerifiesTLSAndBoundsHeaders_LX(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Oversized", strings.Repeat("a", 32*1024))
		w.WriteHeader(http.StatusNoContent)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	if _, err := URLTestHTTP204(context.Background(), server.URL, &recordingProbeDialer{}); err == nil {
		t.Fatal("untrusted TLS certificate was accepted")
	}
	if _, err := URLTestHTTP204(trustedProbeContext(server), server.URL, &recordingProbeDialer{}); err == nil {
		t.Fatal("oversized response headers were accepted")
	}
}

func TestHTTP204ProbeDoesNotDrainErrorBody_LX(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		// An error page can stall forever; the probe only needs its status.
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(trustedProbeContext(server), time.Second)
	defer cancel()
	_, err := URLTestHTTP204(ctx, server.URL, &recordingProbeDialer{})
	var statusError *HTTP204StatusError
	if !errors.As(err, &statusError) || statusError.StatusCode != 200 {
		t.Fatalf("probe waited for an error body: %v", err)
	}
}

func TestHTTP204ProbeHonorsTimeoutAndCancellation_LX(t *testing.T) {
	for _, timeout := range []bool{true, false} {
		name := "cancel"
		if timeout {
			name = "timeout"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{}, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				entered <- struct{}{}
				<-r.Context().Done()
			}))
			defer server.Close()
			var ctx context.Context
			var cancel context.CancelFunc
			if timeout {
				ctx, cancel = context.WithTimeout(trustedProbeContext(server), 100*time.Millisecond)
			} else {
				ctx, cancel = context.WithCancel(trustedProbeContext(server))
			}
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := URLTestHTTP204(ctx, server.URL, &recordingProbeDialer{})
				result <- err
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("TLS request did not reach fixture")
			}
			expected := context.DeadlineExceeded
			if !timeout {
				expected = context.Canceled
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, expected) {
					t.Fatalf("wanted %v, got %v", expected, err)
				}
			case <-time.After(time.Second):
				t.Fatal("probe ignored its context")
			}
		})
	}
}
