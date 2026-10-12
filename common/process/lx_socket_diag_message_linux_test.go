//go:build linux

package process

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"syscall"
	"testing"
)

func socketDiagTestDatagram(payload []byte, count int) []byte {
	length := syscall.NLMSG_HDRLEN + len(payload)
	aligned := (length + syscall.NLMSG_ALIGNTO - 1) &^ (syscall.NLMSG_ALIGNTO - 1)
	data := make([]byte, aligned*count)
	for i := range count {
		frame := data[i*aligned:]
		binary.NativeEndian.PutUint32(frame[:4], uint32(length))
		binary.NativeEndian.PutUint16(frame[4:6], uint16(20+i))
		binary.NativeEndian.PutUint16(frame[6:8], syscall.NLM_F_MULTI)
		binary.NativeEndian.PutUint32(frame[8:12], uint32(i+1))
		binary.NativeEndian.PutUint32(frame[12:16], 1000)
		copy(frame[syscall.NLMSG_HDRLEN:], payload)
	}
	return data
}

func checkSocketDiagMessages(t testing.TB, data []byte) {
	t.Helper()
	want, wantErr := syscall.ParseNetlinkMessage(data)
	got, err := parseSocketDiagMessages(data)
	if !errors.Is(err, wantErr) {
		t.Fatalf("parse error = %v, want %v", err, wantErr)
	}
	if err != nil {
		if len(got) != 0 {
			t.Fatal("malformed datagram exposed messages")
		}
		return
	}
	for i, expected := range want {
		if len(got) == 0 {
			t.Fatalf("missing message %d", i)
		}
		message := got.next()
		if message.Header != expected.Header || !bytes.Equal(message.Data, expected.Data) {
			t.Fatalf("message %d = %+v, want %+v", i, message, expected)
		}
	}
	if len(got) != 0 {
		t.Fatal("extra messages")
	}
}

func TestSocketDiagMessagesCompatibility(t *testing.T) {
	valid := socketDiagTestDatagram([]byte{1, 2, 3, 4, 5}, 2)
	for n := 0; n <= len(valid); n++ {
		t.Run(fmt.Sprintf("prefix_%d", n), func(t *testing.T) {
			checkSocketDiagMessages(t, valid[:n])
		})
	}
	for n := 0; n < syscall.NLMSG_HDRLEN; n++ {
		checkSocketDiagMessages(t, append(bytes.Clone(valid), make([]byte, n)...))
	}
	for _, size := range []int{0, 1, 2, 3, 4, 72, 73, 256} {
		checkSocketDiagMessages(t, socketDiagTestDatagram(make([]byte, size), 16))
	}
}

func TestSocketDiagMessagesRejectInvalidLengths(t *testing.T) {
	for _, length := range []uint32{0, 1, 15, 17, 100, 1<<31 - 1, 1 << 31, ^uint32(0)} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			bad := make([]byte, syscall.NLMSG_HDRLEN)
			binary.NativeEndian.PutUint32(bad[:4], length)
			for _, prefix := range [][]byte{nil, socketDiagTestDatagram(make([]byte, 72), 1)} {
				data := append(bytes.Clone(prefix), bad...)
				messages, err := parseSocketDiagMessages(data)
				if !errors.Is(err, syscall.EINVAL) || len(messages) != 0 {
					t.Fatalf("length %d: messages=%d err=%v", length, len(messages), err)
				}
			}
		})
	}
}

func TestSocketDiagMessagesBorrowBuffer(t *testing.T) {
	data := socketDiagTestDatagram([]byte{1, 2, 3}, 1)
	messages, err := parseSocketDiagMessages(data)
	if err != nil {
		t.Fatal(err)
	}
	message := messages.next()
	data[syscall.NLMSG_HDRLEN] = 9
	if message.Data[0] != 9 || len(messages) != 0 {
		t.Fatal("payload must borrow the input and iterator must be exhausted")
	}
}

func TestSocketDiagMessagesAllocations(t *testing.T) {
	for _, count := range []int{1, 16, 256} {
		data := socketDiagTestDatagram(make([]byte, 72), count)
		allocations := testing.AllocsPerRun(100, func() {
			messages, err := parseSocketDiagMessages(data)
			if err != nil {
				panic(err)
			}
			for len(messages) > 0 {
				message := messages.next()
				socketDiagMessageSink += uint64(message.Header.Seq)
			}
		})
		if allocations != 0 {
			t.Fatalf("%d messages: allocations = %v, want zero", count, allocations)
		}
	}
}

func TestSocketDiagMessagesDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 5000 {
		data := socketDiagTestDatagram(make([]byte, rng.Intn(128)), rng.Intn(16))
		if len(data) != 0 {
			data = data[:rng.Intn(len(data)+1)]
		}
		if len(data) >= syscall.NLMSG_HDRLEN {
			// Keep standard-library lengths small enough to be safe on 32-bit Go.
			binary.NativeEndian.PutUint32(data[:4], uint32(rng.Intn(len(data)+4)))
		}
		checkSocketDiagMessages(t, data)
	}
}

var socketDiagMessageSink uint64

func BenchmarkSocketDiagMessages(b *testing.B) {
	for _, count := range []int{1, 16, 256} {
		data := socketDiagTestDatagram(make([]byte, 72), count)
		b.Run(fmt.Sprintf("stdlib/%d", count), func(b *testing.B) {
			b.ReportAllocs()
			var sum uint64
			for i := 0; i < b.N; i++ {
				messages, err := syscall.ParseNetlinkMessage(data)
				if err != nil {
					b.Fatal(err)
				}
				for _, message := range messages {
					sum += uint64(message.Header.Seq) + uint64(len(message.Data))
				}
			}
			socketDiagMessageSink = sum
		})
		b.Run(fmt.Sprintf("iterator/%d", count), func(b *testing.B) {
			b.ReportAllocs()
			var sum uint64
			for i := 0; i < b.N; i++ {
				messages, err := parseSocketDiagMessages(data)
				if err != nil {
					b.Fatal(err)
				}
				for len(messages) > 0 {
					message := messages.next()
					sum += uint64(message.Header.Seq) + uint64(len(message.Data))
				}
			}
			socketDiagMessageSink = sum
		})
	}
}
