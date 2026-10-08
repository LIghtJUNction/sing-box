package group

import (
	"context"
	"time"

	"github.com/sagernet/sing/service/pause"
)

// lazyActiveLocked reports whether automatic testing is currently allowed.
// The ticker is the existing activity lease; read-only selection and manual
// URL tests deliberately do not acquire that lease. lx: SPEC 019.
func (g *URLTestGroup) lazyActiveLocked() bool {
	return g.started && g.ticker != nil && g.ctx.Err() == nil && !(g.pause.IsDevicePaused() || g.pause.IsNetworkPaused())
}

func (g *URLTestGroup) lazyActive() bool {
	g.access.Lock()
	defer g.access.Unlock()
	return g.lazyActiveLocked()
}

func (g *URLTestGroup) touchLazy() {
	g.access.Lock()
	defer g.access.Unlock()
	if !g.started || g.ctx.Err() != nil {
		return
	}
	// Stamp even the first touch: PostStart may have happened hours ago.
	g.lastActive.Store(time.Now())
	if g.pause.IsDevicePaused() || g.pause.IsNetworkPaused() {
		return
	}
	if g.ticker != nil {
		// A pause can race the first-test goroutine. The first effective touch
		// after waking retries that pending check without waiting an interval.
		if g.lazyInitialCheck && !g.lazyInitialScheduled {
			g.lazyInitialScheduled = true
			go g.checkLazyInitial(g.ticker)
		}
		return
	}
	ticker := time.NewTicker(g.interval)
	ticker.Stop()
	g.ticker = ticker
	g.lazyInitialCheck = true
	g.lazyInitialScheduled = true
	// Use atomic pause flags, and keep callbacks independent of g.access:
	// pause emits while holding its own lock, while Close unregisters under ours.
	g.pauseCallback = g.pause.RegisterCallback(func(event int) {
		switch event {
		case pause.EventDevicePaused, pause.EventNetworkPause:
			ticker.Stop()
		case pause.EventDeviceWake, pause.EventNetworkWake:
			if !g.pause.IsDevicePaused() && !g.pause.IsNetworkPaused() {
				ticker.Reset(g.interval)
			}
		}
	})
	// Start after registration to cover a wake that happened before registration.
	// A new pause racing Reset is harmless: the loop also checks both pause flags.
	if !g.pause.IsDevicePaused() && !g.pause.IsNetworkPaused() {
		ticker.Reset(g.interval)
	}
	go g.loopLazyCheck(ticker)
}

func (g *URLTestGroup) checkLazyInitial(ticker *time.Ticker) {
	g.access.Lock()
	if g.ticker != ticker {
		g.access.Unlock()
		return
	}
	g.lazyInitialScheduled = false
	if !g.lazyActiveLocked() || !g.lazyInitialCheck {
		g.access.Unlock()
		return
	}
	g.lazyInitialCheck = false
	g.access.Unlock()
	g.CheckOutbounds(g.ctx, false)
}

func (g *URLTestGroup) loopLazyCheck(ticker *time.Ticker) {
	g.checkLazyInitial(ticker)
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-g.close:
			return
		case <-ticker.C:
		}
		g.access.Lock()
		if !g.started || g.ticker != ticker || g.ctx.Err() != nil {
			g.access.Unlock()
			return
		}
		// Recheck activity while owning the ticker: a simultaneous Touch must
		// not lose a renewed lease to an earlier idle observation.
		if time.Since(g.lastActive.Load()) > g.idleTimeout {
			g.stopLazyTickerLocked()
			g.access.Unlock()
			return
		}
		if g.pause.IsDevicePaused() || g.pause.IsNetworkPaused() {
			g.access.Unlock()
			continue
		}
		g.lazyInitialCheck = false
		g.lazyInitialScheduled = false
		g.access.Unlock()
		if g.reachability != nil && g.groupTag != "" && !g.reachability.OutboundReachable(g.groupTag) {
			continue
		}
		g.CheckOutbounds(g.ctx, false)
	}
}

func (g *URLTestGroup) stopLazyTickerLocked() {
	ticker := g.ticker
	ticker.Stop()
	g.ticker = nil
	g.lazyInitialCheck, g.lazyInitialScheduled = false, false
	g.pause.UnregisterCallback(g.pauseCallback)
	g.pauseCallback = nil
	ticker.Stop() // A wake before callback removal may have reset it.
}

func (g *URLTestGroup) checkLazyInterface(ctx context.Context) {
	if ctx.Err() != nil || !g.lazyActive() {
		return
	}
	// A reset caller has its own deadline, but group.Close must also cancel a
	// probe that began with that caller's otherwise still-live context.
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(g.ctx, cancel)
	defer stop()
	defer cancel()
	g.CheckOutbounds(ctx, true)
}
