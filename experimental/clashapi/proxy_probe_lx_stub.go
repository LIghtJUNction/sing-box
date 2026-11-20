//go:build !with_lx_probe

package clashapi

import "net/http"

// Without the extension, preserve the legacy delay endpoint's behavior.
// Consumers of strict probes must require the explicit http_status evidence.
func serveHTTP204Probe(*Server, http.ResponseWriter, *http.Request) bool {
	return false
}
