package trafficcontrol

import (
	"testing"

	"github.com/sagernet/sing-box/adapter"
)

type legacyTraceGroup struct{}

type traceNode struct {
	adapter.Outbound
	tag string
}

func (n *traceNode) Tag() string                          { return n.tag }
func (legacyTraceGroup) Selected(string) adapter.Outbound { return &traceNode{tag: "manual-node"} }

type splitTraceGroup struct{}

func (splitTraceGroup) Selected(network string) adapter.Outbound {
	switch network {
	case "tcp":
		return &traceNode{tag: "tcp-node"}
	case "udp":
		return &traceNode{tag: "udp-node"}
	default:
		return nil
	}
}

func TestGroupTagForNetwork(t *testing.T) {
	for _, test := range []struct {
		name    string
		group   interface{ Selected(string) adapter.Outbound }
		network string
		want    string
	}{
		{"manual TCP", legacyTraceGroup{}, "tcp", "manual-node"},
		{"manual UDP", legacyTraceGroup{}, "udp", "manual-node"},
		{"automatic TCP", splitTraceGroup{}, "tcp", "tcp-node"},
		{"automatic UDP", splitTraceGroup{}, "udp", "udp-node"},
		{"unsupported network", splitTraceGroup{}, "icmp", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := groupTagForNetwork(test.group, test.network); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
