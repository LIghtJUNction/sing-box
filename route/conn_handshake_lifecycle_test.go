package route

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func TestPacketHandshakeFailureReleasesConnections(t *testing.T) {
	for _, connected := range []bool{false, true} {
		name := "listen"
		if connected {
			name = "connected"
		}
		t.Run(name, func(t *testing.T) {
			for _, withCallback := range []bool{false, true} {
				callbackName := "nil callback"
				if withCallback {
					callbackName = "callback"
				}
				t.Run(callbackName, func(t *testing.T) {
					inboundSocket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { inboundSocket.Close() })
					destination := inboundSocket.LocalAddr().(*net.UDPAddr)
					var remoteSocket *net.UDPConn
					if connected {
						remoteSocket, err = net.DialUDP("udp4", nil, destination)
					} else {
						remoteSocket, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
					}
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { remoteSocket.Close() })

					handshakeErr := errors.New("inbound handshake response failed")
					inbound := &packetHandshakeFailureConn{
						PacketConn:   bufio.NewPacketConn(inboundSocket),
						handshakeErr: handshakeErr,
					}
					remote := &packetHandshakeRemoteConn{UDPConn: remoteSocket}
					callbackCalls := 0
					var onClose N.CloseHandlerFunc
					if withCallback {
						onClose = func(err error) {
							callbackCalls++
							if !errors.Is(err, handshakeErr) {
								t.Errorf("close cause = %v, want handshake failure", err)
							}
							if inbound.closeCalls != 1 || remote.closeCalls != 1 {
								t.Errorf("callback before socket cleanup: inbound=%d remote=%d", inbound.closeCalls, remote.closeCalls)
							}
						}
					}
					manager := NewConnectionManager(context.Background(), log.NewNOPFactory().Logger())
					manager.NewPacketConnection(context.Background(), &packetHandshakeDialer{remote: remote}, inbound, adapter.InboundContext{
						Network:     N.NetworkUDP,
						Destination: M.SocksaddrFromNet(destination),
						UDPConnect:  connected,
					}, onClose)

					if inbound.handshakeCalls != 1 {
						t.Errorf("handshake calls = %d, want 1", inbound.handshakeCalls)
					}
					if withCallback && callbackCalls != 1 {
						t.Errorf("close callback calls = %d, want 1", callbackCalls)
					}
					if inbound.closeCalls != 1 || remote.closeCalls != 1 {
						t.Errorf("socket close calls: inbound=%d remote=%d, want 1 each", inbound.closeCalls, remote.closeCalls)
					}
					if _, err = inboundSocket.WriteToUDP([]byte{0}, destination); !errors.Is(err, net.ErrClosed) {
						t.Errorf("inbound socket is not closed: %v", err)
					}
					if connected {
						_, err = remoteSocket.Write([]byte{0})
					} else {
						_, err = remoteSocket.WriteToUDP([]byte{0}, destination)
					}
					if !errors.Is(err, net.ErrClosed) {
						t.Errorf("remote socket is not closed: %v", err)
					}
				})
			}
		})
	}
}

type packetHandshakeFailureConn struct {
	N.PacketConn
	handshakeErr   error
	handshakeCalls int
	closeCalls     int
}

func (c *packetHandshakeFailureConn) PacketConnHandshakeSuccess(net.PacketConn) error {
	c.handshakeCalls++
	return c.handshakeErr
}

func (c *packetHandshakeFailureConn) Close() error {
	c.closeCalls++
	return c.PacketConn.Close()
}

type packetHandshakeRemoteConn struct {
	*net.UDPConn
	closeCalls int
}

func (c *packetHandshakeRemoteConn) Close() error {
	c.closeCalls++
	return c.UDPConn.Close()
}

type packetHandshakeDialer struct {
	remote *packetHandshakeRemoteConn
}

func (d *packetHandshakeDialer) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	return d.remote, nil
}

func (d *packetHandshakeDialer) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return d.remote, nil
}
