package route

import (
	"context"
	"io"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/interrupt"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-tun"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func TestFlowCloseDuringAttachDetachesEveryGroup(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	inner := &flowLifecycleGroup{
		selected: &flowLifecycleLeaf{},
		entered:  entered,
		release:  release,
	}
	outer := &flowLifecycleGroup{selected: inner}
	router := &Router{
		logger:   log.NewNOPFactory().NewLogger("test"),
		outbound: &flowLifecycleManager{outbound: outer},
	}
	destination := M.ParseSocksaddr("198.51.100.2:443")
	result := router.preMatchFlow(context.Background(), &adapter.InboundContext{
		Network:     N.NetworkTCP,
		Source:      M.ParseSocksaddr("192.0.2.1:23456"),
		Destination: destination,
	}, destination, nil, "")
	if result.NewTracker == nil {
		t.Fatal("forwarded flow has no lifecycle tracker")
	}
	tracker := result.NewTracker()
	handle := new(flowLifecycleHandle)
	attached := make(chan struct{})
	go func() {
		tracker.AttachFlow(handle)
		close(attached)
	}()
	// The second group has registered the flow, but has not returned its
	// removal callback. Close must account for that late registration too.
	<-entered
	closing := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		close(closing)
		tracker.CloseFlow(tun.FlowCloseReset)
		close(closed)
	}()
	<-closing
	close(release)
	<-attached
	<-closed

	for _, group := range []*flowLifecycleGroup{outer, inner} {
		if calls := group.detaches.Load(); calls != 1 {
			t.Errorf("group removal calls = %d, want 1", calls)
		}
		group.connections.Interrupt(true)
	}
	if calls := handle.closes.Load(); calls != 0 {
		t.Errorf("closed flow retained %d group registrations", calls)
	}
}

type flowLifecycleLeaf struct {
	adapter.FlowOutbound
}

func (*flowLifecycleLeaf) Type() string      { return "direct" }
func (*flowLifecycleLeaf) Tag() string       { return "leaf" }
func (*flowLifecycleLeaf) Network() []string { return []string{N.NetworkTCP} }
func (*flowLifecycleLeaf) PreMatchFlow(string, netip.Addr) adapter.PreMatchAction {
	return adapter.PreMatchFlow
}

type flowLifecycleGroup struct {
	adapter.Outbound
	selected    adapter.Outbound
	connections interrupt.Group
	entered     chan struct{}
	release     chan struct{}
	detaches    atomic.Int32
}

func (*flowLifecycleGroup) Tag() string                        { return "group" }
func (g *flowLifecycleGroup) All() []string                    { return []string{g.selected.Tag()} }
func (g *flowLifecycleGroup) Selected(string) adapter.Outbound { return g.selected }
func (g *flowLifecycleGroup) AttachConnection(closer io.Closer) func() {
	remove := g.connections.Add(closer, true)
	if g.entered != nil {
		close(g.entered)
		<-g.release
	}
	return func() {
		remove()
		g.detaches.Add(1)
	}
}

type flowLifecycleManager struct {
	adapter.OutboundManager
	outbound adapter.Outbound
}

func (m *flowLifecycleManager) Default() adapter.Outbound { return m.outbound }

type flowLifecycleHandle struct {
	closes atomic.Int32
}

func (h *flowLifecycleHandle) CloseFlow() { h.closes.Add(1) }
