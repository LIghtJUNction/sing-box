package sniff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/buf"
)

type probeRejection struct {
	id int
}

func (e *probeRejection) Error() string { return fmt.Sprintf("probe-%d", e.id) }

func TestPeekStreamPreservesRejectedErrors(t *testing.T) {
	conn := &payloadConn{}
	conn.reader.Reset([]byte("payload"))
	buffer := buf.NewPacket()
	defer buffer.Release()
	var sniffers []StreamSniffer
	var rejected []error
	var messages []string
	for i := 0; i < 11; i++ {
		err := fmt.Errorf("wrapped: %w", &probeRejection{id: i})
		rejected = append(rejected, err)
		messages = append(messages, err.Error())
		sniffers = append(sniffers, func(context.Context, *adapter.InboundContext, io.Reader) error {
			return err
		})
	}
	// Duplicate evidence remains present only once, in its first position.
	sniffers = append(sniffers, sniffers[0])
	err := PeekStream(context.Background(), &adapter.InboundContext{}, conn, nil, buffer, time.Second, sniffers...)
	want := "(" + strings.Join(messages, " | ") + ")"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %s", err, want)
	}
	for _, target := range rejected {
		if !errors.Is(err, target) {
			t.Fatalf("lost error %v in %v", target, err)
		}
	}
	var first *probeRejection
	if !errors.As(err, &first) || first.id != 0 {
		t.Fatalf("lost first typed error: %v in %v", first, err)
	}
}

type segmentedPayloadConn struct {
	payloadConn
	segments  [][]byte
	deadlines []time.Time
}

func (c *segmentedPayloadConn) Read(p []byte) (int, error) {
	if len(c.segments) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.segments[0])
	c.segments = c.segments[1:]
	return n, nil
}

func (c *segmentedPayloadConn) SetReadDeadline(deadline time.Time) error {
	c.deadlines = append(c.deadlines, deadline)
	return nil
}

func TestPeekStreamRetriesNeedMoreData(t *testing.T) {
	conn := &segmentedPayloadConn{segments: [][]byte{[]byte("part"), []byte("two")}}
	buffer := buf.NewPacket()
	defer buffer.Release()
	calls := 0
	reject := func(context.Context, *adapter.InboundContext, io.Reader) error {
		return errors.New("other protocol")
	}
	accept := func(_ context.Context, _ *adapter.InboundContext, r io.Reader) error {
		calls++
		payload, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if string(payload) == "part" {
			return fmt.Errorf("partial header: %w", ErrNeedMoreData)
		}
		if string(payload) != "parttwo" {
			t.Fatalf("got payload %q", payload)
		}
		return nil
	}
	err := PeekStream(context.Background(), &adapter.InboundContext{}, conn, nil, buffer, time.Second, reject, accept)
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
	if len(conn.deadlines) != 4 || !conn.deadlines[0].Equal(conn.deadlines[2]) || !conn.deadlines[1].IsZero() || !conn.deadlines[3].IsZero() {
		t.Fatalf("read budget changed or deadline was not cleared: %v", conn.deadlines)
	}
}

func BenchmarkPeekStreamErrorAggregation(b *testing.B) {
	for _, accepted := range []bool{false, true} {
		for _, cached := range []bool{false, true} {
			b.Run(fmt.Sprintf("accepted=%v/cached=%v", accepted, cached), func(b *testing.B) {
				conn := &payloadConn{}
				buffer := buf.NewPacket()
				defer buffer.Release()
				var buffers []*buf.Buffer
				if cached {
					prefix := buf.NewPacket()
					defer prefix.Release()
					_, _ = prefix.WriteString("prefix")
					buffers = []*buf.Buffer{prefix}
				}
				var sniffers []StreamSniffer
				for i := 0; i < 8; i++ {
					rejected := fmt.Errorf("rejected-%d", i)
					sniffers = append(sniffers, func(context.Context, *adapter.InboundContext, io.Reader) error {
						if accepted {
							return nil
						}
						return rejected
					})
				}
				payload := []byte("not a recognized protocol")
				metadata := &adapter.InboundContext{}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					buffer.Reset()
					conn.reader.Reset(payload)
					_ = PeekStream(context.Background(), metadata, conn, buffers, buffer, time.Second, sniffers...)
				}
			})
		}
	}
}

func BenchmarkPeekStreamHTTPHost(b *testing.B) {
	conn := &payloadConn{}
	buffer := buf.NewPacket()
	defer buffer.Release()
	metadata := &adapter.InboundContext{}
	payload := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.Reset()
		conn.reader.Reset(payload)
		if err := PeekStream(context.Background(), metadata, conn, nil, buffer, time.Second, TLSClientHello, HTTPHost); err != nil {
			b.Fatal(err)
		}
	}
}
