//go:build with_ebpf && (linux || android)

package ebpf

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	ECommon "github.com/sagernet/sing-box/common/ebpf"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	udpnat "github.com/sagernet/sing/common/udpnat2"
)

type reconnectPacketEvent struct {
	route       adapter.InboundContext
	destination M.Socksaddr
	conn        N.PacketConn
	writer      *udpPacketWriter
	closed      <-chan struct{}
}

type reconnectPacketRouter struct {
	adapter.Router
	packets chan reconnectPacketEvent
}

func (r *reconnectPacketRouter) RoutePacketConnectionEx(_ context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	closed := make(chan struct{})
	defer func() {
		conn.Close()
		onClose(nil)
		close(closed)
	}()
	writer := conn.(interface{ Upstream() any }).Upstream().(*udpPacketWriter)
	buffer := buf.NewPacket()
	defer buffer.Release()
	for {
		buffer.Reset()
		destination, err := conn.ReadPacket(buffer)
		if err != nil {
			return
		}
		r.packets <- reconnectPacketEvent{metadata, destination, conn, writer, closed}
	}
}

func newReconnectTestInbound(t *testing.T) (*Inbound, *reconnectPacketRouter) {
	t.Helper()
	router := &reconnectPacketRouter{packets: make(chan reconnectPacketEvent, 8)}
	inbound := &Inbound{ctx: context.Background(), router: router, logger: log.NewNOPFactory().Logger()}
	inbound.setCgroupBackend(&ECommon.CgroupBackend{})
	inbound.udpNat = udpnat.New(inbound, inbound.preparePacketConnection, time.Minute, false)
	t.Cleanup(inbound.udpNat.Purge)
	return inbound, router
}

func (r *reconnectPacketRouter) nextPacket(t *testing.T) reconnectPacketEvent {
	t.Helper()
	select {
	case event := <-r.packets:
		return event
	case <-time.After(time.Second):
		t.Fatal("UDP packet was not routed")
		return reconnectPacketEvent{}
	}
}

func sendReconnectPacket(inbound *Inbound, client, destination netip.AddrPort, token netip.Addr, connected bool) {
	sessionKey := client
	if connected {
		sessionKey = netip.AddrPortFrom(token, client.Port())
	}
	// Seed the cache with the original destination supplied by the kernel.
	inbound.udpClientTable.setBinding(sessionKey, destination, token, connected)
	oob := ipv4PacketInfo
	if token.Is6() {
		oob = ipv6PacketInfo
	}
	packet := buf.As([]byte{1})
	inbound.NewPacket(packet, oob(token), M.SocksaddrFromNetIP(client))
	packet.Release()
}

func TestUDPReconnectReroutesExistingSource(t *testing.T) {
	for _, family := range []struct {
		name   string
		client string
		first  string
		second string
		tokens [2]string
	}{
		{"ipv4", "127.0.0.1:23456", "192.0.2.1:443", "192.0.2.2:443", [2]string{"127.128.0.1", "127.128.0.2"}},
		{"ipv6", "[::1]:23456", "[2001:db8::1]:443", "[2001:db8::2]:443", [2]string{"fd53:696e:672d:626f::1", "fd53:696e:672d:626f::2"}},
	} {
		t.Run(family.name, func(t *testing.T) {
			inbound, router := newReconnectTestInbound(t)
			client := netip.MustParseAddrPort(family.client)
			for index, destinationString := range []string{family.first, family.second} {
				destination := netip.MustParseAddrPort(destinationString)
				token := netip.MustParseAddr(family.tokens[index])
				sendReconnectPacket(inbound, client, destination, token, true)
				event := router.nextPacket(t)
				if event.destination.AddrPort() != destination || event.route.Destination.AddrPort() != destination {
					t.Fatalf("packet to %v reused connected UDP route to %v", destination, event.route.Destination)
				}
				if !event.route.UDPConnect || event.route.Source.AddrPort() != client || event.writer.client != client {
					t.Fatal("connected UDP routing or reply lost the real client address")
				}
				binding, loaded := event.writer.clientState.redirectBinding(destination)
				if !loaded || binding.address != token {
					t.Fatal("reply reused the previous peer's redirect token")
				}
			}
		})
	}
}

func TestUDPReusedSourcePortKeepsNewSocketSession(t *testing.T) {
	inbound, router := newReconnectTestInbound(t)
	client := netip.MustParseAddrPort("127.0.0.1:23456")
	destination := netip.MustParseAddrPort("192.0.2.1:443")
	tokens := []netip.Addr{netip.MustParseAddr("127.128.0.1"), netip.MustParseAddr("127.128.0.2")}
	// Deliver both sockets' packets before waiting for either route handler.
	for _, token := range tokens {
		sendReconnectPacket(inbound, client, destination, token, true)
	}
	first, second := router.nextPacket(t), router.nextPacket(t)
	if first.conn == second.conn || first.writer.clientState == second.writer.clientState {
		t.Fatal("different sockets sharing a source port reused one NAT session")
	}
	if first.writer.sessionKey.Addr() != tokens[0] {
		first, second = second, first
	}
	first.conn.Close()
	select {
	case <-first.closed:
	case <-time.After(time.Second):
		t.Fatal("old UDP session cleanup did not finish")
	}
	if !inbound.udpClientTable.current(second.writer.sessionKey, second.writer.clientState) {
		t.Fatal("old session cleanup removed the new socket's state")
	}
	sendReconnectPacket(inbound, client, destination, tokens[1], true)
	if event := router.nextPacket(t); event.conn != second.conn {
		t.Fatal("same connected socket lost its existing NAT session")
	}
}

func TestUDPUnconnectedSocketKeepsMultipleDestinations(t *testing.T) {
	inbound, router := newReconnectTestInbound(t)
	client := netip.MustParseAddrPort("127.0.0.1:23456")
	firstDestination := netip.MustParseAddrPort("192.0.2.1:443")
	secondDestination := netip.MustParseAddrPort("192.0.2.2:443")
	sendReconnectPacket(inbound, client, firstDestination, netip.MustParseAddr("127.128.0.1"), false)
	first := router.nextPacket(t)
	sendReconnectPacket(inbound, client, secondDestination, netip.MustParseAddr("127.128.0.2"), false)
	second := router.nextPacket(t)
	if first.conn != second.conn || second.destination.AddrPort() != secondDestination || second.route.UDPConnect {
		t.Fatal("unconnected UDP did not preserve its session across destinations")
	}
	for _, destination := range []netip.AddrPort{firstDestination, secondDestination} {
		if _, loaded := second.writer.clientState.redirectBinding(destination); !loaded {
			t.Fatalf("unconnected UDP lost reply binding for %v", destination)
		}
	}
}
