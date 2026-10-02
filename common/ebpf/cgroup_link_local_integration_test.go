//go:build with_ebpf && (linux || android) && ebpf_integration

package ebpf

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const integrationLinkLocalHelperEnv = "SING_BOX_EBPF_LINK_LOCAL_HELPER"

func TestCgroupLinkLocalSafetyIntegration(t *testing.T) {
	requireEBPFIntegration(t, "preserve IPv4, mapped IPv6 and native IPv6 link-local destinations")
	mount, err := DetectCgroup2Mount()
	if err != nil {
		t.Fatal(err)
	}
	path, err := os.MkdirTemp(mount, "sing-box-ebpf-link-local-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil {
			t.Errorf("remove link-local test cgroup: %v", err)
		}
	})
	backend, err := PrepareCgroup(CgroupConfig{
		Path: path, EnableUDP: true, EnableIPv6: true,
		RedirectIPv4: netip.MustParsePrefix("127.128.0.0/9"),
		RedirectIPv6: netip.MustParsePrefix("fd53:696e:672d:626f::/64"),
		MapCapacity:  DefaultCgroupMapCapacity(), UDPTimeout: 5 * time.Minute,
		Policy: CgroupPolicy{DNSMode: DNSModeOff, BypassPrivateAddress: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close link-local test backend: %v", err)
		}
	})
	if err := backend.LoadPrograms(65531); err != nil {
		t.Fatal(err)
	}
	if err := backend.Attach(); err != nil {
		t.Fatal(err)
	}
	commands, commandWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer commandWriter.Close()
	reports, reportWriter, err := os.Pipe()
	if err != nil {
		commands.Close()
		t.Fatal(err)
	}
	defer reports.Close()
	var helperOutput bytes.Buffer
	helper := exec.Command(os.Args[0], "-test.run=^TestCgroupLinkLocalSafetyHelper$")
	helper.Env = append(os.Environ(), integrationLinkLocalHelperEnv+"=1")
	helper.ExtraFiles = []*os.File{commands, reportWriter}
	helper.Stdout, helper.Stderr = &helperOutput, &helperOutput
	if err := helper.Start(); err != nil {
		commands.Close()
		reportWriter.Close()
		t.Fatal(err)
	}
	commands.Close()
	reportWriter.Close()
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = helper.Process.Kill()
			_ = helper.Wait()
		}
	})
	if err := os.WriteFile(filepath.Join(path, "cgroup.procs"), []byte(strconv.Itoa(helper.Process.Pid)), 0); err != nil {
		t.Fatal(err)
	}
	advance := func() {
		t.Helper()
		if _, err := commandWriter.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
		if err := reports.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(reports, make([]byte, 1)); err != nil {
			t.Fatalf("link-local helper: %v: %s", err, helperOutput.Bytes())
		}
	}
	readOriginals := func() map[netip.AddrPort]int {
		t.Helper()
		originals := make(map[netip.AddrPort]int)
		var key listenerLookupKey
		var value originalDestinationValue
		iterator := backend.runtime.maps["cgroup_udp_redirect"].Iterate()
		for iterator.Next(&key, &value) {
			original, err := originalDestinationFromValueForProtocol(value, ProtocolUDP)
			if err != nil {
				t.Fatal(err)
			}
			originals[original.Destination]++
		}
		if err := iterator.Err(); err != nil {
			t.Fatal(err)
		}
		return originals
	}
	advance()
	if originals := readOriginals(); len(originals) != 0 {
		t.Fatalf("link-local connects were captured: %v", originals)
	}
	advance()
	originals := readOriginals()
	for _, address := range []string{"192.168.1.2:443", "192.168.1.3:443", "[fd10::1]:443"} {
		if originals[netip.MustParseAddrPort(address)] != 1 {
			t.Fatalf("ordinary private connect was not captured exactly once: destination=%s records=%v", address, originals)
		}
	}
	if len(originals) != 3 {
		t.Fatalf("unexpected captured destinations: %v", originals)
	}
	if _, err := commandWriter.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	if err := helper.Wait(); err != nil {
		waited = true
		t.Fatalf("link-local helper: %v: %s", err, helperOutput.Bytes())
	}
	waited = true
}

func TestCgroupLinkLocalSafetyHelper(t *testing.T) {
	if os.Getenv(integrationLinkLocalHelperEnv) != "1" {
		t.Skip("link-local connect helper")
	}
	commands := os.NewFile(3, "cgroup-commands")
	reports := os.NewFile(4, "cgroup-reports")
	defer commands.Close()
	defer reports.Close()
	var descriptors []int
	defer func() {
		for _, descriptor := range descriptors {
			_ = unix.Close(descriptor)
		}
	}()
	loopback, err := net.InterfaceByName("lo")
	if err != nil {
		t.Fatal(err)
	}
	for _, batch := range [][]string{
		{"169.254.169.254", "::ffff:169.254.1.1", "fe80::1"},
		{"192.168.1.2", "::ffff:192.168.1.3", "fd10::1"},
	} {
		if _, err := io.ReadFull(commands, make([]byte, 1)); err != nil {
			t.Fatal(err)
		}
		for _, text := range batch {
			address := netip.MustParseAddr(text)
			family := unix.AF_INET6
			if address.Is4() {
				family = unix.AF_INET
			}
			descriptor, err := unix.Socket(family, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.IPPROTO_UDP)
			if err != nil {
				t.Fatal(err)
			}
			descriptors = append(descriptors, descriptor)
			var destination unix.Sockaddr
			if address.Is4() {
				destination = &unix.SockaddrInet4{Port: 443, Addr: address.As4()}
			} else {
				if err := unix.SetsockoptInt(descriptor, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY, 0); err != nil {
					t.Fatal(err)
				}
				ipv6 := &unix.SockaddrInet6{Port: 443, Addr: address.As16()}
				if address.IsLinkLocalUnicast() {
					ipv6.ZoneId = uint32(loopback.Index)
				}
				destination = ipv6
			}
			// Connect runs the cgroup hook but sends no datagrams. Route failures
			// after the hook are harmless; open FDs retain the redirect evidence.
			if err := unix.Connect(descriptor, destination); err != nil &&
				!errors.Is(err, unix.ENETUNREACH) && !errors.Is(err, unix.EHOSTUNREACH) &&
				!errors.Is(err, unix.EADDRNOTAVAIL) && !errors.Is(err, unix.EINPROGRESS) {
				t.Fatalf("connect %s: %v", address, err)
			}
		}
		if _, err := reports.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := io.ReadFull(commands, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
}
