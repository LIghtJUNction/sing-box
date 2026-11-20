//go:build with_ebpf && (linux || android) && ebpf_integration

package ebpf

import (
	"errors"
	"net/netip"
	"testing"
	"unsafe"

	CiliumEBPF "github.com/cilium/ebpf"
	"golang.org/x/sys/unix"
)

func TestCgroupOriginalRecordValidationIntegration(t *testing.T) {
	requireEBPFIntegration(t, "validate original destination records before recovery")
	newMap := func(mapType CiliumEBPF.MapType, keySize, valueSize uintptr) int {
		t.Helper()
		instance, err := CiliumEBPF.NewMap(&CiliumEBPF.MapSpec{
			Type: mapType, KeySize: uint32(keySize), ValueSize: uint32(valueSize), MaxEntries: 8,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = instance.Close() })
		return instance.FD()
	}
	backend := &CgroupBackend{
		runtime:          &cgroupRuntime{},
		tcpRedirectMapFD: newMap(CiliumEBPF.Hash, unsafe.Sizeof(listenerLookupKey{}), unsafe.Sizeof(originalDestinationValue{})),
		udpRedirectMapFD: newMap(CiliumEBPF.Hash, unsafe.Sizeof(listenerLookupKey{}), unsafe.Sizeof(originalDestinationValue{})),
		udpRecoveryMapFD: newMap(CiliumEBPF.LRUHash, unsafe.Sizeof(listenerLookupKey{}), unsafe.Sizeof(originalDestinationValue{})),
		udpFlowMapFD:     newMap(CiliumEBPF.LRUHash, unsafe.Sizeof(udpFlowKey{}), unsafe.Sizeof(udpFlowValue{})),
	}
	listener := netip.MustParseAddrPort("127.128.10.40:5300")
	key, err := makeListenerLookupKey(ProtocolUDP, listener)
	if err != nil {
		t.Fatal(err)
	}
	valid := originalDestinationValue{
		Family: addressFamilyIPv4, Protocol: ProtocolUDP, Port: 53,
		Addr: [16]byte{192, 0, 2, 53},
	}
	put := func(fd int, value originalDestinationValue) {
		t.Helper()
		if err := updateMap(fd, unsafe.Pointer(&key), unsafe.Pointer(&value)); err != nil {
			t.Fatal(err)
		}
	}
	assertMissing := func(fd int) {
		t.Helper()
		var value originalDestinationValue
		if err := lookupMap(fd, unsafe.Pointer(&key), unsafe.Pointer(&value)); !errors.Is(err, unix.ENOENT) {
			t.Fatalf("malformed record was published: value=%+v err=%v", value, err)
		}
	}
	malformed := []struct {
		name   string
		change func(*originalDestinationValue)
	}{
		{"protocol_mismatch", func(value *originalDestinationValue) { value.Protocol = ProtocolTCP }},
		{"unknown_protocol", func(value *originalDestinationValue) { value.Protocol = 255 }},
		{"zero_port", func(value *originalDestinationValue) { value.Port = 0 }},
		{"connected_TCP", func(value *originalDestinationValue) {
			value.Protocol = ProtocolTCP
			value.Flags = originalDestinationFlagConnectedUDP
		}},
		{"unknown_flags", func(value *originalDestinationValue) { value.Flags = 2 }},
	}
	for _, test := range malformed {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			test.change(&value)
			put(backend.udpRedirectMapFD, value)
			if _, err := backend.LookupOriginal(ProtocolUDP, listener); err == nil {
				t.Fatal("lookup accepted malformed record")
			}
			if err := backend.DeleteRedirect(ProtocolUDP, listener); err == nil {
				t.Fatal("cleanup retained malformed recovery record")
			}
			assertMissing(backend.udpRecoveryMapFD)
			assertMissing(backend.udpRedirectMapFD)
			put(backend.udpRedirectMapFD, value)
			if _, err := backend.TakeOriginal(ProtocolUDP, listener); err == nil {
				t.Fatal("take accepted malformed record")
			}
			assertMissing(backend.udpRedirectMapFD)
			for _, mode := range []int32{mapLookupAndDeleteUnknown, mapLookupAndDeleteUnsupported} {
				backend.udpRecoveryConsumeMode.Store(mode)
				put(backend.udpRecoveryMapFD, value)
				if _, err := backend.RecoverUDPOriginal(listener); err == nil {
					t.Fatal("recovery accepted malformed record")
				}
				assertMissing(backend.udpRedirectMapFD)
				if backend.udpRecoveryConsumeMode.Load() == mapLookupAndDeleteSupported {
					assertMissing(backend.udpRecoveryMapFD)
				} else {
					var retained originalDestinationValue
					if err := lookupMap(backend.udpRecoveryMapFD, unsafe.Pointer(&key), unsafe.Pointer(&retained)); err != nil || retained != value {
						t.Fatalf("lookup-only recovery changed its bounded input: value=%+v err=%v", retained, err)
					}
				}
				if err := deleteMap(backend.udpRecoveryMapFD, unsafe.Pointer(&key)); err != nil && !errors.Is(err, unix.ENOENT) {
					t.Fatal(err)
				}
			}
		})
	}
	t.Run("concurrent_invalid_redirect", func(t *testing.T) {
		for _, mode := range []int32{mapLookupAndDeleteUnknown, mapLookupAndDeleteUnsupported} {
			backend.udpRecoveryConsumeMode.Store(mode)
			put(backend.udpRecoveryMapFD, valid)
			invalid := valid
			invalid.Protocol = ProtocolTCP
			put(backend.udpRedirectMapFD, invalid)
			if _, err := backend.RecoverUDPOriginal(listener); err == nil {
				t.Fatal("recovery accepted a concurrently claimed invalid redirect")
			}
			var loaded originalDestinationValue
			if err := lookupMap(backend.udpRedirectMapFD, unsafe.Pointer(&key), unsafe.Pointer(&loaded)); err != nil || loaded != invalid {
				t.Fatalf("recovery overwrote concurrent owner: value=%+v err=%v", loaded, err)
			}
			if err := lookupMap(backend.udpRecoveryMapFD, unsafe.Pointer(&key), unsafe.Pointer(&loaded)); err != nil || loaded != valid {
				t.Fatalf("valid recovery input was lost: value=%+v err=%v", loaded, err)
			}
			if err := deleteMap(backend.udpRedirectMapFD, unsafe.Pointer(&key)); err != nil {
				t.Fatal(err)
			}
			original, err := backend.RecoverUDPOriginal(listener)
			if err != nil || original.Destination != netip.MustParseAddrPort("192.0.2.53:53") {
				t.Fatalf("retained valid record could not recover: original=%+v err=%v", original, err)
			}
			if err := deleteMap(backend.udpRedirectMapFD, unsafe.Pointer(&key)); err != nil {
				t.Fatal(err)
			}
		}
	})
}
