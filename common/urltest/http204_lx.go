//go:build with_lx_probe

package urltest

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/ntp"
)

var ErrInvalidHTTP204URL = errors.New("HTTP 204 probe requires an absolute HTTPS URL without credentials or fragment")

// HTTP204StatusError preserves the observed status without exposing the URL or
// reading an untrusted response body.
type HTTP204StatusError struct {
	StatusCode int
}

func (e *HTTP204StatusError) Error() string {
	return fmt.Sprintf("HTTP 204 probe received status %d", e.StatusCode)
}

// URLTestHTTP204 checks a selected outbound without changing a selector or its
// delay history. A response is successful only after a verified HTTPS GET
// returns 204; redirects and arbitrary HTTP responses are not reachability.
func URLTestHTTP204(ctx context.Context, link string, detour N.Dialer) (uint16, error) {
	if len(link) > 4096 {
		return 0, ErrInvalidHTTP204URL
	}
	target, err := url.Parse(link)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" ||
		target.User != nil || target.Opaque != "" || target.Fragment != "" {
		return 0, ErrInvalidHTTP204URL
	}
	if port := target.Port(); port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return 0, ErrInvalidHTTP204URL
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return 0, ErrInvalidHTTP204URL
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return detour.DialContext(ctx, network, M.ParseSocksaddr(address))
		},
		TLSClientConfig: &tls.Config{
			Time:    ntp.TimeFuncFromContext(ctx),
			RootCAs: adapter.RootPoolFromContext(ctx),
		},
		TLSHandshakeTimeout:    C.TCPTimeout,
		MaxResponseHeaderBytes: 16 * 1024,
		DisableKeepAlives:      true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: C.TCPTimeout,
	}
	start := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	// No response body is needed. In particular, never drain an error page.
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return 0, &HTTP204StatusError{StatusCode: response.StatusCode}
	}
	return uint16(time.Since(start) / time.Millisecond), nil
}
