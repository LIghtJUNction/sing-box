//go:build with_gvisor

package tun

import (
	"context"
	"net/netip"
	"testing"

	"github.com/sagernet/sing-box/log"
	tunlib "github.com/sagernet/sing-tun"
)

// MagicNet's existing configuration explicitly selects mixed. Updating the
// native Go stacks used by endpoints must not remove this legacy TUN stack.
func TestLegacyMixedStackConstructs(t *testing.T) {
	stack, err := tunlib.NewStack("mixed", tunlib.StackOptions{
		Context: context.Background(),
		Tun:     &mixedTestTun{},
		TunOptions: tunlib.Options{
			Name:         "magicnet0",
			MTU:          1500,
			Inet4Address: []netip.Prefix{netip.MustParsePrefix("172.19.0.1/30")},
		},
		Logger: log.NewNOPFactory().Logger(),
	})
	if err != nil {
		t.Fatalf("construct existing MagicNet mixed TUN stack: %v", err)
	}
	if _, ok := stack.(*tunlib.Mixed); !ok {
		t.Fatalf("mixed selected %T", stack)
	}
	if err := stack.Close(); err != nil {
		t.Fatal(err)
	}
}

// The constructor requires the same capabilities as the real Linux/Android
// TUN device, but does not open the device or start packet processing.
type mixedTestTun struct{ tunlib.GVisorTun }
