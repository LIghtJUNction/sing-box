package route

import (
	"context"
	"io"
	"net/netip"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
)

type groupFlowLeaf struct {
	adapter.FlowOutbound
	probes int
}

func (*groupFlowLeaf) Tag() string       { return "flow-leaf" }
func (*groupFlowLeaf) Network() []string { return []string{"tcp"} }
func (o *groupFlowLeaf) PreMatchFlow(string, netip.Addr) adapter.PreMatchAction {
	o.probes++
	return adapter.PreMatchFlow
}

type groupFlowPolicy struct {
	adapter.Outbound
	leaf    adapter.Outbound
	managed bool
}

func (*groupFlowPolicy) Tag() string                        { return "policy-group" }
func (*groupFlowPolicy) All() []string                      { return []string{"flow-leaf"} }
func (g *groupFlowPolicy) Selected(string) adapter.Outbound { return g.leaf }
func (*groupFlowPolicy) AttachConnection(io.Closer) func()  { return func() {} }
func (g *groupFlowPolicy) RequiresGroupDialer() bool        { return g.managed }

type groupFlowManager struct {
	adapter.OutboundManager
	defaultOutbound adapter.Outbound
}

func (m *groupFlowManager) Default() adapter.Outbound { return m.defaultOutbound }

func TestFlowShortcutPreservesGroupConnectionPolicy(t *testing.T) {
	for _, managed := range []bool{true, false} {
		t.Run(map[bool]string{true: "managed group", false: "ordinary group"}[managed], func(t *testing.T) {
			leaf := &groupFlowLeaf{}
			group := &groupFlowPolicy{leaf: leaf, managed: managed}
			router := &Router{outbound: &groupFlowManager{defaultOutbound: group}}
			destination := M.ParseSocksaddr("198.51.100.2:443")
			result := router.preMatchFlow(context.Background(), &adapter.InboundContext{
				Network: "tcp", Destination: destination,
			}, destination, nil, "")
			if managed {
				if result.Action != adapter.PreMatchContinue || leaf.probes != 0 {
					t.Fatalf("flow shortcut bypassed connection policy: action=%v probes=%d", result.Action, leaf.probes)
				}
			} else if result.Action != adapter.PreMatchFlow || leaf.probes != 1 {
				t.Fatalf("ordinary group lost flow acceleration: action=%v probes=%d", result.Action, leaf.probes)
			}
		})
	}
}
