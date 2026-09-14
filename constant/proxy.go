package constant

const (
	TypeTun                = "tun"
	TypeEBPF               = "ebpf"
	TypeRedirect           = "redirect"
	TypeTProxy             = "tproxy"
	TypeDirect             = "direct"
	TypeBridge             = "bridge"
	TypeBlock              = "block"
	TypeDNS                = "dns"
	TypeSOCKS              = "socks"
	TypeHTTP               = "http"
	TypeMixed              = "mixed"
	TypeShadowsocks        = "shadowsocks"
	TypeSnell              = "snell"
	TypeVMess              = "vmess"
	TypeTrojan             = "trojan"
	TypeTrustTunnel        = "trusttunnel"
	TypeNaive              = "naive"
	TypeWireGuard          = "wireguard"
	TypeWARP               = "warp"
	TypeMASQUE             = "masque"
	TypeMTProxy            = "mtproxy"
	TypeParser             = "parser"
	TypeHysteria           = "hysteria"
	TypeTor                = "tor"
	TypeSSH                = "ssh"
	TypeShadowTLS          = "shadowtls"
	TypeMieru              = "mieru"
	TypeAnyTLS             = "anytls"
	TypeSudoku             = "sudoku"
	TypeCall               = "call"
	TypeShadowsocksR       = "shadowsocksr"
	TypeVLESS              = "vless"
	TypeTUIC               = "tuic"
	TypeHysteria2          = "hysteria2"
	TypeMASQUE             = "masque"
	TypeOpenConnect        = "openconnect"
	TypeOpenVPNClient      = "openvpn-client"
	TypeOpenVPNServer      = "openvpn-server"
	TypeBond               = "bond"
	TypeFailover           = "failover"
	TypeVPNServer          = "vpn-server"
	TypeVPNClient          = "vpn-client"
	TypeTailscale          = "tailscale"
	TypeCloudflared        = "cloudflared"
	TypeConnectionLimiter  = "connection-limiter"
	TypeBandwidthLimiter   = "bandwidth-limiter"
	TypeTrafficLimiter     = "traffic-limiter"
	TypeRateLimiter        = "rate-limiter"
	TypeFairQueue          = "fair-queue"
	TypeAdminPanel         = "admin-panel"
	TypeManagerAPI         = "manager-api"
	TypeNodeManagerAPI     = "node-manager-api"
	TypeDERP               = "derp"
	TypeManager            = "manager"
	TypeNode               = "node"
	TypeResolved           = "resolved"
	TypeSSMAPI             = "ssm-api"
	TypeAPI                = "api"
	TypeCCM                = "ccm"
	TypeOCM                = "ocm"
	TypeOOMKiller          = "oom-killer"
	TypeProfiler           = "profiler"
	TypeUSBIPServer        = "usbip-server"
	TypeUSBIPClient        = "usbip-client"
	TypeHysteriaRealm      = "hysteria-realm"
	TypeACME               = "acme"
	TypeCloudflareOriginCA = "cloudflare-origin-ca"
)

const (
	TypeFallback = "fallback"
	TypeSelector = "selector"
	TypeURLTest  = "urltest"
)

// lx: SPEC 019 — URLTest balancing mode (urltest "mode" option).
const (
	URLTestModeLeastTest  = "least_test"  // default — pick lowest-delay node (legacy urltest behaviour)
	URLTestModeRoundRobin = "round_robin" // rotate over a fixed-size pool of live nodes
)

// lx: SPEC 019 — balancer.sticky_hash key components.
const (
	URLTestStickyProcess  = "process"
	URLTestStickyDomain   = "domain"
	URLTestStickySourceIP = "source_ip"
	URLTestStickyDestIP   = "dest_ip"
	URLTestStickyDestPort = "dest_port"
	// URLTestStickyNone explicitly disables stickiness: sticky_hash: ["none"]. A bare [] cannot
	// be used because the config decoder (badjson.UnmarshallExcludedContext) re-marshals the
	// struct and collapses an empty array to nil, which is indistinguishable from "omitted" —
	// so an explicit sentinel is required. Omitted → default [process, domain].
	URLTestStickyNone = "none"
)

// lx: SPEC 019 — default rotation pool size when balancer.pool is unset.
const DefaultURLTestPool = 3

func ProxyDisplayName(proxyType string) string {
	switch proxyType {
	case TypeTun:
		return "TUN"
	case TypeEBPF:
		return "eBPF"
	case TypeRedirect:
		return "Redirect"
	case TypeTProxy:
		return "TProxy"
	case TypeDirect:
		return "Direct"
	case TypeBridge:
		return "Bridge"
	case TypeBlock:
		return "Block"
	case TypeDNS:
		return "DNS"
	case TypeSOCKS:
		return "SOCKS"
	case TypeHTTP:
		return "HTTP"
	case TypeMixed:
		return "Mixed"
	case TypeShadowsocks:
		return "Shadowsocks"
	case TypeSnell:
		return "Snell"
	case TypeVMess:
		return "VMess"
	case TypeTrojan:
		return "Trojan"
	case TypeTrustTunnel:
		return "TrustTunnel"
	case TypeNaive:
		return "Naive"
	case TypeWireGuard:
		return "WireGuard"
	case TypeWARP:
		return "WARP"
	case TypeMASQUE:
		return "MASQUE"
	case TypeMTProxy:
		return "MTProxy"
	case TypeParser:
		return "Parser"
	case TypeHysteria:
		return "Hysteria"
	case TypeTor:
		return "Tor"
	case TypeSSH:
		return "SSH"
	case TypeShadowTLS:
		return "ShadowTLS"
	case TypeShadowsocksR:
		return "ShadowsocksR"
	case TypeVLESS:
		return "VLESS"
	case TypeTUIC:
		return "TUIC"
	case TypeHysteria2:
		return "Hysteria2"
	case TypeMASQUE:
		return "MASQUE"
	case TypeAnyTLS:
		return "AnyTLS"
	case TypeSudoku:
		return "Sudoku"
	case TypeCall:
		return "Call"
	case TypeFallback:
		return "Fallback"
	case TypeOpenConnect:
		return "OpenConnect"
	case TypeOpenVPNClient:
		return "OpenVPN Client"
	case TypeOpenVPNServer:
		return "OpenVPN Server"
	case TypeTailscale:
		return "Tailscale"
	case TypeCloudflared:
		return "Cloudflared"
	case TypeSelector:
		return "Selector"
	case TypeURLTest:
		return "URLTest"
	case TypeConnectionLimiter:
		return "Connection Limiter"
	case TypeBandwidthLimiter:
		return "Bandwidth Limiter"
	case TypeTrafficLimiter:
		return "Traffic Limiter"
	case TypeRateLimiter:
		return "Rate Limiter"
	case TypeFairQueue:
		return "Fair Queue"
	case TypeVPNClient:
		return "VPN Client"
	case TypeVPNServer:
		return "VPN Server"
	case TypeProfiler:
		return "Profiler"
	default:
		return "Unknown"
	}
}
