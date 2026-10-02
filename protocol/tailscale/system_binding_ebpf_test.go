//go:build with_tailscale && with_ebpf && linux

package tailscale

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/service"
)

type bindingTestNetwork struct {
	adapter.NetworkManager
	finder *control.DefaultInterfaceFinder
}

func (n *bindingTestNetwork) AutoRedirectOutputMark() uint32 { return 0 }
func (n *bindingTestNetwork) AutoDetectInterfaceFunc() control.Func {
	return func(string, string, syscall.RawConn) error { return nil }
}
func (n *bindingTestNetwork) InterfaceFinder() control.InterfaceFinder { return n.finder }
func (n *bindingTestNetwork) InterfaceMonitor() tun.DefaultInterfaceMonitor {
	return bindingTestMonitor{}
}

type bindingTestMonitor struct{ tun.DefaultInterfaceMonitor }

func (bindingTestMonitor) DefaultInterface() *control.Interface { return nil }

func TestSystemBindingProtectsSecondaryUDPEgressSockets(t *testing.T) {
	loopback, err := net.InterfaceByName("lo")
	if err != nil {
		t.Skip("loopback interface unavailable")
	}
	finder := control.NewDefaultInterfaceFinder()
	// A controlled egress candidate is enough to exercise member socket
	// creation. The unassigned benchmark address keeps the test offline.
	finder.UpdateInterfaces([]control.Interface{{
		Index:     loopback.Index,
		Name:      loopback.Name,
		Flags:     net.FlagUp | net.FlagBroadcast,
		Addresses: []netip.Prefix{netip.MustParsePrefix("198.18.0.1/24")},
	}})
	ctx := service.ContextWith[adapter.NetworkManager](context.Background(), &bindingTestNetwork{finder: finder})
	adapter.PrepareEBPFSocketProtection(ctx)
	var protected atomic.Int32
	registration, err := adapter.RegisterEBPFSocketProtection(ctx, func(string, string, syscall.RawConn) error {
		protected.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Close()
	binding, err := newSystemBinding(ctx, log.NewNOPFactory().Logger())
	if err != nil {
		t.Fatal(err)
	}
	packetConn, err := binding.listenPacket(ctx, "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer packetConn.Close()
	if actual := protected.Load(); actual != 2 {
		t.Fatalf("socket protection called %d times; primary and secondary egress sockets both need protection", actual)
	}
}
