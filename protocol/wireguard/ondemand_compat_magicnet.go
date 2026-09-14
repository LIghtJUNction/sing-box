package wireguard

// SetKeepIdleConnections preserves the current adapter.OnDemandEndpoint ABI
// while delegating the actual sleep/wake behavior to the lx WireGuard runtime.
func (w *Endpoint) SetKeepIdleConnections(keep bool) {
	w.endpoint.SetIdle(!keep)
}
