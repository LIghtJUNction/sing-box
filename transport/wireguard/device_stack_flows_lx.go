package wireguard

import (
	"context"
	"net"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing-tun"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// trackTCPConn keeps a conservative live-flow count without relying on the
// removed gVisor statistics API. Both routed and dialed connections count until
// close; duplicate close/callback paths release the same lease exactly once.
type trackedTCPConn struct {
	net.Conn
	active *atomic.Int64
	once   sync.Once
}

func trackTCPConn(conn net.Conn, active *atomic.Int64) *trackedTCPConn {
	active.Add(1)
	return &trackedTCPConn{Conn: conn, active: active}
}

func (c *trackedTCPConn) release() {
	c.once.Do(func() { c.active.Add(-1) })
}

func (c *trackedTCPConn) Close() error {
	defer c.release()
	return c.Conn.Close()
}

// Copy/splice discovery may unwrap this counting wrapper while the original
// connection still owns Close, preserving the native stack's optimized path.
func (c *trackedTCPConn) ReaderReplaceable() bool { return true }
func (c *trackedTCPConn) WriterReplaceable() bool { return true }
func (c *trackedTCPConn) Upstream() any           { return c.Conn }

type stackFlowHandler struct {
	tun.Handler
	active *atomic.Int64
}

func (h *stackFlowHandler) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	tracked := trackTCPConn(conn, h.active)
	h.Handler.NewConnectionEx(ctx, tracked, source, destination, func(err error) {
		tracked.release()
		if onClose != nil {
			onClose(err)
		}
	})
}
