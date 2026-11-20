package option

import (
	"context"
	"net/netip"
	"reflect"

	"github.com/sagernet/sing-box/schema"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badjson"
	"github.com/sagernet/sing/common/json/badoption"
)

type MASQUEEndpointOptions struct {
	System       bool           `json:"system,omitempty"`
	Name         string         `json:"name,omitempty"`
	MTU          uint32         `json:"mtu,omitempty"`
	UDPMapping   UDPNATBehavior `json:"udp_mapping,omitempty"`
	UDPFiltering UDPNATBehavior `json:"udp_filtering,omitempty"`
	UDPNATMax    uint32         `json:"udp_nat_max,omitempty"`
}

type _MASQUEClientEndpointOptions struct {
	DialerOptions
	ServerOptions
	MASQUEEndpointOptions
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	OutboundTLSOptionsContainer
	Path                   string               `json:"path,omitempty"`
	Headers                badoption.HTTPHeader `json:"headers,omitempty"`
	Version                int                  `json:"version,omitempty" enum:"0,1,2,3"`
	DisableVersionFallback bool                 `json:"disable_version_fallback,omitempty"`
	AdvertiseRoutes        []netip.Prefix       `json:"advertise_routes,omitempty"`
	UDPTimeout             badoption.Duration   `json:"udp_timeout,omitempty"`
	OnDemand               bool                 `json:"on_demand,omitempty"`
	HTTP2Options           HTTP2Options         `json:"-"`
	HTTP3Options           QUICOptions          `json:"-"`
}

type MASQUEClientEndpointOptions _MASQUEClientEndpointOptions

func (o MASQUEClientEndpointOptions) ResolvedVersion() int {
	if o.Version == 0 {
		return 3
	}
	return o.Version
}

func (o MASQUEClientEndpointOptions) MarshalJSON() ([]byte, error) {
	return badjson.MarshallObjects(_MASQUEClientEndpointOptions(o), httpVersionVariant(o.ResolvedVersion(), o.HTTP2Options, o.HTTP3Options))
}

func (o *MASQUEClientEndpointOptions) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	err := json.UnmarshalContext(ctx, content, (*_MASQUEClientEndpointOptions)(o))
	if err != nil {
		return err
	}
	return unmarshalHTTPVersionOptions(ctx, content, (*_MASQUEClientEndpointOptions)(o), o.ResolvedVersion(), &o.HTTP2Options, &o.HTTP3Options)
}

func (o MASQUEClientEndpointOptions) DescribeSchema(builder schema.Builder) (*schema.Node, error) {
	node := schema.StrictObject()
	err := builder.FlattenStruct(node, reflect.TypeFor[MASQUEClientEndpointOptions]())
	if err != nil {
		return nil, err
	}
	err = builder.FlattenStruct(node, reflect.TypeFor[QUICOptions]())
	if err != nil {
		return nil, err
	}
	return node, nil
}

type _MASQUEServerEndpointOptions struct {
	ListenOptions
	MASQUEEndpointOptions
	Users   []auth.User             `json:"users,omitempty"`
	Version badoption.Listable[int] `json:"version,omitempty" enum:"1,2,3"`
	InboundTLSOptionsContainer
	Path            string                           `json:"path,omitempty"`
	Address         badoption.Listable[netip.Prefix] `json:"address"`
	AdvertiseRoutes []netip.Prefix                   `json:"advertise_routes,omitempty"`
	HTTP2Options    HTTP2Options                     `json:"-"`
	HTTP3Options    QUICOptions                      `json:"-"`
}

type MASQUEServerEndpointOptions _MASQUEServerEndpointOptions

func (o MASQUEServerEndpointOptions) Versions() []int {
	if len(o.Version) > 0 {
		return o.Version
	}
	return []int{1, 2, 3}
}

func (o MASQUEServerEndpointOptions) MarshalJSON() ([]byte, error) {
	return badjson.MarshallObjects(_MASQUEServerEndpointOptions(o), httpVersionsVariant(o.Versions(), o.HTTP2Options, o.HTTP3Options))
}

func (o *MASQUEServerEndpointOptions) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	err := json.UnmarshalContext(ctx, content, (*_MASQUEServerEndpointOptions)(o))
	if err != nil {
		return err
	}
	return unmarshalHTTPVersionsOptions(ctx, content, (*_MASQUEServerEndpointOptions)(o), o.Versions(), &o.HTTP2Options, &o.HTTP3Options)
}

func (o MASQUEServerEndpointOptions) DescribeSchema(builder schema.Builder) (*schema.Node, error) {
	node := schema.StrictObject()
	err := builder.FlattenStruct(node, reflect.TypeFor[MASQUEServerEndpointOptions]())
	if err != nil {
		return nil, err
	}
	err = builder.FlattenStruct(node, reflect.TypeFor[QUICOptions]())
	if err != nil {
		return nil, err
	}
	return node, nil
}

// MASQUEOutboundOptions configures a MASQUE (CONNECT-IP / RFC 9484) outbound,
// primarily for Cloudflare WARP. See SPEC 021.
//
// The HTTP version carrying the tunnel (h3/h2) is `vhttp`; the tcp/udp
// allow-list is `network_list`, as in every other outbound. TLS goes in the
// standard `tls` block.
//
// Legacy shapes still work until v1.14.0-lx.30 and report a deprecation:
// `network` for the HTTP version (it meant the opposite of everyone else's
// `network`), and flat `sni` / `skip_cert_verify` / `fragment*` for their `tls`
// counterparts. See SPEC 062.
type MASQUEOutboundOptions struct {
	DialerOptions
	ServerOptions
	// lx: SPEC 062 — the standard `tls: {…}` block, same container every other
	// TLS outbound uses. The flat sni/skip_cert_verify/fragment* fields below
	// are kept as aliases for it until v1.14.0-lx.30.
	OutboundTLSOptionsContainer

	// Profile selects behaviour: "cloudflare" (default) or "standard" (RFC 9484).
	Profile string `json:"profile,omitempty"`
	// VHTTP selects the HTTP version carrying CONNECT-IP: "auto" (default) —
	// try h3 and fall back to h2 when the QUIC handshake does not complete
	// (lx: SPEC 074; the endpoint may ignore QUIC from this source address,
	// which looks like a hang rather than an error) — or a fixed "h3" (QUIC) /
	// "h2". On the standard profile there is no h2 leg, so the default quietly
	// means h3 there. The tcp/udp allow-list is NetworkList, as everywhere
	// else.
	//
	// Not named `transport`: that key is an object (V2RayTransportOptions) on
	// vless/trojan/vmess, and reusing it for a string here would repeat exactly
	// the confusion this migration removes. lx: SPEC 062.
	VHTTP string `json:"vhttp,omitempty" enum:"h3,h2,auto"`

	// Deprecated: use `vhttp`. Removed in v1.14.0-lx.30.
	//
	// This field means the HTTP version (h3/h2), not the tcp/udp list — the
	// opposite of `network` in every other outbound. That inversion is why it
	// is being retired; `network` is expected to take its usual meaning later.
	Network string `json:"network,omitempty"`

	// Key material (required for the cloudflare profile). Base64-encoded DER:
	// PrivateKey via x509.ParseECPrivateKey, PublicKey via x509.ParsePKIXPublicKey.
	PrivateKey string `json:"private_key,omitempty"`
	PublicKey  string `json:"public_key,omitempty"`

	// Local tunnel addresses. At least one of IP/IPv6 is required. A bare
	// address without a mask is treated as /32 (v4) or /128 (v6).
	IP   string `json:"ip,omitempty"`
	IPv6 string `json:"ipv6,omitempty"`

	// URI is the CONNECT-IP request URI template. Defaults per profile.
	URI string `json:"uri,omitempty"`
	// MTU of the userspace stack. Defaults to 1280.
	MTU uint32 `json:"mtu,omitempty"`

	// The fields below moved into the `tls` block. Each is still honoured, and
	// using one reports a deprecation once per outbound. Removed in
	// v1.14.0-lx.30. lx: SPEC 062.
	//
	// Note on the bool ones: an unset field and an explicit `false` are
	// indistinguishable here, so only a legacy `true` can be carried over — see
	// resolveLegacyOptions.

	// Deprecated: use `tls.server_name`. Removed in v1.14.0-lx.30.
	SNI string `json:"sni,omitempty"`
	// Deprecated: use `tls.insecure`. Removed in v1.14.0-lx.30.
	SkipCertVerify bool `json:"skip_cert_verify,omitempty"`
	// Deprecated: use `tls.fragment`. Removed in v1.14.0-lx.30.
	Fragment bool `json:"fragment,omitempty"`
	// Deprecated: use `tls.fragment_fallback_delay`. Removed in v1.14.0-lx.30.
	FragmentFallbackDelay badoption.Duration `json:"fragment_fallback_delay,omitempty"`
	// Deprecated: use `tls.record_fragment`. Removed in v1.14.0-lx.30.
	RecordFragment bool `json:"record_fragment,omitempty"`

	// IdleTimeout suspends the tunnel after this long with no traffic (freeing
	// the userspace stack, pumps and QUIC keepalive); the next dial rebuilds it.
	// Off by default: absent, "0" and negative all keep the tunnel up until
	// Close. Only a positive value enables idle-suspend.
	IdleTimeout badoption.Duration `json:"idle_timeout,omitempty"`
	// KeepAlivePeriod is the QUIC (h3) keepalive interval. Empty = 30s. A
	// negative value disables keepalive.
	KeepAlivePeriod badoption.Duration `json:"keep_alive_period,omitempty"`

	// L4 protocols routed through the tunnel: tcp and/or udp. Empty = both.
	NetworkList NetworkList `json:"network_list,omitempty"`
}
