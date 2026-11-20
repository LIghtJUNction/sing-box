//go:build with_lx_probe

package clashapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing/service"

	"github.com/go-chi/render"
)

// serveHTTP204Probe is an opt-in path of the existing delay endpoint. It keeps
// strict health evidence separate from the legacy HEAD-based delay history.
func serveHTTP204Probe(server *Server, w http.ResponseWriter, r *http.Request) bool {
	query := r.URL.Query()
	expected, requested := query["expected_status"]
	if !requested {
		return false
	}
	result := render.M{
		"strict_probe":    true,
		"expected_status": http.StatusNoContent,
		"ok":              false,
		"http_status":     nil,
	}
	invalid := func() bool {
		result["reason"] = "invalid_request"
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, result)
		return true
	}
	if len(expected) != 1 || expected[0] != "204" {
		return invalid()
	}
	links, timeouts := query["url"], query["timeout"]
	if len(links) != 1 || len(timeouts) != 1 {
		return invalid()
	}
	timeout, err := strconv.ParseInt(timeouts[0], 10, 16)
	if err != nil || timeout <= 0 {
		return invalid()
	}
	// Keep the caller's cancellation while reusing the core's certificate
	// store and adjusted clock. Do not substitute a background request.
	ctx := r.Context()
	if registry := service.RegistryFromContext(server.ctx); registry != nil {
		ctx = service.ContextWithRegistry(ctx, registry)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*time.Duration(timeout))
	defer cancel()
	proxy := r.Context().Value(CtxKeyProxy).(adapter.Outbound)
	delay, err := urltest.URLTestHTTP204(ctx, links[0], proxy)
	if errors.Is(err, urltest.ErrInvalidHTTP204URL) {
		return invalid()
	}
	var statusError *urltest.HTTP204StatusError
	observedStatus := errors.As(err, &statusError)
	if observedStatus {
		result["http_status"] = statusError.StatusCode
	} else if err == nil {
		result["http_status"] = http.StatusNoContent
	}
	switch {
	case errors.Is(ctx.Err(), context.Canceled), errors.Is(err, context.Canceled):
		result["reason"] = "cancelled"
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		result["reason"] = "timeout"
	case observedStatus:
		result["reason"] = "http_status"
	case err != nil:
		result["reason"] = "transport_failure"
	default:
		result["ok"] = true
		result["delay"] = delay
	}
	// A failed measurement is still a completed API request. Returning its
	// evidence with 200 lets clients distinguish it from an unsupported core.
	render.JSON(w, r, result)
	return true
}
