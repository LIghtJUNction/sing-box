from pathlib import Path
import subprocess
root=Path.cwd()
assert subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()=='c5a4f4dc186165f9cd8ae2ee19dd8a8863bed494'
def edit(fn,old,new):
 p=root/fn;s=p.read_text();assert s.count(old)==1,(fn,s.count(old));p.write_text(s.replace(old,new))
edit('transport/v2ray/transport.go','return constructor(ctx, dialer, serverAddr, options, tlsConfig)','client, err := constructor(ctx, dialer, serverAddr, options, tlsConfig)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn client, nil')
edit('protocol/vless/lx_encryption.go','\tencryptedConn, err := h.encryption.Handshake(conn)\n', '''	// Deadlines alone do not observe an earlier cancellation. Close the raw
	// stream while the handshake is in flight, but disarm before returning a
	// successful connection so later dial-context expiry cannot kill it.
	if err := ctx.Err(); err != nil {
		common.Close(conn)
		return nil, E.Cause(err, "encryption handshake")
	}
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		common.Close(conn)
		close(cancelled)
	})
	encryptedConn, err := h.encryption.Handshake(conn)
	if !stop() {
		<-cancelled
		return nil, E.Cause(ctx.Err(), "encryption handshake")
	}
''')
edit('protocol/group/urltest.go','func (g *URLTestGroup) urlTest(ctx context.Context, force bool) (map[string]uint16, error) {\n', '''func (g *URLTestGroup) urlTest(ctx context.Context, force bool) (map[string]uint16, error) {
	// Manual/API callers may use a process-wide context. Every run also belongs
	// to this group and must end when Close cancels the group's lifetime.
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(g.ctx, cancel)
	defer stop()
	defer cancel()
	if g.ctx.Err() != nil {
		cancel()
	}
''')
(root/'protocol/vless/lx_encryption_cancel_test.go').write_text('''package vless

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
''')
(root/'protocol/group/urltest_owner_cancel_test.go').write_text('''package group

import (
	"context"
	"testing"
	"time"
)

func TestCloseCancelsManualTestWithIndependentContext(t *testing.T) {
	group, node := newHangingGroup(t)
	t.Cleanup(func() { group.Close() })
	done := make(chan struct{})
	go func() {
		_, _ = group.URLTest(context.Background())
		close(done)
	}()
	select {
	case <-node.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("manual test never reached the node")
	}
	if err := group.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("manual test outlived its group")
	}
}
''')
files=['transport/v2ray/transport.go','protocol/vless/lx_encryption.go','protocol/vless/lx_encryption_cancel_test.go','protocol/group/urltest.go','protocol/group/urltest_owner_cancel_test.go']
subprocess.run(['gofmt','-w',*files],check=True)
subprocess.run(['git','diff','--check'],check=True)
print('Runtime fixes ready:',', '.join(files))
