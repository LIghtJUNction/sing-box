package wireguard

// SetKeepIdleConnections keeps the current adapter.OnDemandEndpoint ABI while
// delegating the actual sleep/wake behavior to the lx WireGuard transport.
func (w *Endpoint) SetKeepIdleConnections(keep bool) {
	w.endpoint.SetIdle(!keep)
}
