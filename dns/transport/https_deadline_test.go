package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	mDNS "github.com/miekg/dns"
)

type testHTTPDialer struct{ N.Dialer }

func (testHTTPDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, destination.String())
}

func TestHTTPSQueryDeadlineDoesNotCancelConcurrentQuery(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		var query mDNS.Msg
		if query.Unpack(data) != nil {
			return
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		var answer mDNS.Msg
		answer.SetReply(&query)
		encoded, _ := answer.Pack()
		w.Header().Set("Content-Type", MimeType)
		_, _ = w.Write(encoded)
	}))
	defer server.Close()
	destination, _ := url.Parse(server.URL)
	transport := NewHTTPSRaw(dns.TransportAdapter{}, log.NewNOPFactory().Logger(), testHTTPDialer{}, destination, http.Header{}, M.ParseSocksaddr(destination.Host), nil)
	defer transport.Reset()
	query := new(mDNS.Msg)
	query.SetQuestion("example.test.", mDNS.TypeA)
	longCtx, longCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer longCancel()
	longResult := make(chan error, 1)
	go func() { _, err := transport.Exchange(longCtx, query); longResult <- err }()
	select {
	case <-started:
	case <-longCtx.Done():
		t.Fatal("long query did not start")
	}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shortCancel()
	_, err := transport.Exchange(shortCtx, query)
	if !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatalf("short query: %v", err)
	}
	close(release)
	if err := <-longResult; err != nil {
		t.Fatalf("unrelated query was canceled: %v", err)
	}
}
