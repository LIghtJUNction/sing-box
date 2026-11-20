package wireguard

import (
	"context"
	"testing"

	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/pause"
)

func TestApplyPauseEventWithoutManager(t *testing.T) {
	e := &Endpoint{}
	e.pauseOpAccess.Lock()
	defer e.pauseOpAccess.Unlock()
	e.applyPauseEvent(pause.EventNetworkPause)
	e.applyPauseEvent(pause.EventNetworkWake)
}

func TestApplyPauseEventUsesCurrentStateWithoutDevice(t *testing.T) {
	manager := service.FromContext[pause.Manager](pause.WithDefaultManager(context.Background()))
	e := &Endpoint{pause: manager}
	e.suspended.Store(true)
	e.idle.Store(true)
	e.pauseOpAccess.Lock()
	defer e.pauseOpAccess.Unlock()

	manager.NetworkPause()
	e.applyPauseEvent(pause.EventNetworkWake)
	if !e.networkPaused {
		t.Fatal("stale wake event overrode the manager's current paused state")
	}
	manager.NetworkWake()
	e.applyPauseEvent(pause.EventNetworkPause)
	if e.networkPaused {
		t.Fatal("stale pause event overrode the manager's current awake state")
	}
	if !e.suspended.Load() || !e.idle.Load() {
		t.Fatal("network events must preserve protocol suspension and on-demand idle policy")
	}
}
