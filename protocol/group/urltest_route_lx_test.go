package group

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/interrupt"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/route"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

// The route tests observe real leaf dials, rather than calling URLTest.pick or
// simulating a group's NewConnection handler (removed by upstream).
type routedDialNode struct {
	adapter.Outbound
	t      *testing.T
	tag    string
	kind   string
	err    error
	dials  atomic.Int32
	events chan<- string
}

func (n *routedDialNode) Type() string {
	if n.kind != "" {
		return n.kind
	}
	return C.TypeDirect
}

func (n *routedDialNode) Tag() string            { return n.tag }
func (n *routedDialNode) Network() []string      { return []string{N.NetworkTCP, N.NetworkUDP} }
func (n *routedDialNode) Dependencies() []string { return nil }
func (n *routedDialNode) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	n.dials.Add(1)
	n.events <- n.tag
	if n.err != nil {
		return nil, n.err
	}
	local, remote := net.Pipe()
	n.t.Cleanup(func() { _ = local.Close(); _ = remote.Close() })
	return local, nil
}

func newRoutedURLTest(t *testing.T, nodes []*routedDialNode, options option.URLTestOutboundOptions) (*URLTest, context.Context, *stubOutboundManager) {
	t.Helper()
	manager := &stubOutboundManager{byTag: make(map[string]adapter.Outbound)}
	for _, node := range nodes {
		manager.byTag[node.tag] = node
		options.Outbounds = append(options.Outbounds, node.tag)
	}
	ctx := service.ContextWith[adapter.OutboundManager](context.Background(), manager)
	ctx = service.ContextWith[adapter.ConnectionManager](ctx, route.NewConnectionManager(log.NewNOPFactory().NewLogger("connection-test")))
	ctx = service.ContextWithPtr(ctx, urltest.NewHistoryStorage())
	created, err := NewURLTest(ctx, nil, log.NewNOPFactory().NewLogger("urltest-test"), "auto", options)
	if err != nil {
		t.Fatal(err)
	}
	group := created.(*URLTest)
	// Start resolves dependencies but does not launch a URL probe or ticker.
	if err = group.Start(adapter.StartStateStart, newSelectorScope(t, ctx)); err != nil {
		t.Fatal(err)
	}
	for index, node := range nodes {
		group.group.history.StoreURLTestHistory(node.tag, &adapter.URLTestHistory{Time: time.Now(), Delay: uint16(10 * (index + 1))})
	}
	if group.balancer != nil {
		group.balancer.setSlots(options.Outbounds)
	}
	return group, ctx, manager
}

func routeGroupDial(t *testing.T, ctx context.Context, selected adapter.Outbound, host string) {
	t.Helper()
	ctx = service.ContextWith[adapter.OutboundManager](ctx, &selectedOutboundManager{
		OutboundManager: service.FromContext[adapter.OutboundManager](ctx),
		selected:        selected,
	})
	ctx = service.ContextWith[adapter.DNSRouter](ctx, emptyReverseDNS{})
	router := route.NewRouter(ctx, log.NewNOPFactory(), option.RouteOptions{}, option.DNSOptions{})
	client, inbound := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = inbound.Close() })
	router.RouteConnectionEx(ctx, inbound, adapter.InboundContext{
		Destination: M.ParseSocksaddr(host + ":443"),
	}, func(error) {})
}

func routedPick(t *testing.T, events <-chan string) string {
	t.Helper()
	select {
	case tag := <-events:
		return tag
	case <-time.After(time.Second):
		t.Fatal("Router did not dial a leaf")
		return ""
	}
}

func TestLxURLTestRouterRoundRobinPerConnection(t *testing.T) {
	events := make(chan string, 16)
	nodes := []*routedDialNode{
		{t: t, tag: "a", events: events},
		{t: t, tag: "b", events: events},
		{t: t, tag: "c", events: events},
	}
	group, ctx, _ := newRoutedURLTest(t, nodes, option.URLTestOutboundOptions{
		Mode:         C.URLTestModeRoundRobin,
		PassiveCheck: true,
		Balancer:     &option.URLTestBalancerOptions{Pool: 3, StickyHash: []string{C.URLTestStickyNone}},
	})
	var got []string
	for range 6 {
		routeGroupDial(t, ctx, group, "same.example")
		got = append(got, routedPick(t, events))
	}
	if want := []string{"a", "b", "c", "a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("routed round-robin = %v, want %v", got, want)
	}
	for _, node := range nodes {
		if node.dials.Load() != 2 || !group.group.passiveFresh(node.tag) {
			t.Fatalf("%s: dials=%d passive=%v", node.tag, node.dials.Load(), group.group.passiveFresh(node.tag))
		}
	}
}

func TestLxURLTestRouterDestinationAffinity(t *testing.T) {
	events := make(chan string, 32)
	nodes := []*routedDialNode{{t: t, tag: "a", events: events}, {t: t, tag: "b", events: events}}
	group, ctx, _ := newRoutedURLTest(t, nodes, option.URLTestOutboundOptions{
		Mode:     C.URLTestModeRoundRobin,
		Balancer: &option.URLTestBalancerOptions{Pool: 2, StickyHash: []string{C.URLTestStickyDomain}},
	})
	hosts := []string{"alpha.example", "beta.example", "gamma.example", "delta.example", "epsilon.example"}
	picks := make(map[string]string)
	used := make(map[string]bool)
	for round := range 3 {
		for index := range hosts {
			host := hosts[(index+round)%len(hosts)]
			routeGroupDial(t, ctx, group, host)
			picked := routedPick(t, events)
			if previous := picks[host]; previous != "" && previous != picked {
				t.Fatalf("destination %s changed node from %s to %s", host, previous, picked)
			}
			picks[host] = picked
			used[picked] = true
		}
	}
	if len(used) != 2 {
		t.Fatalf("distinct destination keys must use both pool members, got %v", picks)
	}
}

func TestLxURLTestRouterTimeoutBoundedFallback(t *testing.T) {
	for _, fallbackSucceeds := range []bool{true, false} {
		t.Run(fmt.Sprintf("success=%v", fallbackSucceeds), func(t *testing.T) {
			events := make(chan string, 8)
			primary := &routedDialNode{t: t, tag: "primary", err: context.DeadlineExceeded, events: events}
			fallback := &routedDialNode{t: t, tag: "fallback", events: events}
			third := &routedDialNode{t: t, tag: "third", events: events}
			if !fallbackSucceeds {
				fallback.err = context.DeadlineExceeded
			}
			group, ctx, _ := newRoutedURLTest(t, []*routedDialNode{primary, fallback, third}, option.URLTestOutboundOptions{PassiveCheck: true})
			group.group.selectedOutboundTCP = primary
			routeGroupDial(t, ctx, group, "target.example")
			if got := []string{routedPick(t, events), routedPick(t, events)}; !reflect.DeepEqual(got, []string{"primary", "fallback"}) {
				t.Fatalf("timeout dial path = %v", got)
			}
			if primary.dials.Load() != 1 || fallback.dials.Load() != 1 || third.dials.Load() != 0 {
				t.Fatalf("fallback must stop after two attempts: %d/%d/%d", primary.dials.Load(), fallback.dials.Load(), third.dials.Load())
			}
			if group.group.penaltyOf("primary") != 1 {
				t.Fatal("classified timeout must penalize the failed primary")
			}
			if fallbackSucceeds {
				if group.group.selectedOutboundTCP != adapter.Outbound(fallback) || !group.group.passiveFresh("fallback") {
					t.Fatal("successful fallback must become selected with passive liveness")
				}
				routeGroupDial(t, ctx, group, "another.example")
				if got := routedPick(t, events); got != "fallback" || primary.dials.Load() != 1 {
					t.Fatalf("next route must retain healthy fallback, picked %s", got)
				}
			} else if group.group.passiveFresh("fallback") {
				t.Fatal("failed fallback cannot publish passive liveness")
			}
		})
	}
}

type routedLeafOverride struct {
	leaf        adapter.Outbound
	replacement adapter.Outbound
	access      sync.Mutex
	resolved    []string
}

func (r *routedLeafOverride) ResolveLeaf(_ context.Context, leaf adapter.Outbound) (adapter.Outbound, error) {
	r.access.Lock()
	defer r.access.Unlock()
	r.resolved = append(r.resolved, leaf.Tag())
	if leaf != r.leaf {
		return nil, fmt.Errorf("unexpected chain leaf %s", leaf.Tag())
	}
	return r.replacement, nil
}

func TestLxGroupRouterEndpointChainLeafOverride(t *testing.T) {
	for _, kind := range []string{"selector", "urltest"} {
		t.Run(kind, func(t *testing.T) {
			events := make(chan string, 4)
			original := &routedDialNode{t: t, tag: "endpoint", kind: C.TypeWireGuard, events: events}
			clone := &routedDialNode{t: t, tag: "endpoint-clone", kind: C.TypeWireGuard, events: events}
			group, ctx, manager := newRoutedURLTest(t, []*routedDialNode{original}, option.URLTestOutboundOptions{})
			var selected adapter.Outbound = group
			if kind == "selector" {
				selector := &Selector{
					Adapter:        outbound.NewAdapter(C.TypeSelector, "selector", nil, []string{"endpoint"}),
					ctx:            ctx,
					outbound:       manager,
					logger:         log.NewNOPFactory().NewLogger("selector-test"),
					tags:           []string{"endpoint"},
					outbounds:      make(map[string]adapter.Outbound),
					interruptGroup: interrupt.NewGroup(),
				}
				if err := selector.Start(adapter.StartStateStart, newSelectorScope(t, ctx)); err != nil {
					t.Fatal(err)
				}
				selected = selector
			}
			resolver := &routedLeafOverride{leaf: original, replacement: clone}
			ctx = adapter.ContextWithChainHop(ctx, resolver)
			routeGroupDial(t, ctx, selected, "endpoint-target.example")
			if got := routedPick(t, events); got != "endpoint-clone" {
				t.Fatalf("routed chain bypassed leaf override: %s", got)
			}
			if original.dials.Load() != 0 || clone.dials.Load() != 1 || !reflect.DeepEqual(resolver.resolved, []string{"endpoint"}) {
				t.Fatalf("leaf override original=%d clone=%d resolved=%v", original.dials.Load(), clone.dials.Load(), resolver.resolved)
			}
		})
	}
}
