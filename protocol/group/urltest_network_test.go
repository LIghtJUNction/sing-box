package group

import (
	"testing"

	"github.com/sagernet/sing-box/adapter"
)

type traceProtocolNode struct {
	adapter.Outbound
	tag string
}

func (n *traceProtocolNode) Tag() string { return n.tag }

func TestURLTestNowForNetwork(t *testing.T) {
	group := &URLTest{group: &URLTestGroup{
		selectedOutboundTCP: &traceProtocolNode{tag: "tcp-node"},
		selectedOutboundUDP: &traceProtocolNode{tag: "udp-node"},
	}}
	for network, want := range map[string]string{"tcp": "tcp-node", "udp": "udp-node", "icmp": ""} {
		t.Run(network, func(t *testing.T) {
			if got := group.NowForNetwork(network); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
	if got := group.Now(); got != "tcp-node" {
		t.Fatalf("legacy group view changed: %q", got)
	}
	group.group.selectedOutboundUDP = nil
	if got := group.NowForNetwork("udp"); got != "" {
		t.Fatalf("missing UDP selection fell back to TCP: %q", got)
	}
	if got := new(URLTest).NowForNetwork("udp"); got != "" {
		t.Fatalf("unstarted group returned %q", got)
	}
}
