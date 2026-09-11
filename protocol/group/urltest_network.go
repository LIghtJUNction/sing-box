package group

// NowForNetwork reports the selection used by DialContext/ListenPacket for a
// particular network. Now remains the legacy TCP-preferred group API view.
func (s *URLTest) NowForNetwork(network string) string {
	if s.group == nil {
		return ""
	}
	switch network {
	case "tcp":
		if outbound := s.group.selectedOutboundTCP; outbound != nil {
			return outbound.Tag()
		}
	case "udp":
		if outbound := s.group.selectedOutboundUDP; outbound != nil {
			return outbound.Tag()
		}
	default:
		return ""
	}
	// Mirror the existing dial-time fallback before a selection is cached.
	if outbound, _ := s.group.Select(network); outbound != nil {
		return outbound.Tag()
	}
	return ""
}
