//go:build linux

package process

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Controlled netlink-format replies over real local datagram sockets; this
// measures query-lock contention, not live netlink or internet throughput.
func socketDiagPoolFixture(tb testing.TB, family, protocol uint8, delay time.Duration) *socketDiagPool {
	tb.Helper()
	pool := newSocketDiagPool(family, protocol)
	var workers sync.WaitGroup
	var peers []int
	tb.Cleanup(func() {
		for _, fd := range peers {
			_ = syscall.Shutdown(fd, syscall.SHUT_RDWR)
		}
		workers.Wait()
		for _, fd := range peers {
			syscall.Close(fd)
		}
		if err := pool.Close(); err != nil {
			tb.Error(err)
		}
	})
	for i := range pool.conns {
		if pool.conns[i].fd != -1 {
			tb.Fatal("pool must open sockets lazily")
		}
		fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
		if err != nil {
			tb.Fatal(err)
		}
		pool.conns[i].fd = fds[0]
		peers = append(peers, fds[1])
		for _, fd := range fds {
			timeout := syscall.Timeval{Sec: 2}
			for _, option := range []int{syscall.SO_RCVTIMEO, syscall.SO_SNDTIMEO} {
				if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, option, &timeout); err != nil {
					tb.Fatal(err)
				}
			}
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			var request [sizeOfSocketDiagRequest]byte
			for {
				n, err := readSocketDiag(fds[1], request[:])
				if err == syscall.EINTR || err == syscall.EAGAIN {
					continue
				}
				if err != nil || n != len(request) {
					return
				}
				port := request[24:26]
				if protocol == syscall.IPPROTO_UDP {
					port = request[26:28] // Exact UDP queries reverse endpoints.
				}
				id := uint32(binary.BigEndian.Uint16(port))
				time.Sleep(delay)
				if _, err := writeSocketDiag(fds[1], socketDiagBufferReply(family, id, id+10000)); err != nil {
					return
				}
			}
		}()
	}
	return pool
}

func TestSocketDiagPoolIsolation(t *testing.T) {
	for _, family := range []uint8{syscall.AF_INET, syscall.AF_INET6} {
		for _, protocol := range []uint8{syscall.IPPROTO_TCP, syscall.IPPROTO_UDP} {
			t.Run(fmt.Sprintf("%d/%d", family, protocol), func(t *testing.T) {
				pool := socketDiagPoolFixture(t, family, protocol, 0)
				address := netip.MustParseAddr("127.0.0.1")
				if family == syscall.AF_INET6 {
					address = netip.IPv6Loopback()
				}
				for worker := range 32 {
					t.Run(fmt.Sprint(worker), func(t *testing.T) {
						t.Parallel()
						id := uint32(worker + 20000)
						for range 8 {
							inode, uid, err := pool.query(netip.AddrPortFrom(address, uint16(id)), netip.AddrPortFrom(address, 443))
							if err != nil || inode != id || uid != id+10000 {
								t.Fatalf("identity crossed callers: %d/%d %v; want %d", inode, uid, err, id)
							}
						}
					})
				}
			})
		}
	}
}

func TestSocketDiagPoolBlockedLane(t *testing.T) {
	pool := socketDiagPoolFixture(t, syscall.AF_INET, syscall.IPPROTO_TCP, 0)
	pool.next.Store(^uint32(0)) // Also exercise counter wraparound.
	pool.conns[0].access.Lock()
	first := make(chan struct{})
	query := func() {
		_, _, err := pool.query(netip.MustParseAddrPort("127.0.0.1:20000"), netip.MustParseAddrPort("127.0.0.1:443"))
		if err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { pool.conns[0].access.Unlock(); <-first })
	go func() { defer close(first); query() }()
	deadline := time.Now().Add(time.Second)
	for pool.next.Load() == ^uint32(0) {
		if time.Now().After(deadline) {
			t.Fatal("first query did not start")
		}
		runtime.Gosched()
	}
	second := make(chan struct{})
	go func() { defer close(second); query() }()
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Error("an unrelated query waited for the blocked lane")
		pool.conns[0].access.Unlock()
		<-second
		pool.conns[0].access.Lock()
	}
}

func BenchmarkSocketDiagPool(b *testing.B) {
	for _, delay := range []time.Duration{0, 100 * time.Microsecond} {
		for _, pooled := range []bool{false, true} {
			b.Run(fmt.Sprintf("delay=%s/pool=%t", delay, pooled), func(b *testing.B) {
				pool := socketDiagPoolFixture(b, syscall.AF_INET, syscall.IPPROTO_TCP, delay)
				query := pool.conns[0].query // Previous single-socket behavior.
				if pooled {
					query = pool.query
				}
				source, destination := netip.MustParseAddrPort("127.0.0.1:20000"), netip.MustParseAddrPort("127.0.0.1:443")
				b.SetParallelism(8)
				b.ReportAllocs()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						inode, uid, err := query(source, destination)
						if err != nil || inode != 20000 || uid != 30000 {
							b.Errorf("%d/%d: %v", inode, uid, err)
							return
						}
					}
				})
			})
		}
	}
}

func TestSocketDiagPoolClose(t *testing.T) {
	pool := socketDiagPoolFixture(t, syscall.AF_INET, syscall.IPPROTO_TCP, 0)
	for range 2 {
		if err := pool.Close(); err != nil {
			t.Fatal(err)
		}
		for i := range pool.conns {
			if pool.conns[i].fd != -1 {
				t.Fatalf("lane %d retained an open descriptor", i)
			}
		}
	}
}
