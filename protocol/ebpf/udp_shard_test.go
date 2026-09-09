//go:build with_ebpf && (linux || android)

package ebpf

import (
	"net/netip"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func samePortUDPClients(ipv6 bool, count int) []netip.AddrPort {
	clients := make([]netip.AddrPort, count)
	for index := range clients {
		address := netip.AddrFrom4([4]byte{192, 0, byte(index >> 8), byte(index)})
		if ipv6 {
			address = netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(index >> 8), byte(index)})
		}
		clients[index] = netip.AddrPortFrom(address, 3478)
	}
	return clients
}

func TestUDPClientTableSamePortIsolation(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		var table udpClientTable
		clients := samePortUDPClients(ipv6, 64)
		destination := netip.MustParseAddrPort("192.0.2.1:3478")
		var workers sync.WaitGroup
		for index, client := range clients {
			workers.Add(1)
			go func() {
				defer workers.Done()
				redirect := netip.AddrFrom4([4]byte{127, 128, 0, byte(index + 1)})
				table.setBinding(client, destination, redirect, false)
				state, loaded := table.load(client)
				if !loaded {
					t.Error("client state was not retained")
					return
				}
				binding, loaded := state.redirectBinding(destination)
				if !loaded || binding.address != redirect {
					t.Error("same-port clients shared a binding")
				}
				table.delete(client, state)
				if _, loaded := table.load(client); loaded {
					t.Error("client state survived deletion")
				}
			}()
		}
		workers.Wait()
	}
}

func BenchmarkUDPClientTableSamePortParallel(b *testing.B) {
	for _, family := range []string{"ipv4", "ipv6"} {
		b.Run(family, func(b *testing.B) {
			const clientCount = 256
			clients := samePortUDPClients(family == "ipv6", clientCount)
			destination := netip.MustParseAddrPort("192.0.2.1:3478")
			redirect := netip.MustParseAddr("127.128.0.9")
			var table udpClientTable
			for _, client := range clients {
				table.setBinding(client, destination, redirect, false)
			}
			var nextWorker atomic.Uint32
			stride := runtime.GOMAXPROCS(0)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				index := int(nextWorker.Add(1) - 1)
				for pb.Next() {
					original, ready, loaded := table.cachedPacketState(clients[index%clientCount], redirect)
					if !loaded || !ready || original.original.Destination != destination {
						b.Fatal("cached UDP destination was lost")
					}
					index += stride
				}
			})
		})
	}
}
