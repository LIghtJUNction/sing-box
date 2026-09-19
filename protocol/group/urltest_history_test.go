package group

import (
	"context"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/service"
)

type historyNode struct {
	adapter.Outbound
	tag string
}

func (n *historyNode) Tag() string       { return n.tag }
func (n *historyNode) Network() []string { return []string{"tcp", "udp"} }

func TestURLTestGroupsDoNotSelectUsingAnotherTargetHistory(t *testing.T) {
	shared := urltest.NewHistoryStorage()
	ctx := service.ContextWithPtr(context.Background(), shared)
	nodes := []adapter.Outbound{&historyNode{tag: "a"}, &historyNode{tag: "b"}}
	newGroup := func(link string) *URLTestGroup {
		g, err := NewURLTestGroup(ctx, nil, log.NewNOPFactory().Logger(), nodes, link, time.Minute, 30, 10*time.Minute, false)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	chat := newGroup("https://chatgpt.com/")
	generic := newGroup("https://www.gstatic.com/generate_204")
	chat.history.StoreURLTestHistory("a", &adapter.URLTestHistory{Time: time.Now(), Delay: 100})
	chat.history.StoreURLTestHistory("b", &adapter.URLTestHistory{Time: time.Now(), Delay: 800})
	if generic.history.LoadURLTestHistory("a") != nil {
		t.Fatal("another target can suppress this group's probe")
	}
	generic.history.StoreURLTestHistory("a", &adapter.URLTestHistory{Time: time.Now(), Delay: 900})
	generic.history.StoreURLTestHistory("b", &adapter.URLTestHistory{Time: time.Now(), Delay: 200})
	for network, want := range map[string]string{"tcp": "a", "udp": "a"} {
		got, measured := chat.Select(network)
		if !measured || got.Tag() != want {
			t.Fatalf("chat selected %v, measured=%v", got, measured)
		}
	}
	got, measured := generic.Select("tcp")
	if !measured || got.Tag() != "b" {
		t.Fatal("generic group lost its own evidence")
	}
	chat.history.DeleteURLTestHistory("b")
	if shared.LoadURLTestHistory("b") == nil || generic.history.LoadURLTestHistory("b") == nil {
		t.Fatal("one target's failure erased another target's result")
	}
	generic.history.DeleteURLTestHistory("b")
	if shared.LoadURLTestHistory("b") != nil {
		t.Fatal("latest failed result remained visible")
	}
	shared.StoreURLTestHistory("a", &adapter.URLTestHistory{Time: time.Now(), Delay: 5000})
	if chat.history.LoadURLTestHistory("a").Delay != 100 {
		t.Fatal("manual probe changed another target's selection evidence")
	}
}
