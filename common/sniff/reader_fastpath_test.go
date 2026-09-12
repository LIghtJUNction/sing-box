package sniff

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/buf"
)

type payloadConn struct {
	net.Conn
	reader bytes.Reader
}

func (c *payloadConn) Read(p []byte) (int, error)      { return c.reader.Read(p) }
func (c *payloadConn) SetReadDeadline(time.Time) error { return nil }

func TestPeekStreamResetsReaders(t *testing.T) {
	for _, cached := range []bool{false, true} {
		conn := &payloadConn{}
		conn.reader.Reset([]byte("payload"))
		buffer := buf.NewPacket()
		var buffers []*buf.Buffer
		want := "payload"
		if cached {
			prefix := buf.NewPacket()
			_, _ = prefix.WriteString("prefix")
			defer prefix.Release()
			buffers = []*buf.Buffer{prefix}
			want = "prefixpayload"
		}
		calls := 0
		sniffer := func(_ context.Context, _ *adapter.InboundContext, r io.Reader) error {
			calls++
			data, err := io.ReadAll(r)
			if err != nil || string(data) != want {
				t.Fatalf("cached=%v: got %q, %v", cached, data, err)
			}
			if calls == 1 {
				return errors.New("try next protocol")
			}
			return nil
		}
		err := PeekStream(context.Background(), &adapter.InboundContext{}, conn, buffers, buffer, time.Second, sniffer, sniffer)
		buffer.Release()
		if err != nil || calls != 2 {
			t.Fatalf("calls=%d error=%v", calls, err)
		}
	}
}

func BenchmarkPeekStreamSingleBuffer(b *testing.B) {
	conn := &payloadConn{}
	buffer := buf.NewPacket()
	defer buffer.Release()
	metadata := &adapter.InboundContext{}
	payload := []byte("not a recognized protocol")
	rejected := errors.New("unrecognized")
	var header [8]byte
	sniffer := func(_ context.Context, _ *adapter.InboundContext, r io.Reader) error {
		_, _ = io.ReadFull(r, header[:])
		return rejected
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.Reset()
		conn.reader.Reset(payload)
		_ = PeekStream(context.Background(), metadata, conn, nil, buffer, time.Second, sniffer, sniffer, sniffer, sniffer, sniffer, sniffer)
	}
}
