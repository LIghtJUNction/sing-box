package vless

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"net"
	"sync"
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

type cancelBlockingConn struct {
	writeStarted chan struct{}
	unblock      chan struct{}
	startOnce    sync.Once
	unblockOnce  sync.Once
}

func newCancelBlockingConn() *cancelBlockingConn {
	return &cancelBlockingConn{
		writeStarted: make(chan struct{}),
		unblock:      make(chan struct{}),
	}
}

func (c *cancelBlockingConn) Read([]byte) (int, error) {
	return 0, errors.New("unexpected handshake read")
}

func (c *cancelBlockingConn) Write(b []byte) (int, error) {
	c.startOnce.Do(func() { close(c.writeStarted) })
	<-c.unblock
	return 0, context.Canceled
}

func (c *cancelBlockingConn) Close() error {
	c.unblockOnce.Do(func() { close(c.unblock) })
	return nil
}

func (c *cancelBlockingConn) LocalAddr() net.Addr  { return M.Socksaddr{} }
func (c *cancelBlockingConn) RemoteAddr() net.Addr { return M.Socksaddr{} }
func (c *cancelBlockingConn) SetDeadline(deadline time.Time) error {
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		c.unblockOnce.Do(func() { close(c.unblock) })
	}
	return nil
}
func (c *cancelBlockingConn) SetReadDeadline(time.Time) error {
	return nil
}
func (c *cancelBlockingConn) SetWriteDeadline(time.Time) error {
	return nil
}

// TestWrapEncryptionCancelInterruptsHandshake pins the shutdown path: cancelling
// the URL-test/group context must interrupt the bare encryption handshake now,
// not wait for the original TCP deadline and leave a zombie across box.Close.
func TestWrapEncryptionCancelInterruptsHandshake(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encryptionClient := &encryption.ClientInstance{}
	if err := encryptionClient.Init([][]byte{privateKey.PublicKey().Bytes()}, 0, 0, ""); err != nil {
		t.Fatal(err)
	}

	dialer := &vlessDialer{encryption: encryptionClient}
	conn := newCancelBlockingConn()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := dialer.wrapEncryption(ctx, conn)
		result <- err
	}()

	select {
	case <-conn.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("encryption handshake did not reach the blocked write")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wrapEncryption error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("encryption handshake survived context cancellation")
	}
}
