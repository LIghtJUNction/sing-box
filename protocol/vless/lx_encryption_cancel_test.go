package vless

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing-box/protocol/vless/encryption"
)

type handshakeSignalConn struct {
	net.Conn
	entered chan struct{}
}

func (c *handshakeSignalConn) Write(p []byte) (int, error) {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	return c.Conn.Write(p)
}

func cancellationTestDialer(t *testing.T) *vlessDialer {
	t.Helper()
	config, err := parseClientEncryption("mlkem768x25519plus.native.1rtt.AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA")
	if err != nil {
		t.Fatal(err)
	}
	instance := new(encryption.ClientInstance)
	if err := instance.Init(config.keys, config.xorMode, config.seconds, config.padding); err != nil {
		t.Fatal(err)
	}
	return &vlessDialer{encryption: instance}
}

func TestWrapEncryptionCancellationUnblocksWriteWithoutDeadline(t *testing.T) {
	dialer := cancellationTestDialer(t)
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	conn := &handshakeSignalConn{Conn: left, entered: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := dialer.wrapEncryption(ctx, conn)
		done <- err
	}()
	select {
	case <-conn.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("handshake never entered its blocking write")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled handshake retained its stream and goroutine")
	}
}

func TestWrapEncryptionDisarmsCancellationAfterSuccess(t *testing.T) {
	dialer := cancellationTestDialer(t)
	// A valid cached ticket takes the ordinary 0-RTT successful return path.
	dialer.encryption.Seconds = 60
	dialer.encryption.Expire = time.Now().Add(time.Minute)
	dialer.encryption.PfsKey = make([]byte, 32)
	dialer.encryption.Ticket = make([]byte, 32)
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := dialer.wrapEncryption(ctx, left)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	left.SetWriteDeadline(time.Now().Add(time.Second))
	right.SetReadDeadline(time.Now().Add(time.Second))
	done := make(chan error, 1)
	go func() { _, err := left.Write([]byte("alive")); done <- err }()
	buffer := make([]byte, 5)
	if _, err := io.ReadFull(right, buffer); err != nil {
		t.Fatalf("dial cancellation closed a successfully returned stream: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
