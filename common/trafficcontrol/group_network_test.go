package trafficcontrol

import "testing"

type legacyTraceGroup struct{}

func (legacyTraceGroup) Now() string { return "manual-node" }

type splitTraceGroup struct{ legacyTraceGroup }

func (splitTraceGroup) NowForNetwork(network string) string {
	switch network {
	case "tcp":
		return "tcp-node"
	case "udp":
		return "udp-node"
	default:
		return ""
	}
}

func TestGroupTagForNetwork(t *testing.T) {
	for _, test := range []struct {
		name    string
		group   interface{ Now() string }
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
