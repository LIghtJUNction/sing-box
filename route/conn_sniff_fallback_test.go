package route

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type sniffFallbackDialer struct {
	domainErr   error
	fallbackErr error
	remote      net.Conn
	calls       []M.Socksaddr
}

func (d *sniffFallbackDialer) DialContext(_ context.Context, _ string, destination M.Socksaddr) (net.Conn, error) {
	d.calls = append(d.calls, destination)
	if destination.IsIP() {
		return d.remote, d.fallbackErr
	}
	return nil, d.domainErr
}

func (*sniffFallbackDialer) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("unexpected packet dial")
}

type sniffPolicyDialer struct {
	sniffFallbackDialer
	networkCalls       int
	destination        M.Socksaddr
	addresses          []netip.Addr
	strategy           *C.NetworkStrategy
	interfaces         []C.InterfaceType
	fallbackInterfaces []C.InterfaceType
	delay              time.Duration
}

func (d *sniffPolicyDialer) DialParallelNetwork(_ context.Context, network string, destination M.Socksaddr, addresses []netip.Addr, strategy *C.NetworkStrategy, interfaces, fallbackInterfaces []C.InterfaceType, delay time.Duration) (net.Conn, error) {
	if network != N.NetworkTCP {
		return nil, errors.New("unexpected network")
	}
	d.networkCalls++
	d.destination, d.addresses, d.strategy = destination, slices.Clone(addresses), strategy
	d.interfaces, d.fallbackInterfaces, d.delay = slices.Clone(interfaces), slices.Clone(fallbackInterfaces), delay
	return d.remote, d.fallbackErr
}

func (*sniffPolicyDialer) ListenSerialNetworkPacket(context.Context, M.Socksaddr, []netip.Addr, *C.NetworkStrategy, []C.InterfaceType, []C.InterfaceType, time.Duration) (net.PacketConn, netip.Addr, error) {
	return nil, netip.Addr{}, errors.New("unexpected packet dial")
}

func sniffFallbackFailure(t *testing.T, dialer N.Dialer, metadata adapter.InboundContext) error {
	t.Helper()
	inbound, peer := net.Pipe()
	t.Cleanup(func() { inbound.Close(); peer.Close() })
	var closeErr error
	calls := 0
	manager := NewConnectionManager(context.Background(), log.NewNOPFactory().Logger())
	manager.NewConnection(context.Background(), dialer, inbound, metadata, func(err error) {
		calls++
		closeErr = err
	})
	if calls != 1 {
		t.Fatalf("close callback calls = %d, want 1", calls)
	}
	return closeErr
}

func TestSniffFallbackPreservesNetworkPolicyAndBothErrors(t *testing.T) {
	domainErr := errors.New("domain dial failed")
	fallbackErr := errors.New("original IP dial failed")
	strategy := C.NetworkStrategyFallback
	original := M.Socksaddr{Addr: netip.MustParseAddr("192.0.2.10"), Port: 443}
	metadata := adapter.InboundContext{
		Destination:              M.Socksaddr{Fqdn: "unresolved.example", Port: 443},
		RouteOriginalDestination: original,
		NetworkStrategy:          &strategy,
		NetworkType:              []C.InterfaceType{C.InterfaceTypeWIFI},
		FallbackNetworkType:      []C.InterfaceType{C.InterfaceTypeCellular},
		FallbackDelay:            123 * time.Millisecond,
	}
	d := &sniffPolicyDialer{sniffFallbackDialer: sniffFallbackDialer{domainErr: domainErr, fallbackErr: fallbackErr}}
	err := sniffFallbackFailure(t, d, metadata)
	if len(d.calls) != 1 || d.calls[0] != metadata.Destination || d.networkCalls != 1 {
		t.Fatalf("domain calls = %v, policy calls = %d", d.calls, d.networkCalls)
	}
	if d.destination != original || !slices.Equal(d.addresses, []netip.Addr{original.Addr}) ||
		d.strategy != &strategy || !slices.Equal(d.interfaces, metadata.NetworkType) ||
		!slices.Equal(d.fallbackInterfaces, metadata.FallbackNetworkType) || d.delay != metadata.FallbackDelay {
		t.Fatalf("fallback lost network selection: %#v", d)
	}
	if !errors.Is(err, domainErr) || !errors.Is(err, fallbackErr) {
		t.Fatalf("close error = %v, want both dial errors", err)
	}
}

func TestSniffFallbackPlainDialerRetriesOnce(t *testing.T) {
	domainErr, fallbackErr := errors.New("domain failed"), errors.New("IP failed")
	original := M.Socksaddr{Addr: netip.MustParseAddr("192.0.2.11"), Port: 8443}
	destination := M.Socksaddr{Fqdn: "unresolved.example", Port: 8443}
	d := &sniffFallbackDialer{domainErr: domainErr, fallbackErr: fallbackErr}
	err := sniffFallbackFailure(t, d, adapter.InboundContext{Destination: destination, RouteOriginalDestination: original})
	if !slices.Equal(d.calls, []M.Socksaddr{destination, original}) {
		t.Fatalf("dial sequence = %v, want domain then original IP once", d.calls)
	}
	if !errors.Is(err, fallbackErr) {
		t.Fatalf("close error does not preserve fallback failure: %v", err)
	}
}

func TestSniffFallbackRequiresDifferentOriginalDestination(t *testing.T) {
	domain := M.Socksaddr{Fqdn: "unresolved.example", Port: 443}
	ip := M.Socksaddr{Addr: netip.MustParseAddr("192.0.2.12"), Port: 443}
	for _, tc := range []struct {
		name                  string
		destination, original M.Socksaddr
	}{
		{"missing original", domain, M.Socksaddr{}},
		{"same destination", domain, domain},
		{"IP destination", ip, M.Socksaddr{Addr: netip.MustParseAddr("192.0.2.13"), Port: 443}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &sniffFallbackDialer{domainErr: errors.New("failed"), fallbackErr: errors.New("failed")}
			sniffFallbackFailure(t, d, adapter.InboundContext{Destination: tc.destination, RouteOriginalDestination: tc.original})
			if len(d.calls) != 1 {
				t.Fatalf("dial calls = %v, want one attempt", d.calls)
			}
		})
	}
}
