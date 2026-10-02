package wireguard

// lx:begin SPEC 052 netstack connect deadline

import (
	"context"
	"errors"
	"testing"
	"time"

	"net/netip"
	"sync/atomic"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
)

// TestConnectContextLx_budget pins the deadline contract: a bare parent gets
// the probe-aligned C.TCPTimeout budget (SPEC 052 §S5: user deadline must not
// be below the probe budget), and a parent with an EARLIER deadline keeps it —
// the wrapper must never extend.
func TestConnectContextLx_budget(t *testing.T) {
	ctx, cancel := connectContextLx(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("connect ctx must carry a deadline")
	}
	if remaining := time.Until(deadline); remaining > C.TCPTimeout || remaining < C.TCPTimeout-time.Second {
		t.Fatalf("budget must be ~C.TCPTimeout (%v), got %v", C.TCPTimeout, remaining)
	}

	parent, parentCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer parentCancel()
	child, childCancel := connectContextLx(parent)
	defer childCancel()
	childDeadline, _ := child.Deadline()
	if time.Until(childDeadline) > 100*time.Millisecond {
		t.Fatal("an earlier parent deadline must win — the wrapper must never extend")
	}
}

// The native Go stack connect must honor the scoped deadline even when every
// SYN is silently discarded. This exercises the actual stackDevice dial path.
func TestStackDeviceDeadlineCutsBlackholeConnect(t *testing.T) {
	memoryTun := tun.NewMemoryTun(tun.MemoryTunOptions{
		MTU:      1500,
		Outbound: func(packets []*buf.Buffer) { buf.ReleaseMulti(packets) },
	})
	s, err := tun.NewGo(tun.StackOptions{
		Context:    context.Background(),
		Tun:        memoryTun,
		TunOptions: tun.Options{MTU: 1500},
		Logger:     logger.NOP(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	defer memoryTun.Close()
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	w := &stackDevice{
		stack:        s,
		activeTCP:    new(atomic.Int64),
		inet4Address: netip.MustParseAddr("10.99.0.2"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = w.DialContext(ctx, "tcp", M.ParseSocksaddr("203.0.113.5:80"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the failure must be the ctx deadline, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the deadline must cut the connect promptly, took %v", elapsed)
	}
	if live := w.CurrentEstablished(); live != 0 {
		t.Fatalf("a failed connect must not count as live, got %d", live)
	}
}
