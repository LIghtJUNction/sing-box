package wireguard

import (
	"context"
	"net/netip"
	"sync"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/wireguard-go/device"
)

func TestEndpointFirstKeepaliveHasDomainResolver(t *testing.T) {
	resolved := make(chan struct{})
	var resolveOnce sync.Once
	e, err := NewEndpoint(EndpointOptions{
		Context:     context.Background(),
		Logger:      logger.NOP(),
		Dialer:      N.SystemDialer,
		UDPTimeout:  C.UDPTimeout,
		ICMPTimeout: C.ICMPTimeout,
		Workers:     1,
		MTU:         1280,
		Address:     []netip.Prefix{netip.MustParsePrefix("10.0.0.2/32")},
		PrivateKey:  "iOx8sYFBnQjKMkTfTBaLZ5+DEHU4S3vzcSLp+HDaOWc=",
		ResolvePeer: func(domain string) ([]netip.Addr, error) {
			if domain != "peer.test" {
				t.Errorf("unexpected peer domain: %s", domain)
			}
			resolveOnce.Do(func() { close(resolved) })
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		},
		Peers: []PeerOptions{{
			Endpoint:                    M.ParseSocksaddr("peer.test:51820"),
			PublicKey:                   "eBpqZlJmSVBGNW9BOEZ4S3lRZTNQd0RhbWFnZTBTNTA=",
			AllowedIPs:                  []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
			PersistentKeepaliveInterval: "1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := e.Initialize(nil); err != nil {
		t.Fatal(err)
	}
	// An already-up device triggers persistent keepalive synchronously from
	// IpcSet. The resolver must therefore exist before configuration returns.
	e.beforeConfigureForTest = func(wgDevice *device.Device) error { return wgDevice.Up() }
	if err := e.Start(true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-resolved:
	default:
		t.Fatal("first keepalive handshake missed the domain resolver installed before IpcSet")
	}
}
