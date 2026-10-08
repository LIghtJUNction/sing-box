package group

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/x/list"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/pause"
)

// The sentinel answers HEAD over net.Pipe: all counts exercise the real URL
// test path without DNS, a listener, or any external network requests.
type lazyProbeNode struct {
	adapter.Outbound
	tag     string
	probes  atomic.Int32
	block   atomic.Bool
	entered chan struct{}
}

func (n *lazyProbeNode) Type() string           { return C.TypeDirect }
func (n *lazyProbeNode) Tag() string            { return n.tag }
func (n *lazyProbeNode) Network() []string      { return []string{N.NetworkTCP, N.NetworkUDP} }
func (n *lazyProbeNode) Dependencies() []string { return nil }
func (n *lazyProbeNode) DialContext(ctx context.Context, _ string, _ M.Socksaddr) (net.Conn, error) {
	n.probes.Add(1)
	if n.block.Load() {
		select {
		case n.entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		request, err := http.ReadRequest(bufio.NewReader(server))
		if err == nil {
			_ = request.Body.Close()
			_, _ = io.WriteString(server, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		}
	}()
	return client, nil
}

func newLazyProbeGroups(t *testing.T, lazy bool, groups, members int, mode string, interval, idle time.Duration) ([]*URLTest, []*lazyProbeNode, pause.Manager) {
	t.Helper()
	nodes := make([]*lazyProbeNode, members)
	tags := make([]string, members)
	manager := &fakeManager{byTag: make(map[string]adapter.Outbound)}
	for i := range nodes {
		tag := fmt.Sprintf("sentinel-%d", i)
		nodes[i] = &lazyProbeNode{tag: tag, entered: make(chan struct{}, 16)}
		tags[i] = tag
		manager.byTag[tag] = nodes[i]
	}
	ctx := service.ContextWith[adapter.OutboundManager](context.Background(), manager)
	ctx = service.ContextWithPtr(ctx, urltest.NewHistoryStorage())
	ctx = pause.WithDefaultManager(ctx)
	result := make([]*URLTest, groups)
	for i := range result {
		options := option.URLTestOutboundOptions{Outbounds: tags, URL: "http://probe.invalid/generate_204", LazyStart: lazy, Mode: mode}
		outbound, err := NewURLTest(ctx, nil, log.NewNOPFactory().NewLogger("lazy-test"), fmt.Sprintf("group-%d", i), options)
		if err != nil {
			t.Fatal(err)
		}
		group := outbound.(*URLTest)
		group.interval, group.idleTimeout = interval, idle
		scope := newSelectorScope(t, ctx)
		if err := group.Start(adapter.StartStateStart, scope); err != nil {
			t.Fatal(err)
		}
		if err := group.Start(adapter.StartStateStarted, scope); err != nil {
			t.Fatal(err)
		}
		result[i] = group
	}
	return result, nodes, service.FromContext[pause.Manager](ctx)
}

func lazyProbeCount(nodes []*lazyProbeNode) int {
	var count int
	for _, node := range nodes {
		count += int(node.probes.Load())
	}
	return count
}

func waitLazy(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func lazyChecksIdle(groups []*URLTest) bool {
	for _, group := range groups {
		if group.group.checking.Load() {
			return false
		}
	}
	return true
}

func assertLazyQuiet(t *testing.T, nodes []*lazyProbeNode, count int) {
	t.Helper()
	time.Sleep(30 * time.Millisecond)
	if got := lazyProbeCount(nodes); got != count {
		t.Fatalf("unexpected automatic probes: got %d, want %d", got, count)
	}
}

func TestURLTestLazyStartProbeCounts(t *testing.T) {
	for _, lazy := range []bool{false, true} {
		t.Run(fmt.Sprintf("lazy=%t", lazy), func(t *testing.T) {
			groups, nodes, _ := newLazyProbeGroups(t, lazy, 5, 27, "", time.Minute, time.Hour)
			want := 135
			if lazy {
				want = 0
			} else {
				waitLazy(t, "135 startup probes", func() bool { return lazyProbeCount(nodes) == want && lazyChecksIdle(groups) })
			}
			assertLazyQuiet(t, nodes, want)
			t.Logf("startup: lazy_start=%t groups=5 members=27 probes=%d", lazy, lazyProbeCount(nodes))
			startupCount := lazyProbeCount(nodes)
			for _, group := range groups {
				group.InterfaceUpdated(context.Background())
			}
			if !lazy {
				want += 135
				waitLazy(t, "135 network-reset probes", func() bool { return lazyProbeCount(nodes) == want && lazyChecksIdle(groups) })
			}
			assertLazyQuiet(t, nodes, want)
			t.Logf("network reset: lazy_start=%t additional_probes=%d", lazy, lazyProbeCount(nodes)-startupCount)
		})
	}
}

func TestURLTestLazyStartColdReadsAndFirstTouch(t *testing.T) {
	for _, mode := range []string{"", C.URLTestModeRoundRobin} {
		t.Run(mode, func(t *testing.T) {
			groups, nodes, _ := newLazyProbeGroups(t, true, 1, 3, mode, time.Minute, time.Hour)
			s := groups[0]
			if s.Now() != nodes[0].Tag() && mode == "" {
				t.Fatal("cold least-test fallback changed")
			}
			if s.Selected(N.NetworkTCP) == nil || len(s.References()) == 0 {
				t.Fatal("cold group is not routable")
			}
			_ = s.Pool()
			assertLazyQuiet(t, nodes, 0)
			if s.group.lazyActive() {
				t.Fatal("read-only query activated the ticker")
			}
			// An old PostStart baseline must not instantly retire the first lease.
			s.group.lastActive.Store(time.Now().Add(-2 * time.Hour))
			var touches sync.WaitGroup
			for i := 0; i < 20; i++ {
				touches.Go(s.group.Touch)
			}
			touches.Wait()
			waitLazy(t, "immediate first touch", func() bool { return lazyProbeCount(nodes) == 3 && lazyChecksIdle(groups) })
			if !s.group.lazyActive() || time.Since(s.group.lastActive.Load()) > time.Second {
				t.Fatal("first touch did not renew the activity lease")
			}
			assertLazyQuiet(t, nodes, 3)
			s.InterfaceUpdated(context.Background())
			waitLazy(t, "active network reset", func() bool { return lazyProbeCount(nodes) == 6 && lazyChecksIdle(groups) })
		})
	}
}

func TestURLTestLazyStartManualForceUnused(t *testing.T) {
	groups, nodes, _ := newLazyProbeGroups(t, true, 1, 3, "", time.Minute, time.Hour)
	for i := 1; i <= 2; i++ {
		result, err := groups[0].URLTest(context.Background())
		if err != nil || len(result) != 3 || lazyProbeCount(nodes) != i*3 {
			t.Fatalf("forced manual run %d: count=%d result=%d error=%v", i, lazyProbeCount(nodes), len(result), err)
		}
	}
	if groups[0].group.lazyActive() {
		t.Fatal("manual test activated background testing")
	}
	groups[0].InterfaceUpdated(context.Background())
	assertLazyQuiet(t, nodes, 6)
}

func TestURLTestLazyStartPauseAndIdle(t *testing.T) {
	groups, nodes, manager := newLazyProbeGroups(t, true, 1, 1, "", 20*time.Millisecond, 60*time.Millisecond)
	s := groups[0]
	manager.DevicePause()
	manager.NetworkPause()
	s.group.Touch()
	s.InterfaceUpdated(context.Background())
	assertLazyQuiet(t, nodes, 0)
	manager.NetworkWake()
	s.group.Touch()
	assertLazyQuiet(t, nodes, 0)
	manager.DeviceWake()
	s.group.Touch()
	waitLazy(t, "awake first touch", func() bool { return lazyProbeCount(nodes) > 0 && lazyChecksIdle(groups) })
	waitLazy(t, "idle ticker retirement", func() bool { return !s.group.lazyActive() })
	count := lazyProbeCount(nodes)
	s.InterfaceUpdated(context.Background())
	assertLazyQuiet(t, nodes, count)
	s.group.history.DeleteURLTestHistory(nodes[0].Tag())
	s.group.Touch()
	waitLazy(t, "idle restart first touch", func() bool { return lazyProbeCount(nodes) > count && lazyChecksIdle(groups) })
}

func TestURLTestLazyStartCloseCancelsInitialAndReset(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprintf("reset=%t", reset), func(t *testing.T) {
			groups, nodes, _ := newLazyProbeGroups(t, true, 1, 1, "", time.Minute, time.Hour)
			s, node := groups[0], nodes[0]
			if reset {
				s.group.Touch()
				waitLazy(t, "initial run", func() bool { return node.probes.Load() == 1 && !s.group.checking.Load() })
			}
			node.block.Store(true)
			if reset {
				s.InterfaceUpdated(context.Background())
			} else {
				s.group.Touch()
			}
			select {
			case <-node.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("probe did not block")
			}
			_ = s.group.Close()
			_ = s.group.Close()
			waitLazy(t, "Close-owned probe cancellation", func() bool { return !s.group.checking.Load() })
			s.group.Touch()
			if s.group.lazyActive() {
				t.Fatal("closed group was resurrected")
			}
		})
	}
}

func TestURLTestLazyStartQueuedResetRechecks(t *testing.T) {
	for _, closeGroup := range []bool{false, true} {
		t.Run(fmt.Sprintf("close=%t", closeGroup), func(t *testing.T) {
			groups, nodes, manager := newLazyProbeGroups(t, true, 1, 1, "", time.Minute, time.Hour)
			s := groups[0]
			s.group.Touch()
			waitLazy(t, "initial run", func() bool { return lazyProbeCount(nodes) == 1 && lazyChecksIdle(groups) })
			s.checkAccess.Lock()
			s.InterfaceUpdated(context.Background())
			if closeGroup {
				_ = s.group.Close()
			} else {
				manager.DevicePause()
			}
			s.checkAccess.Unlock()
			assertLazyQuiet(t, nodes, 1)
		})
	}
}

func TestURLTestLazyStartOptionDefault(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var options option.URLTestOutboundOptions
		data := `{}`
		if enabled {
			data = `{"lazy_start":true}`
		}
		if err := json.Unmarshal([]byte(data), &options); err != nil || options.LazyStart != enabled {
			t.Fatalf("option default/wire format: %v %#v", err, options)
		}
	}
}

func TestURLTestLazyStartAttachActivates(t *testing.T) {
	groups, nodes, _ := newLazyProbeGroups(t, true, 1, 1, "", time.Minute, time.Hour)
	remove := groups[0].AttachConnection(io.NopCloser(strings.NewReader("")))
	defer remove()
	waitLazy(t, "attachment first touch", func() bool { return lazyProbeCount(nodes) == 1 && lazyChecksIdle(groups) })
}

func TestURLTestLazyStartPendingPauseRetainsFirstCheck(t *testing.T) {
	groups, nodes, manager := newLazyProbeGroups(t, true, 1, 1, "", time.Minute, time.Hour)
	g := groups[0].group
	// Hold access to stage a pause after Touch has acquired its ticker but
	// before the initial-check owner can claim the work.
	g.access.Lock()
	ticker := time.NewTicker(g.interval)
	g.ticker = ticker
	g.lastActive.Store(time.Now())
	g.lazyInitialCheck, g.lazyInitialScheduled = true, true
	g.pauseCallback = pause.RegisterTicker(manager, ticker, g.interval, nil)
	manager.DevicePause()
	g.access.Unlock()
	g.checkLazyInitial(ticker)
	assertLazyQuiet(t, nodes, 0)
	g.access.Lock()
	pending, scheduled := g.lazyInitialCheck, g.lazyInitialScheduled
	g.access.Unlock()
	if !pending || scheduled {
		t.Fatal("paused initial work was lost or still claimed")
	}
	manager.DeviceWake()
	g.Touch()
	waitLazy(t, "pending first check after waking touch", func() bool { return lazyProbeCount(nodes) == 1 && lazyChecksIdle(groups) })
}

func TestURLTestLazyStartCloseTouchRace(t *testing.T) {
	for i := 0; i < 10; i++ {
		groups, _, _ := newLazyProbeGroups(t, true, 1, 1, "", time.Minute, time.Hour)
		g := groups[0].group
		var workers sync.WaitGroup
		for j := 0; j < 20; j++ {
			workers.Go(g.Touch)
		}
		workers.Go(func() { _ = g.Close() })
		workers.Wait()
		_ = g.Close()
		if g.lazyActive() {
			t.Fatal("concurrent Touch resurrected a closed group")
		}
	}
}

// Force a Pause/Wake pair just before callback registration. The wake edge is
// absent from the callback, so registration must reconcile the atomic flags.
type lazyMissedWakeManager struct {
	pause.Manager
	triggered atomic.Bool
}

func (m *lazyMissedWakeManager) RegisterCallback(callback pause.Callback) *list.Element[pause.Callback] {
	if m.triggered.CompareAndSwap(false, true) {
		m.Manager.DevicePause()
	}
	m.Manager.DeviceWake()
	return m.Manager.RegisterCallback(callback)
}

func TestURLTestLazyStartMissedWakeRegistration(t *testing.T) {
	groups, nodes, manager := newLazyProbeGroups(t, true, 1, 1, "", 10*time.Millisecond, 40*time.Millisecond)
	g := groups[0].group
	wrapped := &lazyMissedWakeManager{Manager: manager}
	g.pause = wrapped
	g.Touch()
	if !wrapped.triggered.Load() {
		t.Fatal("registration-gap fixture did not run")
	}
	waitLazy(t, "periodic probes after registration-gap wake", func() bool { return lazyProbeCount(nodes) > 1 && lazyChecksIdle(groups) })
	waitLazy(t, "idle retirement after registration-gap wake", func() bool { return !g.lazyActive() })
}

func TestURLTestLazyStartPauseTouchRace(t *testing.T) {
	groups, _, manager := newLazyProbeGroups(t, true, 1, 1, "", time.Minute, time.Hour)
	g := groups[0].group
	var workers sync.WaitGroup
	workers.Go(func() {
		for i := 0; i < 100; i++ {
			manager.DevicePause()
			manager.NetworkPause()
			manager.DeviceWake()
			manager.NetworkWake()
		}
	})
	workers.Go(func() {
		for i := 0; i < 100; i++ {
			g.Touch()
		}
	})
	workers.Wait()
	g.Touch()
	if !g.lazyActive() {
		t.Fatal("awake final touch failed to activate group")
	}
}

func TestURLTestLazyStartNestedDependencyProbeRetained(t *testing.T) {
	groups, nodes, _ := newLazyProbeGroups(t, true, 1, 3, "", time.Minute, time.Hour)
	child := groups[0]
	manager := child.outbound.(*fakeManager)
	manager.byTag[child.Tag()] = child
	outbound, err := NewURLTest(child.ctx, nil, child.logger, "parent", option.URLTestOutboundOptions{
		Outbounds: []string{child.Tag()}, URL: "http://probe.invalid/generate_204", LazyStart: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	parent := outbound.(*URLTest)
	parent.interval, parent.idleTimeout = time.Minute, time.Hour
	scope := newSelectorScope(t, child.ctx)
	if err := parent.Start(adapter.StartStateStart, scope); err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(adapter.StartStateStarted, scope); err != nil {
		t.Fatal(err)
	}
	parent.group.Touch()
	waitLazy(t, "active parent dependency probe", func() bool {
		return lazyProbeCount(nodes) == 3 && lazyChecksIdle([]*URLTest{parent, child})
	})
	if child.group.lazyActive() {
		t.Fatal("dependency probe activated child's independent ticker")
	}
	parent.InterfaceUpdated(context.Background())
	waitLazy(t, "active parent forced dependency reset", func() bool {
		return lazyProbeCount(nodes) == 6 && lazyChecksIdle([]*URLTest{parent, child})
	})
}
