package wireguard

// SetIdle preserves the current testing branch's on-demand endpoint contract
// while the lx runtime owns the richer suspend/resume implementation.
func (e *Endpoint) SetIdle(idle bool) {
	if idle {
		e.Suspend()
		return
	}
	// Match the current upstream behavior: non-system endpoints resume lazily
	// on the next dial; system endpoints must be brought back immediately.
	if e.options.System {
		_ = e.Resume()
	}
}
