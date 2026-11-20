package trafficcontrol

import (
	"io"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/adapter"
)

type trackerNode struct {
	adapter.Outbound
	tag  string
	deps []string
}

func (n *trackerNode) Tag() string            { return n.tag }
func (n *trackerNode) Type() string           { return "test" }
func (n *trackerNode) Dependencies() []string { return n.deps }

type trackerGroup struct {
	trackerNode
	tcp, udp adapter.Outbound
}

func (g *trackerGroup) All() []string { return nil }
func (g *trackerGroup) Selected(network string) adapter.Outbound {
	if network == "udp" {
		return g.udp
	}
	return g.tcp
}
func (g *trackerGroup) AttachConnection(io.Closer) func() { return func() {} }

type trackerOutboundManager struct {
	adapter.OutboundManager
	nodes map[string]adapter.Outbound
}

func (m *trackerOutboundManager) Outbound(tag string) (adapter.Outbound, bool) {
	node, loaded := m.nodes[tag]
	return node, loaded
}

func TestTrackerUsesRouteSnapshotAndNetworkSpecificDetour(t *testing.T) {
	leaf := &trackerNode{tag: "routed-node", deps: []string{"transport-group"}}
	changed := &trackerNode{tag: "changed-node"}
	transportTCP := &trackerNode{tag: "transport-tcp"}
	transportUDP := &trackerNode{tag: "transport-udp"}
	routingGroup := &trackerGroup{trackerNode: trackerNode{tag: "routing-group"}, tcp: changed, udp: changed}
	transportGroup := &trackerGroup{trackerNode: trackerNode{tag: "transport-group"}, tcp: transportTCP, udp: transportUDP}
	manager := &Manager{outbound: &trackerOutboundManager{nodes: map[string]adapter.Outbound{
		"routing-group":   routingGroup,
		"routed-node":     leaf,
		"changed-node":    changed,
		"transport-group": transportGroup,
		"transport-tcp":   transportTCP,
		"transport-udp":   transportUDP,
	}}}
	metadata := manager.newTrackerMetadata(adapter.InboundContext{
		Network:       "udp",
		OutboundChain: []adapter.Outbound{routingGroup, leaf},
	}, nil, routingGroup, new(atomic.Int64), new(atomic.Int64))
	if want := []string{"routed-node", "routing-group"}; !reflect.DeepEqual(metadata.Chain, want) {
		t.Fatalf("routing snapshot = %v, want %v", metadata.Chain, want)
	}
	if metadata.Outbound != leaf.Tag() {
		t.Fatalf("routed outbound = %q, want %q", metadata.Outbound, leaf.Tag())
	}
	if want := []string{"transport-group", "transport-udp"}; !reflect.DeepEqual(metadata.Detour, want) {
		t.Fatalf("UDP detour = %v, want %v", metadata.Detour, want)
	}
}
