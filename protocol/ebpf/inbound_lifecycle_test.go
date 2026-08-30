//go:build with_ebpf && (linux || android)

package ebpf

import (
	"net/netip"
	"testing"
	"time"

	ECommon "github.com/sagernet/sing-box/common/ebpf"
	"github.com/sagernet/sing-box/option"
	udpnat "github.com/sagernet/sing/common/udpnat2"
)

func TestTakeCgroupBackendRetiresCleanupAccess(t *testing.T) {
	backend := &ECommon.CgroupBackend{}
	inbound := &Inbound{}
	inbound.listeners.port = 5300
	inbound.setCgroupBackend(backend)
	if retired := inbound.takeCgroupBackend(); retired != backend {
		t.Fatal("unexpected retired cgroup backend")
	}
	if current := inbound.cgroupBackendInstance(); current != nil {
		t.Fatal("retired cgroup backend remained published")
	}
	inbound.deleteUDPRedirectsWithBackend(backend, []netip.Addr{netip.MustParseAddr("127.128.0.1")})
}

func TestCloseResourcesKeepsSocketProtectionUntilTCDetach(t *testing.T) {
	attachment, detachCalls := newRetryingTestTCAttachment(t, "sing-box-inbound-retry", 424243)
	manager := &sharedTCManager{
		attachments: map[string]*sharedTCAttachment{"sing-box-inbound-retry": attachment},
	}
	inbound := &Inbound{udpTimeout: time.Minute}
	inbound.udpNat = udpnat.New(inbound, inbound.preparePacketConnection, inbound.udpTimeout, false)
	inbound.socketProtector = newSocketProtector()
	protector := inbound.socketProtector
	inbound.sharedNetwork = newSharedNetwork(inbound, option.EBPFSharedOptions{})
	inbound.sharedNetwork.tcManager = manager

	if err := inbound.Close(); err == nil {
		t.Fatal("expected the first TC manager close to fail")
	}
	if protector.closed {
		t.Fatal("socket protection closed while TC interception remained attached")
	}
	if err := inbound.Close(); err != nil {
		t.Fatal("retry inbound close: ", err)
	}
	if !protector.closed {
		t.Fatal("socket protection remained open after interception detached")
	}
	if *detachCalls != 2 {
		t.Fatalf("unexpected TC detach attempts: %d", *detachCalls)
	}
}

func TestTakeSharedBackend(t *testing.T) {
	backend := &ECommon.SharedNetworkBackend{}
	shared := &sharedNetwork{}
	shared.setSharedBackend(backend)
	if retired := shared.takeSharedBackend(); retired != backend {
		t.Fatal("unexpected retired shared-network backend")
	}
	if current := shared.sharedBackendInstance(); current != nil {
		t.Fatal("retired shared-network backend remained published")
	}
}
