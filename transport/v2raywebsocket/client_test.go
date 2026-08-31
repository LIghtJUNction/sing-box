package v2raywebsocket

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
)

type pipeDialer struct {
	conn net.Conn
}

func (d *pipeDialer) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	return d.conn, nil
}

func (d *pipeDialer) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, os.ErrInvalid
}

func TestClientDialContextUpgradeFailureReturnsNilConn(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})

	serverResult := make(chan error, 1)
	go func() {
		request, err := http.ReadRequest(bufio.NewReader(serverConn))
		if err != nil {
			serverResult <- err
			return
		}
		_ = request.Body.Close()
		_, err = io.WriteString(serverConn, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
		serverResult <- err
	}()

	client, err := NewClient(
		t.Context(),
		&pipeDialer{conn: clientConn},
		M.ParseSocksaddr("example.com:80"),
		option.V2RayWebsocketOptions{Path: "/"},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := client.DialContext(t.Context())
	if err == nil {
		t.Fatal("DialContext() error = nil, want a WebSocket handshake error")
	}
	if conn != nil {
		t.Fatalf("DialContext() conn = %#v (%T), want a nil net.Conn interface", conn, conn)
	}
	if err := <-serverResult; err != nil {
		t.Fatalf("serve rejecting WebSocket handshake: %v", err)
	}
}
