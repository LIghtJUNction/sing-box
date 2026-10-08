//go:build linux

package process

import (
	"fmt"
	"net/netip"
	"syscall"
	"testing"
)

var socketDiagRequestChecksum byte

func BenchmarkSocketDiagRequestEncoding(b *testing.B) {
	for _, family := range []byte{syscall.AF_INET, syscall.AF_INET6} {
		for _, protocol := range []byte{syscall.IPPROTO_TCP, syscall.IPPROTO_UDP} {
			b.Run(fmt.Sprintf("%d/%d", family, protocol), func(b *testing.B) {
				source := netip.MustParseAddrPort("192.0.2.1:12345")
				destination := netip.MustParseAddrPort("198.51.100.2:443")
				if family == syscall.AF_INET6 {
					source = netip.MustParseAddrPort("[2001:db8::1]:12345")
					destination = netip.MustParseAddrPort("[2001:db8::2]:443")
				}
				var checksum byte
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					request := packSocketDiagRequest(family, protocol, source, destination, false)
					checksum ^= request[24]
				}
				socketDiagRequestChecksum = checksum
			})
		}
	}
}
