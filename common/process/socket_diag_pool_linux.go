//go:build linux

package process

import (
	"errors"
	"net/netip"
	"sync/atomic"
)

// Bound persistent descriptors while avoiding one query lock for every
// connection of the same address family and protocol. Sockets open lazily.
const socketDiagPoolSize = 4

type socketDiagPool struct {
	conns [socketDiagPoolSize]socketDiagConn
	next  atomic.Uint32
}

func newSocketDiagPool(family, protocol uint8) *socketDiagPool {
	pool := new(socketDiagPool)
	for i := range pool.conns {
		pool.conns[i].family = family
		pool.conns[i].protocol = protocol
		pool.conns[i].fd = -1
	}
	return pool
}

func (p *socketDiagPool) query(source, destination netip.AddrPort) (uint32, uint32, error) {
	// Each socket still serializes its own request/reply exchange, so no
	// response can be consumed by a different concurrent caller.
	return p.conns[p.next.Add(1)%socketDiagPoolSize].query(source, destination)
}

func (p *socketDiagPool) Close() error {
	var result error
	for i := range p.conns {
		result = errors.Join(result, p.conns[i].Close())
	}
	return result
}
