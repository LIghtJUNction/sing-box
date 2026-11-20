/* SPDX-License-Identifier: MIT
 *
 * lx: Regression for upstream 34ff6c5. Reusing an address shortened by an
 * IPv4 send must overwrite all sixteen bytes on the next IPv6 send.
 */

package conn

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"
)

func TestStdNetBindIPv6AfterIPv4AddressReuse(t *testing.T) {
	sink, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	if err != nil {
		t.Skipf("environment without IPv6 loopback: %v", err)
	}
	defer sink.Close()

	bind := NewStdNetBind(nil).(*StdNetBind)
	if _, _, err := bind.Open(0); err != nil {
		t.Fatal(err)
	}
	defer bind.Close()
	if bind.ipv6 == nil {
		t.Fatal("IPv6 loopback is available but bind has no IPv6 socket")
	}
	// Model a pooled address previously used for IPv6, then IPv4. Keeping
	// nonzero stale tail bytes makes truncation visible on the real send path.
	bind.udpAddrPool = sync.Pool{New: func() any {
		return &net.UDPAddr{IP: net.ParseIP("2001:db8::bad")[:net.IPv4len]}
	}}
	payload := []byte("IPv6 address pool regression")
	endpoint := &StdNetEndpoint{AddrPort: sink.LocalAddr().(*net.UDPAddr).AddrPort()}
	if err := bind.Send([][]byte{payload}, endpoint, 0); err != nil {
		t.Fatalf("IPv6 send after IPv4 address reuse: %v", err)
	}
	if err := sink.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	received := make([]byte, len(payload)+1)
	n, _, err := sink.ReadFromUDP(received)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(received[:n], payload) {
		t.Fatalf("received %q, want %q", received[:n], payload)
	}
}
