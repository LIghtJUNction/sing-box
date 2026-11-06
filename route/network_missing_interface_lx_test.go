package route

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/pause"
)

type missingInterfaceMonitor struct {
	tun.DefaultInterfaceMonitor
	current atomic.Pointer[control.Interface]
}

func (m *missingInterfaceMonitor) DefaultInterface() *control.Interface { return m.current.Load() }
func (m *missingInterfaceMonitor) AndroidVPNEnabled() bool              { return false }

type missingInterfacePlatform struct{ adapter.PlatformInterface }

type missingInterfaceLogger struct {
	log.ContextLogger
	errors int
}

func (l *missingInterfaceLogger) Error(...any) { l.errors++ }

func newMissingInterfaceManager(t *testing.T) (*NetworkManager, *missingInterfaceMonitor) {
	t.Helper()
	ctx, cancel := context.WithCancel(pause.WithDefaultManager(context.Background()))
	t.Cleanup(cancel)
	monitor := &missingInterfaceMonitor{}
	manager := &NetworkManager{
		ctx:              ctx,
		startedCtx:       ctx,
		logger:           log.NewNOPFactory().Logger(),
		pauseManager:     service.FromContext[pause.Manager](ctx),
		interfaceMonitor: monitor,
	}
	return manager, monitor
}

func TestMissingDefaultInterfaceAndroidSystemRouting(t *testing.T) {
	strategy := C.NetworkStrategyDefault
	cases := []struct {
		name    string
		android bool
		modify  func(*NetworkManager)
		paused  bool
	}{
		{name: "android-system-route", android: true},
		{name: "other-platform", paused: true},
		{name: "auto-detect", android: true, paused: true, modify: func(m *NetworkManager) { m.autoDetectInterface = true }},
		{name: "platform-adapter", android: true, paused: true, modify: func(m *NetworkManager) { m.platformInterface = &missingInterfacePlatform{} }},
		{name: "default-interface", android: true, paused: true, modify: func(m *NetworkManager) { m.defaultOptions.BindInterface = "test-egress" }},
		{name: "network-strategy", android: true, paused: true, modify: func(m *NetworkManager) { m.defaultOptions.NetworkStrategy = &strategy }},
		{name: "network-type", android: true, paused: true, modify: func(m *NetworkManager) { m.defaultOptions.NetworkType = []C.InterfaceType{C.InterfaceTypeWIFI} }},
		{name: "fallback-network", android: true, paused: true, modify: func(m *NetworkManager) {
			m.defaultOptions.FallbackNetworkType = []C.InterfaceType{C.InterfaceTypeCellular}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			manager, monitor := newMissingInterfaceManager(t)
			logger := &missingInterfaceLogger{ContextLogger: manager.logger}
			manager.logger = logger
			if test.modify != nil {
				test.modify(manager)
			}
			oldUpdate, cancel := context.WithCancel(manager.ctx)
			manager.interfaceUpdateCancel = cancel
			manager.handleMissingDefaultInterface(test.android)
			if got := manager.pauseManager.IsNetworkPaused(); got != test.paused {
				t.Errorf("network paused = %v; want %v", got, test.paused)
			}
			if oldUpdate.Err() != context.Canceled || manager.interfaceUpdateCancel != nil {
				t.Error("missing interface must cancel and clear the previous update")
			}
			if !manager.networkResetPending {
				t.Error("recovery must retain a pending network reset")
			}
			if monitor.DefaultInterface() != nil || manager.DefaultNetworkInterface() != nil {
				t.Error("system routing must not fabricate interface availability")
			}
			if logger.errors != 0 {
				t.Error("an absent interface must be reported as waiting or system routing")
			}
		})
	}
}

func TestMissingDefaultInterfaceRepeatedLossPreservesDevicePause(t *testing.T) {
	manager, _ := newMissingInterfaceManager(t)
	manager.pauseManager.DevicePause()
	manager.pauseManager.NetworkPause()
	manager.handleMissingDefaultInterface(true)
	manager.handleMissingDefaultInterface(true)
	if manager.pauseManager.IsNetworkPaused() {
		t.Error("optional monitor left Android system routing paused")
	}
	if !manager.pauseManager.IsDevicePaused() || !manager.pauseManager.IsPaused() {
		t.Error("network fallback must preserve an independent device pause")
	}
	if !manager.networkResetPending {
		t.Error("a repeated nil interface lost pending recovery")
	}
}

type missingInterfaceRouter struct {
	adapter.Router
	resets chan struct{}
}

func (r *missingInterfaceRouter) ResetNetwork() { r.resets <- struct{}{} }

type missingInterfaceEndpoints struct{ adapter.EndpointManager }

func (*missingInterfaceEndpoints) Endpoints() []adapter.Endpoint { return nil }

type missingInterfaceInbounds struct{ adapter.InboundManager }

func (*missingInterfaceInbounds) Inbounds() []adapter.Inbound { return nil }

type missingInterfaceOutbounds struct{ adapter.OutboundManager }

func (*missingInterfaceOutbounds) Outbounds() []adapter.Outbound { return nil }

func TestMissingDefaultInterfaceRecoveryCallback(t *testing.T) {
	manager, monitor := newMissingInterfaceManager(t)
	router := &missingInterfaceRouter{resets: make(chan struct{}, 4)}
	manager.router = router
	manager.endpoint = &missingInterfaceEndpoints{}
	manager.inbound = &missingInterfaceInbounds{}
	manager.outbound = &missingInterfaceOutbounds{}
	// Supplying synthetic gateway/Wi-Fi evidence keeps the real recovery path
	// off host netlink and neighbour tables.
	manager.wifiState = adapter.WIFIState{SSID: "synthetic-test-network"}
	interfaces := []adapter.NetworkInterface{
		{Interface: control.Interface{Index: 9001, Name: "test-wifi", Flags: net.FlagUp}, Gateways: []netip.Addr{netip.MustParseAddr("192.0.2.1")}},
		{Interface: control.Interface{Index: 9002, Name: "test-cellular", Flags: net.FlagUp}, Gateways: []netip.Addr{netip.MustParseAddr("198.51.100.1")}},
	}
	manager.networkInterfaces.Store(interfaces)

	for _, current := range interfaces {
		monitor.current.Store(nil)
		manager.notifyInterfaceUpdate(nil, 0)
		manager.notifyInterfaceUpdate(nil, 0)
		manager.interfaceUpdateAccess.Lock()
		pending := manager.networkResetPending
		manager.interfaceUpdateAccess.Unlock()
		if !pending {
			t.Fatal("repeated loss cleared pending recovery")
		}
		monitor.current.Store(&current.Interface)
		manager.notifyInterfaceUpdate(&current.Interface, 0)
		select {
		case <-router.resets:
		case <-time.After(3 * time.Second):
			t.Fatal("interface recovery did not reset the network")
		}
		manager.resetRunAccess.Lock()
		manager.resetRunAccess.Unlock()
		if manager.pauseManager.IsNetworkPaused() {
			t.Fatal("interface recovery did not wake the network")
		}
		manager.interfaceUpdateAccess.Lock()
		pending = manager.networkResetPending
		manager.interfaceUpdateAccess.Unlock()
		if pending {
			t.Fatal("successful recovery did not consume pending reset")
		}
		select {
		case <-router.resets:
			t.Fatal("one recovery emitted more than one reset")
		default:
		}
	}
}
