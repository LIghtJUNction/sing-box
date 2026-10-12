//go:build linux

package process

import (
	"encoding/binary"
	"syscall"
)

// socketDiagMessages borrows a validated datagram. Consume it before returning
// the socket-diag receive buffer to its pool. No message slice is allocated.
type socketDiagMessages []byte

func parseSocketDiagMessages(data []byte) (socketDiagMessages, error) {
	remaining := data
	// Validate the entire datagram before exposing a match. A malformed later
	// message must fail even when an earlier message contains the wanted socket.
	for len(remaining) >= syscall.NLMSG_HDRLEN {
		length := uint64(binary.NativeEndian.Uint32(remaining[:4]))
		aligned := (length + syscall.NLMSG_ALIGNTO - 1) &^ uint64(syscall.NLMSG_ALIGNTO-1)
		if length < syscall.NLMSG_HDRLEN || aligned > uint64(len(remaining)) {
			return nil, syscall.EINVAL
		}
		remaining = remaining[int(aligned):]
	}
	// Like syscall.ParseNetlinkMessage, ignore a short trailing header, but
	// require alignment padding for each message with a complete header.
	return socketDiagMessages(data[:len(data)-len(remaining)]), nil
}

// next consumes one message. The caller must first check len(*m) > 0.
func (m *socketDiagMessages) next() syscall.NetlinkMessage {
	data := *m
	header := syscall.NlMsghdr{
		Len:   binary.NativeEndian.Uint32(data[:4]),
		Type:  binary.NativeEndian.Uint16(data[4:6]),
		Flags: binary.NativeEndian.Uint16(data[6:8]),
		Seq:   binary.NativeEndian.Uint32(data[8:12]),
		Pid:   binary.NativeEndian.Uint32(data[12:16]),
	}
	length := int(header.Len) // Bounded by len(data) during validation.
	aligned := (length + syscall.NLMSG_ALIGNTO - 1) &^ (syscall.NLMSG_ALIGNTO - 1)
	message := syscall.NetlinkMessage{Header: header, Data: data[syscall.NLMSG_HDRLEN:length]}
	*m = data[aligned:]
	return message
}
