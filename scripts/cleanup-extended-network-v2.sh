#!/usr/bin/env bash
set -euo pipefail

git fetch --no-tags origin testing
base=origin/testing

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

python3 - <<'PY'
from pathlib import Path
import re
p = Path('parser/link/vless.go')
s = p.read_text()

xmux_fields = {
    'CMaxReuseTimes', 'MaxConcurrency', 'MaxConnections',
    'HMaxRequestTimes', 'HMaxReusableSecs',
}
for field in xmux_fields:
    pattern = rf'if r, err := common\.ParseXHTTPRange\(val\); err == nil \{{\s*Transport\.XHTTPOptions\.Xmux\.{field} = r\s*\}}'
    s, n = re.subn(pattern, f'Transport.XHTTPOptions.Xmux.{field} = option.XmuxRange(val)', s, count=1)
    if n != 1:
        raise SystemExit(f'missing XHTTP parser compatibility block: {field}')

for field in ('XPaddingBytes', 'ScMaxEachPostBytes', 'ScMinPostsIntervalMs', 'ScStreamUpServerSecs'):
    pattern = rf'if r, err := common\.ParseXHTTPRange\(val\); err == nil \{{\s*Transport\.XHTTPOptions\.{field} = &?r\s*\}}'
    s, n = re.subn(pattern, f'Transport.XHTTPOptions.{field} = val', s, count=1)
    if n != 1:
        raise SystemExit(f'missing XHTTP parser compatibility block: {field}')

if 'common.ParseXHTTPRange' in s:
    raise SystemExit('unconverted XHTTP range parser remains')
p.write_text(s)
PY

gofmt -w parser/link/vless.go common/utils.go

git add -A
