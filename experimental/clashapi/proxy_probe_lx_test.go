//go:build with_lx_probe

package clashapi

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

type strictProbeCertificateStore struct{ pool *x509.CertPool }

func (s *strictProbeCertificateStore) Name() string                                   { return "probe-test" }
func (s *strictProbeCertificateStore) Start(adapter.StartStage, *adapter.Scope) error { return nil }
func (s *strictProbeCertificateStore) Pool() *x509.CertPool                           { return s.pool }
func (s *strictProbeCertificateStore) ExclusiveAnchors() bool                         { return true }

func strictProbeCoreContext(server *httptest.Server) context.Context {
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return service.ContextWith[adapter.CertificateStore](context.Background(), &strictProbeCertificateStore{pool: pool})
}

type strictProbeOutbound struct {
	N.DefaultDialer
	calls atomic.Int32
}

func (d *strictProbeOutbound) Type() string           { return "test" }
func (d *strictProbeOutbound) Tag() string            { return "selected-test-outbound" }
func (d *strictProbeOutbound) Network() []string      { return []string{"tcp"} }
func (d *strictProbeOutbound) Dependencies() []string { return nil }
func (d *strictProbeOutbound) DialContext(ctx context.Context, network string, address M.Socksaddr) (net.Conn, error) {
	d.calls.Add(1)
	return d.DefaultDialer.DialContext(ctx, network, address)
}

func strictProbeRequest(ctx context.Context, target string, outbound *strictProbeOutbound) *http.Request {
	return httptest.NewRequest(http.MethodGet, target, nil).WithContext(context.WithValue(ctx, CtxKeyProxy, outbound))
}

type strictProbeResponse struct {
	StrictProbe    bool    `json:"strict_probe"`
	ExpectedStatus int     `json:"expected_status"`
	OK             bool    `json:"ok"`
	HTTPStatus     *int    `json:"http_status"`
	Delay          *uint16 `json:"delay"`
	Reason         string  `json:"reason"`
}

func decodeStrictProbeResponse(t *testing.T, recorder *httptest.ResponseRecorder) strictProbeResponse {
	t.Helper()
	var result strictProbeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !result.StrictProbe || result.ExpectedStatus != 204 {
		t.Fatalf("strict capability evidence missing: %s", recorder.Body)
	}
	return result
}

func TestStrictProxyProbeReturnsStatusAndPreservesLegacyDispatch_LX(t *testing.T) {
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer fixture.Close()
	// Nil history and manager ensure strict measurements cannot mutate them.
	server := &Server{ctx: strictProbeCoreContext(fixture)}
	outbound := &strictProbeOutbound{}
	recorder := httptest.NewRecorder()
	target := "/delay?expected_status=204&timeout=1000&url=" + url.QueryEscape(fixture.URL)
	getProxyDelay(server)(recorder, strictProbeRequest(context.Background(), target, outbound))
	if recorder.Code != http.StatusOK {
		t.Fatalf("API status: %d; %s", recorder.Code, recorder.Body)
	}
	result := decodeStrictProbeResponse(t, recorder)
	if !result.OK || result.HTTPStatus == nil || *result.HTTPStatus != 204 || result.Delay == nil || result.Reason != "" {
		t.Fatalf("unexpected success: %s", recorder.Body)
	}
	if outbound.calls.Load() != 1 {
		t.Fatalf("selected outbound calls: %d", outbound.calls.Load())
	}
	legacy := httptest.NewRecorder()
	if serveHTTP204Probe(server, legacy, httptest.NewRequest(http.MethodGet, "/delay?timeout=1000", nil)) {
		t.Fatal("strict handler captured a legacy delay request")
	}
	if legacy.Body.Len() != 0 {
		t.Fatal("strict handler wrote a legacy response")
	}
}

func TestStrictProxyProbeValidatesParametersBeforeDial_LX(t *testing.T) {
	base := "&timeout=1000&url=https%3A%2F%2Fexample.invalid%2F"
	for _, query := range []string{
		"expected_status=200" + base,
		"expected_status=" + base,
		"expected_status=204&expected_status=204" + base,
		"expected_status=204&timeout=0&url=https://example.invalid/",
		"expected_status=204&timeout=-1&url=https://example.invalid/",
		"expected_status=204&timeout=32768&url=https://example.invalid/",
		"expected_status=204&timeout=1000",
		"expected_status=204&timeout=1000&timeout=1000&url=https://example.invalid/",
		"expected_status=204&timeout=1000&url=https://example.invalid/&url=https://example.invalid/",
		"expected_status=204&timeout=1000&url=http://example.invalid/",
		"expected_status=204&timeout=1000&url=https://user:password@example.invalid/",
	} {
		outbound := &strictProbeOutbound{}
		recorder := httptest.NewRecorder()
		getProxyDelay(&Server{ctx: context.Background()})(recorder, strictProbeRequest(context.Background(), "/delay?"+query, outbound))
		result := decodeStrictProbeResponse(t, recorder)
		if recorder.Code != http.StatusBadRequest || result.OK || result.Reason != "invalid_request" || result.HTTPStatus != nil || result.Delay != nil {
			t.Fatalf("invalid request accepted: %q => %d %s", query, recorder.Code, recorder.Body)
		}
		if outbound.calls.Load() != 0 {
			t.Fatalf("invalid request dialed: %q", query)
		}
	}
}

func TestStrictProxyProbeFailureEvidenceAndCancellation_LX(t *testing.T) {
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/timeout" {
			<-r.Context().Done()
			return
		}
		status := map[string]int{"/ordinary": 200, "/forbidden": 403, "/error": 500, "/redirect": 302}[r.URL.Path]
		if status == 302 {
			w.Header().Set("Location", "/redirect-target")
		}
		if status == 0 {
			status = 204
		}
		w.WriteHeader(status)
	}))
	defer fixture.Close()
	coreCtx := strictProbeCoreContext(fixture)
	for _, tc := range []struct {
		path    string
		status  int
		reason  string
		timeout string
	}{
		{"/ordinary", 200, "http_status", "1000"},
		{"/forbidden", 403, "http_status", "1000"},
		{"/error", 500, "http_status", "1000"},
		{"/redirect", 302, "http_status", "1000"},
		{"/timeout", 0, "timeout", "100"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			target := "/delay?expected_status=204&timeout=" + tc.timeout + "&url=" + url.QueryEscape(fixture.URL+tc.path)
			getProxyDelay(&Server{ctx: coreCtx})(recorder, strictProbeRequest(context.Background(), target, &strictProbeOutbound{}))
			result := decodeStrictProbeResponse(t, recorder)
			if recorder.Code != http.StatusOK || result.OK || result.Reason != tc.reason || result.Delay != nil {
				t.Fatalf("failure lost its evidence: %d %s", recorder.Code, recorder.Body)
			}
			if tc.status == 0 {
				if result.HTTPStatus != nil {
					t.Fatalf("invented HTTP status: %s", recorder.Body)
				}
			} else if result.HTTPStatus == nil || *result.HTTPStatus != tc.status {
				t.Fatalf("actual HTTP status missing: %s", recorder.Body)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	target := "/delay?expected_status=204&timeout=1000&url=" + url.QueryEscape(fixture.URL)
	getProxyDelay(&Server{ctx: coreCtx})(recorder, strictProbeRequest(ctx, target, &strictProbeOutbound{}))
	result := decodeStrictProbeResponse(t, recorder)
	if recorder.Code != http.StatusOK || result.OK || result.Reason != "cancelled" || result.HTTPStatus != nil || result.Delay != nil {
		t.Fatalf("caller cancellation lost: %d %s", recorder.Code, recorder.Body)
	}
	recorder = httptest.NewRecorder()
	getProxyDelay(&Server{ctx: context.Background()})(recorder, strictProbeRequest(context.Background(), target, &strictProbeOutbound{}))
	result = decodeStrictProbeResponse(t, recorder)
	if recorder.Code != http.StatusOK || result.OK || result.Reason != "transport_failure" || result.HTTPStatus != nil {
		t.Fatalf("untrusted TLS failure lost: %d %s", recorder.Code, recorder.Body)
	}
}
