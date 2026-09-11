//go:build linux

package process

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"runtime"
	"syscall"
	"testing"
)

// Real local datagram I/O with controlled netlink replies, independent of the
// host's socket-owner lookup permissions. This does not measure proxy bandwidth.
func socketDiagExchange(tb testing.TB) func([]byte) (uint32, uint32, error) {
	tb.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		syscall.Close(fds[0])
		syscall.Close(fds[1])
	})
	for _, fd := range fds {
		timeout := syscall.Timeval{Sec: 1}
		for _, option := range []int{syscall.SO_RCVTIMEO, syscall.SO_SNDTIMEO} {
			if err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, option, &timeout); err != nil {
				tb.Fatal(err)
			}
		}
	}
	request := packSocketDiagRequest(syscall.AF_INET, syscall.IPPROTO_TCP,
		netip.MustParseAddrPort("127.0.0.1:12345"), netip.MustParseAddrPort("127.0.0.1:443"), false)
	var received [sizeOfSocketDiagRequest]byte
	return func(reply []byte) (uint32, uint32, error) {
		if _, err := syscall.Write(fds[1], reply); err != nil {
			return 0, 0, err
		}
		inode, uid, queryErr := querySocketDiag(fds[0], request)
		n, err := syscall.Read(fds[1], received[:])
		if err != nil {
			return 0, 0, err
		}
		if !bytes.Equal(received[:n], request) {
			return 0, 0, fmt.Errorf("netlink request changed")
		}
		return inode, uid, queryErr
	}
}

func socketDiagBufferReply(family uint8, inode, uid uint32) []byte {
	reply := make([]byte, syscall.SizeofNlMsghdr+socketDiagResponseMinSize)
	binary.NativeEndian.PutUint32(reply[:4], uint32(len(reply)))
	binary.NativeEndian.PutUint16(reply[4:6], socketDiagByFamily)
	data := reply[syscall.SizeofNlMsghdr:]
	data[0] = family
	binary.NativeEndian.PutUint32(data[64:68], uid)
	binary.NativeEndian.PutUint32(data[68:72], inode)
	return reply
}

func TestSocketDiagBufferIsolation(t *testing.T) {
	for worker := range 32 {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			exchange := socketDiagExchange(t)
			wantInode, wantUID := uint32(worker+1), uint32(10000+worker)
			replies := [][]byte{
				socketDiagBufferReply(syscall.AF_INET, wantInode, wantUID),
				socketDiagBufferReply(syscall.AF_INET6, wantInode, wantUID),
			}
			notFound := make([]byte, syscall.SizeofNlMsghdr+4)
			binary.NativeEndian.PutUint32(notFound[:4], uint32(len(notFound)))
			binary.NativeEndian.PutUint16(notFound[4:6], syscall.NLMSG_ERROR)
			binary.NativeEndian.PutUint32(notFound[syscall.SizeofNlMsghdr:], ^uint32(syscall.ENOENT-1))
			for i := range 16 {
				for _, reply := range replies {
					inode, uid, err := exchange(reply)
					if err != nil || inode != wantInode || uid != wantUID {
						t.Fatalf("inode=%d uid=%d err=%v; want %d/%d", inode, uid, err, wantInode, wantUID)
					}
				}
				inode, uid, err := exchange(notFound)
				if !errors.Is(err, ErrNotFound) || inode != 0 || uid != 0 {
					t.Fatalf("error response retained identity: %d/%d %v", inode, uid, err)
				}
				inode, uid, err = exchange(replies[0][:syscall.SizeofNlMsghdr+3])
				if err == nil || inode != 0 || uid != 0 {
					t.Fatalf("truncated response retained identity: %d/%d %v", inode, uid, err)
				}
				if worker == 0 && i == 8 {
					runtime.GC()
				}
			}
		})
	}
}

func BenchmarkSocketDiagBuffer(b *testing.B) {
	b.Run("serial", func(b *testing.B) {
		exchange := socketDiagExchange(b)
		reply := socketDiagBufferReply(syscall.AF_INET, 42, 10000)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			inode, uid, err := exchange(reply)
			if err != nil || inode != 42 || uid != 10000 {
				b.Fatalf("inode=%d uid=%d err=%v", inode, uid, err)
			}
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.SetParallelism(8)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			exchange := socketDiagExchange(b)
			reply := socketDiagBufferReply(syscall.AF_INET, 42, 10000)
			for pb.Next() {
				inode, uid, err := exchange(reply)
				if err != nil || inode != 42 || uid != 10000 {
					b.Errorf("inode=%d uid=%d err=%v", inode, uid, err)
					return
				}
			}
		})
	})
}
