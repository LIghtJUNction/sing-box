package vless

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing-box/protocol/vless/encryption"
	M "github.com/sagernet/sing/common/metadata"
)

// SPECS/TASKS/050-URLTEST_ZOMBIE_RUN_SURVIVES_RESTART
//
// wrapEncryption bounds the PQ handshake with the dial deadline, because
// Handshake works on a bare net.Conn and would otherwise block forever on a
// half-alive node. It must arm the WRITE side only.
//
// Why that matters: on an XHTTP conn the deadlines are one-shot. An expired read
// deadline closes the late-bound download body and clearing it cannot reopen it,
// so arming both directions (what SetDeadline does) hands the caller a conn whose
// download side is already dead whenever the handshake overruns the deadline but
// still succeeds — a connection that looks healthy and fails on its first read.
// An audit of this task caught exactly that; this test is the guard.

// deadlineRecordingConn records which deadline setters were called.
type deadlineRecordingConn struct {
	net.Conn
	bothCalled  bool
	writeCalled bool
	readCalled  bool
}

func (c *deadlineRecordingConn) SetDeadline(t time.Time) error {
	c.bothCalled = true
	return nil
}

func (c *deadlineRecordingConn) SetWriteDeadline(t time.Time) error {
	c.writeCalled = true
	return nil
}

func (c *deadlineRecordingConn) SetReadDeadline(t time.Time) error {
	c.readCalled = true
	return nil
}

func (c *deadlineRecordingConn) Read(b []byte) (int, error)  { return 0, errors.New("handshake stub") }
func (c *deadlineRecordingConn) Write(b []byte) (int, error) { return len(b), nil }
func (c *deadlineRecordingConn) Close() error                { return nil }
func (c *deadlineRecordingConn) LocalAddr() net.Addr         { return M.Socksaddr{} }
func (c *deadlineRecordingConn) RemoteAddr() net.Addr        { return M.Socksaddr{} }

// TestWrapEncryptionArmsWriteDeadlineOnly pins the contract: the dial deadline is
// applied to the write direction and never to the read direction.
func TestWrapEncryptionArmsWriteDeadlineOnly(t *testing.T) {
	dialer := &vlessDialer{
		// A non-nil instance is all wrapEncryption needs to take the guarded path;
		// the handshake itself fails on the stub conn, which is fine — the deadline
		// is armed before it runs.
		encryption: &encryption.ClientInstance{},
	}
	conn := &deadlineRecordingConn{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, _ = dialer.wrapEncryption(ctx, conn)

	if conn.bothCalled {
		t.Fatal("wrapEncryption called SetDeadline: that arms the read side too, and an " +
			"expired XHTTP read deadline permanently closes the download body")
	}
	if conn.readCalled {
		t.Fatal("wrapEncryption armed the read deadline; the handshake hang is a blocked Write")
	}
	if !conn.writeCalled {
		t.Fatal("wrapEncryption did not arm the write deadline: the handshake is unbounded again")
	}
}

// TestWrapEncryptionWithoutDeadlineTouchesNothing: a dial context with no deadline
// (ordinary proxied traffic) must leave the conn's deadlines alone.
func TestWrapEncryptionWithoutDeadlineTouchesNothing(t *testing.T) {
	dialer := &vlessDialer{encryption: &encryption.ClientInstance{}}
	conn := &deadlineRecordingConn{}

	_, _ = dialer.wrapEncryption(context.Background(), conn)

	if conn.bothCalled || conn.writeCalled || conn.readCalled {
		t.Fatal("wrapEncryption armed a deadline although the dial context carries none")
	}
}

// A cancelled parent without an explicit deadline must also release a blocked
// encryption write. Box shutdown uses cancellation, not a deadline; missing
// this path left one URL-test goroutine alive after every restart.
func TestWrapEncryptionCancellationReleasesBlockedWrite(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	instance := new(encryption.ClientInstance)
	if err = instance.Init([][]byte{privateKey.PublicKey().Bytes()}, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	dialer := &vlessDialer{encryption: instance}
	client, server := net.Pipe()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, wrapErr := dialer.wrapEncryption(ctx, client)
		done <- wrapErr
	}()

	// net.Pipe has no buffer, so Handshake is blocked in its first write here.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("cancelled handshake unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled handshake left its write blocked")
	}
}
