package trafficcontrol

// groupTagForNetwork preserves protocol-specific selection for groups such as
// URLTest. This is a routing-time snapshot, not post-dial telemetry.
func groupTagForNetwork(group interface{ Now() string }, network string) string {
	if networkGroup, ok := group.(interface{ NowForNetwork(string) string }); ok {
		return networkGroup.NowForNetwork(network)
	}
	return group.Now()
}
