package direct

import (
	"context"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/sing-box/common/dialer"
	C "github.com/sagernet/sing-box/constant"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func TestTFOResolvedAddressesKeepCallerContext(t *testing.T) {
	h := &Outbound{
		dialer:      lazyContextDialer{},
		tcpFastOpen: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := h.dialResolvedNetwork(
		ctx,
		N.NetworkTCP,
		M.Socksaddr{Fqdn: "example.com", Port: 443},
		[]netip.Addr{
			netip.MustParseAddr("192.0.2.1"),
			netip.MustParseAddr("2001:db8::1"),
		},
		nil,
		nil,
		nil,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err = conn.Write([]byte("hello")); err != nil {
		t.Fatalf("lazy TFO connection lost its caller context before first write: %v", err)
	}
}

type lazyContextDialer struct{}

var _ dialer.ParallelInterfaceDialer = lazyContextDialer{}

func (lazyContextDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return &lazyContextConn{ctx: ctx}, nil
}

func (lazyContextDialer) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, io.ErrClosedPipe
}

func (d lazyContextDialer) DialParallelInterface(ctx context.Context, network string, destination M.Socksaddr, _ *C.NetworkStrategy, _ []C.InterfaceType, _ []C.InterfaceType, _ time.Duration) (net.Conn, error) {
	return d.DialContext(ctx, network, destination)
}

func (lazyContextDialer) ListenSerialInterfacePacket(context.Context, M.Socksaddr, *C.NetworkStrategy, []C.InterfaceType, []C.InterfaceType, time.Duration) (net.PacketConn, error) {
	return nil, io.ErrClosedPipe
}

type lazyContextConn struct {
	ctx context.Context
}

func (c *lazyContextConn) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (c *lazyContextConn) Write(payload []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (*lazyContextConn) Close() error {
	return nil
}

func (*lazyContextConn) LocalAddr() net.Addr {
	return lazyContextAddr("local")
}

func (*lazyContextConn) RemoteAddr() net.Addr {
	return lazyContextAddr("remote")
}

func (*lazyContextConn) SetDeadline(time.Time) error {
	return nil
}

func (*lazyContextConn) SetReadDeadline(time.Time) error {
	return nil
}

func (*lazyContextConn) SetWriteDeadline(time.Time) error {
	return nil
}

type lazyContextAddr string

func (lazyContextAddr) Network() string {
	return "test"
}

func (a lazyContextAddr) String() string {
	return string(a)
}
