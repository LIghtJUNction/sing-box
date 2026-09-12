//go:build linux

package process

import (
	"bytes"
	"syscall"
	"testing"
)

func TestSocketDiagIORetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		n       int
		err     error
		attempt int
		want    bool
	}{
		{"success", 3, nil, 0, false},
		{"interrupted", -1, syscall.EINTR, 0, true},
		{"third_retry", 0, syscall.EINTR, 2, true},
		{"bounded_signal_storm", -1, syscall.EINTR, 3, false},
		{"timeout", -1, syscall.EAGAIN, 0, false},
		{"permission", -1, syscall.EPERM, 0, false},
		{"partial_result", 1, syscall.EINTR, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := socketDiagInterrupted(tc.n, tc.err, tc.attempt); got != tc.want {
				t.Fatalf("retry=%v; want %v", got, tc.want)
			}
		})
	}
}

func TestSocketDiagIODatagram(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fds[0])
	defer syscall.Close(fds[1])
	for _, fd := range fds {
		timeout := syscall.Timeval{Sec: 1}
		for _, option := range []int{syscall.SO_RCVTIMEO, syscall.SO_SNDTIMEO} {
			if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, option, &timeout); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := []byte("one datagram")
	if n, err := writeSocketDiag(fds[0], want); err != nil || n != len(want) {
		t.Fatalf("write=%d/%v", n, err)
	}
	var buffer [32]byte
	if n, err := readSocketDiag(fds[1], buffer[:]); err != nil || n < 0 ||
		!bytes.Equal(buffer[:n], want) {
		t.Fatalf("read=%d/%v", n, err)
	}
	if _, err := readSocketDiag(-1, buffer[:]); err != syscall.EBADF {
		t.Fatalf("invalid fd error=%v", err)
	}
}
