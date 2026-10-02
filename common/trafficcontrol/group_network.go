package trafficcontrol

import "github.com/sagernet/sing-box/adapter"

// groupTagForNetwork resolves a transport detour through that network's choice.
func groupTagForNetwork(group interface{ Selected(string) adapter.Outbound }, network string) string {
	if selected := group.Selected(network); selected != nil {
		return selected.Tag()
	}
	return ""
}
