//go:build !with_lx_probe

package clashapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStrictProbeDisabledPreservesLegacyDispatch_LX(t *testing.T) {
	for _, target := range []string{"/delay", "/delay?expected_status=204"} {
		recorder := httptest.NewRecorder()
		if serveHTTP204Probe(nil, recorder, httptest.NewRequest(http.MethodGet, target, nil)) {
			t.Fatalf("disabled extension intercepted %s", target)
		}
		if recorder.Body.Len() != 0 {
			t.Fatal("disabled extension wrote a response")
		}
	}
}
