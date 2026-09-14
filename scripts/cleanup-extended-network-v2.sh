#!/usr/bin/env bash
set -euo pipefail

git fetch --no-tags origin testing
base=origin/testing

# Restore shared core/control-plane files that the extended fork changed for its
# own manager/provider/server product. MagicNet keeps the current testing/lx
# implementations for these surfaces.
git restore --source="$base" --staged --worktree -- \
  .fpm_openwrt Dockerfile \
  adapter/endpoint/manager.go adapter/experimental.go adapter/inbound.go adapter/inbound/registry.go \
  adapter/outbound.go adapter/outbound/manager.go adapter/outbound/registry.go adapter/platform.go \
  clients/android clients/apple \
  cmd/internal/update_desktop_version/main.go cmd/sing-box/cmd_run.go \
  common/dialer/default.go common/dialer/detour.go common/mux/router.go common/tls/std_server.go \
  daemon experimental log route test \
  dns/client.go dns/transport/dhcp/dhcp.go dns/transport_manager.go \
  include/wireguard.go \
  option/experimental.go option/options.go option/rule_action.go \
  protocol/anytls/inbound.go protocol/group/selector.go protocol/http/inbound.go \
  protocol/hysteria/inbound.go protocol/hysteria2/inbound.go protocol/mixed/inbound.go \
  protocol/naive/inbound.go protocol/naive/outbound.go protocol/socks/inbound.go \
  protocol/tailscale/dns_transport.go protocol/tailscale/endpoint.go \
  protocol/trojan/inbound.go protocol/trojan/outbound.go protocol/tuic/inbound.go \
  protocol/tun/inbound.go protocol/vmess/inbound.go protocol/vmess/outbound.go

# Drop files/directories that exist only for the other fork's release,
# manager/provider/database stack or duplicate WARP/MASQUE implementations.
rm -rf \
  .goreleaser.yaml DONATE.md codeberg-release.sh \
  adapter/provider.go cmd/internal/admin_panel_pack \
  common/cloudflare common/migrate common/sql common/tls/masque_client.go \
  constant/manager_api.go constant/node_manager_api.go constant/provider.go \
  include/masque.go include/masque_stub.go \
  option/admin_panel.go option/cloudflare.go option/manager.go option/manager_api.go \
  option/node.go option/node_manager_api.go option/profiler.go option/provider.go \
  protocol/warp

git add -A

# Keep the link parser, but translate extended's old range objects to the
# canonical lx XHTTP schema already merged into testing.
python3 - <<'PY'
from pathlib import Path
p = Path('parser/link/vless.go')
s = p.read_text()
repls = {
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\t\tTransport.XHTTPOptions.Xmux.CMaxReuseTimes = r\n\t\t\t\t\t}''': '''Transport.XHTTPOptions.Xmux.CMaxReuseTimes = option.XmuxRange(val)''',
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\t\tTransport.XHTTPOptions.Xmux.MaxConcurrency = r\n\t\t\t\t\t}''': '''Transport.XHTTPOptions.Xmux.MaxConcurrency = option.XmuxRange(val)''',
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\t\tTransport.XHTTPOptions.Xmux.MaxConnections = r\n\t\t\t\t\t}''': '''Transport.XHTTPOptions.Xmux.MaxConnections = option.XmuxRange(val)''',
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\t\tTransport.XHTTPOptions.Xmux.HMaxRequestTimes = r\n\t\t\t\t\t}''': '''Transport.XHTTPOptions.Xmux.HMaxRequestTimes = option.XmuxRange(val)''',
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\t\tTransport.XHTTPOptions.Xmux.HMaxReusableSecs = r\n\t\t\t\t\t}''': '''Transport.XHTTPOptions.Xmux.HMaxReusableSecs = option.XmuxRange(val)''',
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\tTransport.XHTTPOptions.XPaddingBytes = r\n\t\t\t\t}''': '''Transport.XHTTPOptions.XPaddingBytes = val''',
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\tTransport.XHTTPOptions.ScMaxEachPostBytes = &r\n\t\t\t\t}''': '''Transport.XHTTPOptions.ScMaxEachPostBytes = val''',
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\tTransport.XHTTPOptions.ScMinPostsIntervalMs = &r\n\t\t\t\t}''': '''Transport.XHTTPOptions.ScMinPostsIntervalMs = val''',
'''if r, err := common.ParseXHTTPRange(val); err == nil {\n\t\t\t\t\tTransport.XHTTPOptions.ScStreamUpServerSecs = &r\n\t\t\t\t}''': '''Transport.XHTTPOptions.ScStreamUpServerSecs = val''',
}
for old, new in repls.items():
    if old not in s:
        raise SystemExit('missing XHTTP parser compatibility block')
    s = s.replace(old, new, 1)
p.write_text(s)
PY

gofmt -w parser/link/vless.go common/utils.go

git add -A
