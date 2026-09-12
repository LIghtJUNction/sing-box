package clashapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
)

type loginEndpoint struct {
	adapter.Endpoint
	adapter.TailscaleEndpoint
	done chan struct{}
}

func (e *loginEndpoint) SubscribeTailscaleStatus(ctx context.Context, callback func(*adapter.TailscaleEndpointStatus)) error {
	defer close(e.done)
	callback(&adapter.TailscaleEndpointStatus{BackendState: "NeedsLogin", AuthURL: "https://login.tailscale.com/a/fixture"})
	<-ctx.Done()
	return ctx.Err()
}

type loginEndpoints struct {
	adapter.EndpointManager
	endpoint adapter.Endpoint
}

func (e loginEndpoints) Get(tag string) (adapter.Endpoint, bool) { return e.endpoint, tag == "tailnet" }

func TestTailscaleLoginStatusIsPrivateAndCancelsSubscription(t *testing.T) {
	endpoint := &loginEndpoint{done: make(chan struct{})}
	handler := tailscaleRouter(&Server{endpoint: loginEndpoints{endpoint: endpoint}, tailscaleSecret: "fixture-secret"})
	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/tailnet", nil)
	request.Header.Set("Authorization", "Bearer fixture-secret")
	handler.ServeHTTP(response, request)
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(response)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 3 || payload["state"] != "NeedsLogin" || payload["auth_url"] != "https://login.tailscale.com/a/fixture" || payload["online"] != false {
		t.Fatal(payload)
	}
	select {
	case <-endpoint.done:
	case <-time.After(time.Second):
		t.Fatal("status watcher leaked")
	}
}
func TestTailscaleLoginStatusRejectsMissingEndpoint(t *testing.T) {
	handler := tailscaleRouter(&Server{endpoint: loginEndpoints{}, tailscaleSecret: "fixture-secret"})
	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/missing", nil)
	request.Header.Set("Authorization", "Bearer fixture-secret")
	handler.ServeHTTP(response, request)
	if response.Code != 404 {
		t.Fatal(response.Code)
	}
}

func TestTailscaleLoginStatusRequiresConfiguredSecret(t *testing.T) {
	response := httptest.NewRecorder()
	tailscaleRouter(&Server{}).ServeHTTP(response, httptest.NewRequest("GET", "/tailnet", nil))
	if response.Code != 403 {
		t.Fatal(response.Code)
	}
}

func TestTailscaleLoginStatusRejectsUnauthenticatedCaller(t *testing.T) {
	response := httptest.NewRecorder()
	tailscaleRouter(&Server{tailscaleSecret: "fixture-secret"}).ServeHTTP(response, httptest.NewRequest("GET", "/tailnet", nil))
	if response.Code != 401 {
		t.Fatal(response.Code)
	}
}
