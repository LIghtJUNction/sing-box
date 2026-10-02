package wireguard

import (
	"context"
	"net"

	"github.com/sagernet/sing-tun"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTrackedTCPConnectionReleasesExactlyOnce(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	active := new(atomic.Int64)
	conn := trackTCPConn(local, active)
	if active.Load() != 1 {
		t.Fatal("a live TCP connection must block idle suspension")
	}
	var done sync.WaitGroup
	for range 8 {
		done.Add(1)
		go func() {
			defer done.Done()
			_ = conn.Close()
			conn.release() // routed onClose can race with the actual Close
		}()
	}
	done.Wait()
	if active.Load() != 0 {
		t.Fatalf("closed connection must release one lease, got %d", active.Load())
	}
}

type routedFlowReceiver struct {
	tun.Handler
	conn    net.Conn
	onClose N.CloseHandlerFunc
}

func (h *routedFlowReceiver) NewConnectionEx(_ context.Context, conn net.Conn, _, _ M.Socksaddr, onClose N.CloseHandlerFunc) {
	h.conn, h.onClose = conn, onClose
}

func TestRoutedTCPConnectionBlocksIdleUntilClose(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	active := new(atomic.Int64)
	receiver := &routedFlowReceiver{}
	handler := &stackFlowHandler{Handler: receiver, active: active}
	var callbackCalls int
	handler.NewConnectionEx(context.Background(), local, M.Socksaddr{}, M.Socksaddr{}, func(error) { callbackCalls++ })
	if active.Load() != 1 {
		t.Fatal("a routed TCP stream must block idle suspension")
	}
	receiver.onClose(nil)
	_ = receiver.conn.Close()
	if active.Load() != 0 || callbackCalls != 1 {
		t.Fatalf("routed close must release once and preserve callback: active=%d callbacks=%d", active.Load(), callbackCalls)
	}
}
