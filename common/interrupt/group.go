package interrupt

import (
	"io"
	"net"
	"sync"

	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/x/list"
)

type Group struct {
	access      sync.Mutex
	connections list.List[*groupConnItem]
}

type groupConnItem struct {
	conn       io.Closer
	isExternal bool
}

func NewGroup() *Group {
	return &Group{}
}

func (g *Group) NewConn(conn net.Conn, isExternal bool) net.Conn {
	g.access.Lock()
	defer g.access.Unlock()
	item := g.connections.PushBack(&groupConnItem{conn, isExternal})
	return &Conn{Conn: conn, group: g, element: item}
}

func (g *Group) NewPacketConn(conn net.PacketConn, isExternal bool) net.PacketConn {
	g.access.Lock()
	defer g.access.Unlock()
	item := g.connections.PushBack(&groupConnItem{conn, isExternal})
	return &PacketConn{PacketConn: conn, group: g, element: item}
}

// N.PacketConn variant used by selector packet connections.
func (g *Group) NewSingPacketConn(conn N.PacketConn, isExternal bool) N.PacketConn {
	g.access.Lock()
	defer g.access.Unlock()
	item := g.connections.PushBack(&groupConnItem{conn, isExternal})
	return &SingPacketConn{PacketConn: conn, group: g, element: item}
}

// The underlying Close must run outside g.access. A registered connection may
// itself be another group's wrapper, so closing under the mutex can take two
// group locks in opposite order and deadlock. Detach first, close afterwards.
func (g *Group) Interrupt(interruptExternalConnections bool) {
	g.access.Lock()
	var toClose []io.Closer
	for element := g.connections.Front(); element != nil; {
		next := element.Next()
		if !element.Value.isExternal || interruptExternalConnections {
			toClose = append(toClose, element.Value.conn)
			g.connections.Remove(element)
		}
		element = next
	}
	g.access.Unlock()
	for _, conn := range toClose {
		conn.Close()
	}
}
