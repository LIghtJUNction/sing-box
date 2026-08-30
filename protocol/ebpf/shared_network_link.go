//go:build with_ebpf && (linux || android)

package ebpf

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/sagernet/netlink"
	ECommon "github.com/sagernet/sing-box/common/ebpf"
	E "github.com/sagernet/sing/common/exceptions"

	"golang.org/x/sys/unix"
)

func attachSharedTC(
	link netlink.Link,
	backend *ECommon.SharedNetworkBackend,
	enableIPv4 bool,
	priority uint16,
) (*sharedTCAttachment, error) {
	interfaceLock, err := acquireSharedTCInterfaceLock(link.Attrs().Name, link.Attrs().Index)
	if err != nil {
		return nil, err
	}
	closeInterfaceLock := true
	defer func() {
		if closeInterfaceLock {
			_ = interfaceLock.Close()
		}
	}()
	attachment := &sharedTCAttachment{
		interfaceName:  link.Attrs().Name,
		interfaceIndex: link.Attrs().Index,
		interfaceLock:  interfaceLock,
	}
	rollback := func(startErr error) (*sharedTCAttachment, error) {
		closeInterfaceLock = false
		cleanupErr := detachSharedTCAttachment(attachment)
		if cleanupErr != nil {
			return attachment, E.Errors(startErr, E.Cause(cleanupErr, "roll back shared-network TC attachment"))
		}
		return nil, startErr
	}
	if enableIPv4 {
		attachment.restoreRouteLocalnet, err = enableSharedRouteLocalnet(link.Attrs().Name)
		if err != nil {
			return rollback(err)
		}
	}
	if priority == defaultSharedNetworkTCPriority {
		var supported bool
		var tcxErr error
		attachment.tcx, supported, tcxErr = backend.TryAttachTCX(link.Attrs().Index)
		if tcxErr != nil {
			return rollback(tcxErr)
		}
		if supported {
			closeInterfaceLock = false
			return attachment, nil
		}
	}
	attachment.removeClsact, err = ensureClsact(link)
	if err != nil {
		return rollback(err)
	}
	attachment.egress, _, err = attachSharedTCFilter(
		link,
		netlink.HANDLE_MIN_EGRESS,
		backend.EgressProgramFD(),
		"sb_share_out",
		sharedEgressFilterHandle,
		priority,
		0,
		true,
	)
	if err != nil {
		return rollback(err)
	}
	attachment.ingress, _, err = attachSharedTCFilter(
		link,
		netlink.HANDLE_MIN_INGRESS,
		backend.IngressProgramFD(),
		"sb_share_in",
		sharedIngressFilterHandle,
		priority,
		0,
		true,
	)
	if err != nil {
		return rollback(err)
	}
	closeInterfaceLock = false
	return attachment, nil
}

func acquireSharedTCInterfaceLock(interfaceName string, interfaceIndex int) (*net.UnixConn, error) {
	address := &net.UnixAddr{
		Name: "@sing-box-ebpf-shared-" + fmt.Sprint(interfaceIndex),
		Net:  "unixgram",
	}
	connection, err := net.ListenUnixgram("unixgram", address)
	if err != nil {
		if errors.Is(err, unix.EADDRINUSE) {
			return nil, E.New("shared-network interface ", interfaceName, " is already managed by another eBPF inbound")
		}
		return nil, E.Cause(err, "lock shared-network interface ", interfaceName)
	}
	return connection, nil
}

func sharedRouteLocalnetPath(interfaceName string) string {
	return "/proc/sys/net/ipv4/conf/" + interfaceName + "/route_localnet"
}

func enableSharedRouteLocalnet(interfaceName string) (bool, error) {
	path := sharedRouteLocalnetPath(interfaceName)
	value, err := os.ReadFile(path)
	if err != nil {
		return false, E.Cause(err, "read route_localnet for ", interfaceName)
	}
	if strings.TrimSpace(string(value)) == "1" {
		return false, nil
	}
	if strings.TrimSpace(string(value)) != "0" {
		return false, E.New("unexpected route_localnet value for ", interfaceName)
	}
	if err = os.WriteFile(path, []byte("1"), 0o644); err != nil {
		return false, E.Cause(err, "enable route_localnet for ", interfaceName)
	}
	return true, nil
}

func restoreSharedRouteLocalnet(interfaceName string) error {
	path := sharedRouteLocalnetPath(interfaceName)
	value, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return E.Cause(err, "read route_localnet for ", interfaceName)
	}
	if strings.TrimSpace(string(value)) != "1" {
		return nil
	}
	if err = os.WriteFile(path, []byte("0"), 0o644); err != nil {
		return E.Cause(err, "restore route_localnet for ", interfaceName)
	}
	return nil
}

func attachSharedTCFilter(
	link netlink.Link,
	parent uint32,
	programFD int,
	programName string,
	handle uint16,
	priority uint16,
	expectedProgramID int,
	replaceExisting bool,
) (*netlink.BpfFilter, bool, error) {
	if programFD < 0 {
		return nil, false, E.New("shared-network eBPF program is unavailable")
	}
	filters, err := netlink.FilterList(link, parent)
	if err != nil {
		return nil, false, err
	}
	filterHandle := netlink.MakeHandle(0, handle)
	for _, existing := range filters {
		bpfFilter, isBPF := existing.(*netlink.BpfFilter)
		if isBPF && bpfFilter.Name == programName {
			if !replaceExisting && sharedTCFilterMatches(
				bpfFilter,
				link.Attrs().Index,
				parent,
				filterHandle,
				priority,
				programName,
				expectedProgramID,
			) {
				return bpfFilter, false, nil
			}
			if err = netlink.FilterDel(existing); err != nil && !errors.Is(err, unix.ENOENT) {
				return nil, false, err
			}
			continue
		}
		if existing.Attrs().Handle == filterHandle {
			return nil, false, E.New("TC filter handle conflict on ", link.Attrs().Name)
		}
	}
	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    parent,
			Handle:    filterHandle,
			Priority:  priority,
			Protocol:  unix.ETH_P_ALL,
		},
		Fd:           programFD,
		Name:         programName,
		DirectAction: true,
	}
	if err = netlink.FilterAdd(filter); err != nil {
		return nil, false, err
	}
	filters, err = netlink.FilterList(link, parent)
	if err != nil {
		if rollbackErr := detachSharedTCFilter(filter); rollbackErr != nil {
			return filter, true, E.Errors(err, E.Cause(rollbackErr, "roll back unverified TC filter"))
		}
		return nil, false, err
	}
	for _, existing := range filters {
		bpfFilter, isBPF := existing.(*netlink.BpfFilter)
		if isBPF && sharedTCFilterMatches(
			bpfFilter,
			link.Attrs().Index,
			parent,
			filterHandle,
			priority,
			programName,
			0,
		) {
			return bpfFilter, true, nil
		}
	}
	notVisibleErr := E.New("new shared-network TC filter is not visible on ", link.Attrs().Name)
	if rollbackErr := detachSharedTCFilter(filter); rollbackErr != nil {
		return filter, true, E.Errors(notVisibleErr, E.Cause(rollbackErr, "roll back unverified TC filter"))
	}
	return nil, false, notVisibleErr
}

func repairSharedTC(
	link netlink.Link,
	attachment *sharedTCAttachment,
	backend *ECommon.SharedNetworkBackend,
	priority uint16,
) (bool, error) {
	createdClsact, err := ensureClsact(link)
	if err != nil {
		return false, err
	}
	repaired := createdClsact
	if createdClsact {
		attachment.removeClsact = true
	}
	egress, egressRepaired, err := attachSharedTCFilter(
		link,
		netlink.HANDLE_MIN_EGRESS,
		backend.EgressProgramFD(),
		"sb_share_out",
		sharedEgressFilterHandle,
		priority,
		sharedTCFilterProgramID(attachment.egress),
		false,
	)
	if egress != nil {
		attachment.egress = egress
	}
	if err != nil {
		return false, err
	}
	repaired = repaired || egressRepaired
	ingress, ingressRepaired, err := attachSharedTCFilter(
		link,
		netlink.HANDLE_MIN_INGRESS,
		backend.IngressProgramFD(),
		"sb_share_in",
		sharedIngressFilterHandle,
		priority,
		sharedTCFilterProgramID(attachment.ingress),
		false,
	)
	if ingress != nil {
		attachment.ingress = ingress
	}
	if err != nil {
		return repaired, err
	}
	repaired = repaired || ingressRepaired
	return repaired, nil
}

func repairSharedTCAttachment(
	link netlink.Link,
	attachment *sharedTCAttachment,
	backend *ECommon.SharedNetworkBackend,
	enableIPv4 bool,
	priority uint16,
) (bool, error) {
	repaired := false
	if enableIPv4 {
		routeLocalnetChanged, err := enableSharedRouteLocalnet(link.Attrs().Name)
		if err != nil {
			return false, err
		}
		if routeLocalnetChanged {
			attachment.restoreRouteLocalnet = true
			repaired = true
		}
	}
	if attachment.tcx != nil {
		tcxRepaired, err := backend.RepairTCX(attachment.tcx, link.Attrs().Index)
		return repaired || tcxRepaired, err
	}
	tcRepaired, err := repairSharedTC(link, attachment, backend, priority)
	return repaired || tcRepaired, err
}

func sharedTCFilterMatches(
	filter *netlink.BpfFilter,
	linkIndex int,
	parent uint32,
	handle uint32,
	priority uint16,
	name string,
	programID int,
) bool {
	if filter == nil {
		return false
	}
	attributes := filter.Attrs()
	return attributes != nil &&
		attributes.LinkIndex == linkIndex &&
		attributes.Parent == parent &&
		attributes.Handle == handle &&
		attributes.Priority == priority &&
		attributes.Protocol == unix.ETH_P_ALL &&
		filter.Name == name &&
		filter.DirectAction &&
		(programID == 0 || filter.Id == programID)
}

func sharedTCFilterProgramID(filter *netlink.BpfFilter) int {
	if filter == nil {
		return 0
	}
	return filter.Id
}

func detachSharedTCFilter(filter *netlink.BpfFilter) error {
	if filter == nil {
		return nil
	}
	err := netlink.FilterDel(filter)
	if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENODEV) || errors.Is(err, unix.ESRCH) {
		return nil
	}
	return err
}

func ensureClsact(link netlink.Link) (bool, error) {
	qdiscs, err := netlink.QdiscList(link)
	if err != nil {
		return false, err
	}
	for _, qdisc := range qdiscs {
		if qdisc.Type() == "clsact" {
			return false, nil
		}
	}
	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err = netlink.QdiscAdd(qdisc); errors.Is(err, unix.EEXIST) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func releaseSharedClsact(interfaceIndex int) error {
	link, err := netlink.LinkByIndex(interfaceIndex)
	if isSharedNetworkLinkNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, parent := range []uint32{netlink.HANDLE_MIN_INGRESS, netlink.HANDLE_MIN_EGRESS} {
		filters, filterErr := netlink.FilterList(link, parent)
		if isSharedNetworkLinkNotFound(filterErr) {
			return nil
		}
		if filterErr != nil {
			return filterErr
		}
		for _, filter := range filters {
			bpfFilter, isBPF := filter.(*netlink.BpfFilter)
			if !isBPF {
				continue
			}
			handle := filter.Attrs().Handle
			if (bpfFilter.Name == "sb_share_in" && handle == netlink.MakeHandle(0, sharedIngressFilterHandle)) ||
				(bpfFilter.Name == "sb_share_out" && handle == netlink.MakeHandle(0, sharedEgressFilterHandle)) {
				return E.New("sing-box-owned TC filter remained on ", link.Attrs().Name)
			}
		}
		if len(filters) > 0 {
			return nil
		}
	}
	qdiscs, err := netlink.QdiscList(link)
	if isSharedNetworkLinkNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, qdisc := range qdiscs {
		if qdisc.Type() != "clsact" {
			continue
		}
		err = netlink.QdiscDel(qdisc)
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENODEV) || errors.Is(err, unix.ESRCH) {
			return nil
		}
		return err
	}
	return nil
}

func sharedHostAddresses() ([]netip.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, E.Cause(err, "list interfaces for shared-network host bypass")
	}
	var addresses []netip.Addr
	for _, networkInterface := range interfaces {
		interfaceAddresses, addressErr := networkInterface.Addrs()
		if addressErr != nil {
			return nil, E.Cause(addressErr, "list addresses for interface ", networkInterface.Name)
		}
		for _, interfaceAddress := range interfaceAddresses {
			prefix, parseErr := netip.ParsePrefix(interfaceAddress.String())
			if parseErr == nil {
				addresses = append(addresses, prefix.Addr().Unmap())
			}
		}
	}
	return addresses, nil
}
