//go:build with_ebpf && (linux || android)

package ebpf

import (
	"net/netip"
	"runtime"
	"testing"
)

func TestUDPIndexesReleaseBurstStorage(t *testing.T) {
	var table udpClientTable
	clients := make([]netip.AddrPort, 0, 1024)
	for port := 1; len(clients) < 1024; port++ {
		client := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
		if table.clientShard(client) == &table.clientShards[0] {
			clients = append(clients, client)
			table.loadOrCreate(client)
			table.retainRedirectLocked(udpRedirectReference{client: client})
		}
	}
	for _, client := range clients {
		state, _ := table.load(client)
		table.deleteClient(client, state)
		if !table.releaseRedirectLocked(udpRedirectReference{client: client}) {
			t.Fatal("missing reference")
		}
	}
	if table.clientShards[0].clients != nil || table.redirectReferences != nil {
		t.Fatal("burst storage retained")
	}
	client := clients[0]
	state := table.loadOrCreate(client)
	table.retainRedirectLocked(udpRedirectReference{client: client})
	if !table.current(client, state) || !table.releaseRedirectLocked(udpRedirectReference{client: client}) {
		t.Fatal("indexes cannot be reused")
	}
	table.deleteClient(client, state)
	if table.clientShards[0].clients == nil {
		t.Fatal("small index unnecessarily discarded")
	}
}

func BenchmarkUDPIdleIndexRetention(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		var table udpClientTable
		for port := 1; port <= 32768; port++ {
			table.loadOrCreate(netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port)))
		}
		for port := 1; port <= 32768; port++ {
			client := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
			state, _ := table.load(client)
			table.deleteClient(client, state)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)), "idle-heap-B")
		runtime.KeepAlive(&table)
	}
}
