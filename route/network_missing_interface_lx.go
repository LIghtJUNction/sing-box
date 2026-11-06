package route

// handleMissingDefaultInterface runs with interfaceUpdateAccess held. The
// platform is supplied by the caller so host regressions cover Android too.
func (r *NetworkManager) handleMissingDefaultInterface(android bool) {
	if r.interfaceUpdateCancel != nil {
		r.interfaceUpdateCancel()
		r.interfaceUpdateCancel = nil
	}
	r.networkResetPending = true

	// Android CLI may route sockets successfully through OEM policy tables
	// even when the monitor cannot identify a default interface. When no
	// interface-based routing was requested, keep those socket attempts alive.
	// The monitor remains nil: this does not establish network connectivity.
	if android && r.platformInterface == nil && !r.autoDetectInterface &&
		r.defaultOptions.BindInterface == "" && r.defaultOptions.NetworkStrategy == nil &&
		len(r.defaultOptions.NetworkType) == 0 && len(r.defaultOptions.FallbackNetworkType) == 0 {
		r.pauseManager.NetworkWake()
		r.logger.Info("default interface not detected; using Android system routing")
		return
	}

	r.pauseManager.NetworkPause()
	r.logger.Info("waiting for default network interface")
}
