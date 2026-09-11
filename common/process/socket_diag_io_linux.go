//go:build linux

package process

import "syscall"

// Retry only the interrupted datagram operation, not the whole exchange: an
// interrupted read leaves the reply on the same socket. Bound retries so a
// signal storm cannot spin forever. SO_*TIMEO is still a per-syscall timeout,
// not a deadline for the entire exchange. Never retry progress or EAGAIN.
func socketDiagInterrupted(n int, err error, attempt int) bool {
	return n <= 0 && err == syscall.EINTR && attempt < 3
}

func readSocketDiag(fd int, buffer []byte) (int, error) {
	for attempt := 0; ; attempt++ {
		n, err := syscall.Read(fd, buffer)
		if !socketDiagInterrupted(n, err, attempt) {
			return n, err
		}
	}
}

func writeSocketDiag(fd int, buffer []byte) (int, error) {
	for attempt := 0; ; attempt++ {
		n, err := syscall.Write(fd, buffer)
		if !socketDiagInterrupted(n, err, attempt) {
			return n, err
		}
	}
}
