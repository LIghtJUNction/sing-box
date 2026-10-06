package v2rayxhttp

import "net/http"

// newH2CTestServer serves the same prior-knowledge HTTP/2 used by the test
// clients. Native Protocols replace the deprecated Upgrade-based h2c wrapper.
func newH2CTestServer(handler http.Handler) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{Handler: handler, Protocols: protocols}
}
