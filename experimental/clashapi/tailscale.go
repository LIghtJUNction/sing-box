package clashapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/sagernet/sing-box/adapter"
)

// Login status is served inside the authenticated API group. Never return the
// full tailnet peer list or retain an unbounded status subscription for polling.
func tailscaleLoginStatus(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if server.tailscaleSecret == "" {
			http.Error(w, "API secret required", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+server.tailscaleSecret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		endpoint, found := server.endpoint.Get(getEscapeParam(r, "name"))
		if !found {
			http.Error(w, "endpoint not found", http.StatusNotFound)
			return
		}
		tailscale, ok := endpoint.(adapter.TailscaleEndpoint)
		if !ok {
			http.Error(w, "not a tailscale endpoint", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		statuses := make(chan *adapter.TailscaleEndpointStatus, 1)
		failures := make(chan error, 1)
		go func() {
			failures <- tailscale.SubscribeTailscaleStatus(ctx, func(status *adapter.TailscaleEndpointStatus) {
				select {
				case statuses <- status:
				default:
				}
			})
		}()
		select {
		case status := <-statuses:
			if status == nil {
				http.Error(w, "status unavailable", http.StatusServiceUnavailable)
				return
			}
			render.JSON(w, r, map[string]any{"state": status.BackendState, "auth_url": status.AuthURL, "online": status.Self != nil && status.Self.Online})
		case <-failures:
			http.Error(w, "status unavailable", http.StatusServiceUnavailable)
		case <-ctx.Done():
			http.Error(w, "status timeout", http.StatusGatewayTimeout)
		}
	}
}

func tailscaleRouter(server *Server) http.Handler {
	r := chi.NewRouter()
	r.Get("/{name}", tailscaleLoginStatus(server))
	return r
}
