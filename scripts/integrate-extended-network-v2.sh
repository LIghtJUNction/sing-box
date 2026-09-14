#!/usr/bin/env bash
set -euo pipefail

before="$(git rev-parse HEAD)"
if ! git remote get-url extended >/dev/null 2>&1; then
  git remote add extended https://github.com/shtorm-7/sing-box-extended.git
fi
git fetch --no-tags extended extended

set +e
git merge --squash -Xours extended/extended
rc=$?
set -e
if [ "$rc" -ne 0 ]; then
  mapfile -t conflicts < <(git diff --name-only --diff-filter=U)
  for f in "${conflicts[@]}"; do
    if git cat-file -e ":2:$f" 2>/dev/null; then git checkout --ours -- "$f"; else git checkout --theirs -- "$f"; fi
    git add -- "$f"
  done
fi

# Keep this fork's project/release/control plane and already validated lx families.
git restore --source="$before" --staged --worktree -- \
  .github .gitmodules Makefile README.md release service box.go \
  include/registry.go include/quic.go constant/proxy.go \
  common/interrupt dns/transport/group dns/transport_adapter.go \
  protocol/masque transport/masque option/masque.go \
  protocol/wireguard transport/wireguard option/wireguard.go \
  protocol/vless option/vless.go \
  protocol/chain option/chain_lx.go include/lx_chain.go include/lx_chain_stub.go \
  transport/v2ray transport/v2rayxhttp option/v2ray_transport.go option/v2ray_xhttp.go option/v2ray_xhttp_xmux_range.go \
  protocol/group/urltest.go protocol/group/urltest_balance_lx.go protocol/group/urltest_penalty_lx.go protocol/group/reachability_lx.go || true
[ ! -f README.ru.md ] || git restore --source="$before" --staged --worktree -- README.ru.md || true
# Provider/manager facilities duplicate MagicNet's control plane and are deliberately excluded.
git restore --source="$before" --staged --worktree -- adapter/provider provider 2>/dev/null || true
# Keep existing documentation; integration is about runtime code, not importing another fork's manuals/assets.
git restore --source="$before" --staged --worktree -- docs 2>/dev/null || true
git rm -rf --cached examples 2>/dev/null || true
rm -rf examples

# Add extended dataplane type constants without replacing current eBPF/lx constants.
python3 - <<'PY'
from pathlib import Path
p=Path('constant/proxy.go'); s=p.read_text()
marker='\tTypeTrojan             = "trojan"\n'
extra='\tTypeTrustTunnel        = "trusttunnel"\n\tTypeMTProxy            = "mtproxy"\n\tTypeParser             = "parser"\n'
if 'TypeTrustTunnel' not in s: s=s.replace(marker, marker+extra, 1)
marker='\tTypeShadowTLS          = "shadowtls"\n'
extra='\tTypeMieru              = "mieru"\n\tTypeSudoku             = "sudoku"\n\tTypeCall               = "call"\n'
if 'TypeMieru' not in s: s=s.replace(marker, marker+extra, 1)
marker='\tTypeOpenVPNServer      = "openvpn-server"\n'
extra='\tTypeBond               = "bond"\n\tTypeFailover           = "failover"\n\tTypeVPNServer          = "vpn-server"\n\tTypeVPNClient          = "vpn-client"\n'
if 'TypeBond' not in s: s=s.replace(marker, marker+extra, 1)
marker='\tTypeCloudflared        = "cloudflared"\n'
extra='\tTypeConnectionLimiter  = "connection-limiter"\n\tTypeBandwidthLimiter   = "bandwidth-limiter"\n\tTypeTrafficLimiter     = "traffic-limiter"\n\tTypeRateLimiter        = "rate-limiter"\n\tTypeFairQueue          = "fair-queue"\n'
if 'TypeConnectionLimiter' not in s: s=s.replace(marker, marker+extra, 1)
marker='\tTypeURLTest  = "urltest"\n'
if 'TypeFallback' not in s: s=s.replace(marker, '\tTypeFallback = "fallback"\n'+marker, 1)
p.write_text(s)
PY

# Union the extended dataplane registrations into the current registry.
python3 - <<'PY'
from pathlib import Path
p=Path('include/registry.go'); s=p.read_text()
def add_import(after, text, sentinel):
    global s
    if sentinel not in s:
        if after not in s: raise SystemExit('missing import marker '+after)
        s=s.replace(after, after+text, 1)
def add_call(after, text, sentinel):
    global s
    if sentinel not in s:
        if after not in s: raise SystemExit('missing call marker '+after)
        s=s.replace(after, after+text, 1)
add_import('\t"github.com/sagernet/sing-box/dns/transport/fakeip"\n','\t"github.com/sagernet/sing-box/dns/transport/fallback"\n','dns/transport/fallback')
add_import('\t"github.com/sagernet/sing-box/protocol/block"\n','\t"github.com/sagernet/sing-box/protocol/bond"\n','protocol/bond')
add_import('\t"github.com/sagernet/sing-box/protocol/direct"\n','\t"github.com/sagernet/sing-box/protocol/failover"\n','protocol/failover')
add_import('\t"github.com/sagernet/sing-box/protocol/http"\n','\t"github.com/sagernet/sing-box/protocol/limiter/bandwidth"\n\t"github.com/sagernet/sing-box/protocol/limiter/connection"\n\t"github.com/sagernet/sing-box/protocol/limiter/rate"\n\t"github.com/sagernet/sing-box/protocol/limiter/traffic"\n\t"github.com/sagernet/sing-box/protocol/mieru"\n','protocol/mieru')
add_import('\t"github.com/sagernet/sing-box/protocol/naive"\n','\t"github.com/sagernet/sing-box/protocol/parser"\n','protocol/parser')
add_import('\t"github.com/sagernet/sing-box/protocol/vmess"\n','\t"github.com/sagernet/sing-box/protocol/vpn"\n','protocol/vpn')
add_call('\tanytls.RegisterInbound(registry)\n','\tmieru.RegisterInbound(registry)\n\tbond.RegisterInbound(registry)\n\tfailover.RegisterInbound(registry)\n\tregisterTrustTunnelInbound(registry)\n\tregisterMTProxyInbound(registry)\n\tregisterSudokuInbound(registry)\n\tregisterCallInbound(registry)\n','mieru.RegisterInbound')
add_call('\tgroup.RegisterSelector(registry)\n','\tgroup.RegisterFallback(registry)\n','group.RegisterFallback')
add_call('\tanytls.RegisterOutbound(registry)\n','\tmieru.RegisterOutbound(registry)\n\tbond.RegisterOutbound(registry)\n\tfailover.RegisterOutbound(registry)\n\tregisterTrustTunnelOutbound(registry)\n\tbandwidth.RegisterOutbound(registry)\n\tconnection.RegisterOutbound(registry)\n\ttraffic.RegisterOutbound(registry)\n\trate.RegisterOutbound(registry)\n\tparser.RegisterOutbound(registry)\n\tregisterSudokuOutbound(registry)\n\tregisterCallOutbound(registry)\n','mieru.RegisterOutbound')
add_call('\tregisterWireGuardEndpoint(registry)\n','\tvpn.RegisterServerEndpoint(registry)\n\tvpn.RegisterClientEndpoint(registry)\n','vpn.RegisterServerEndpoint')
add_call('\ttransport.RegisterHTTPS(registry)\n','\ttransport.RegisterSDNS(registry)\n','transport.RegisterSDNS')
add_call('\tfakeip.RegisterTransport(registry)\n','\tfallback.RegisterTransport(registry)\n','fallback.RegisterTransport')
p.write_text(s)
PY

# Enable build-tagged extended protocols, but not its duplicate MASQUE or manager/admin plane.
python3 - <<'PY'
from pathlib import Path
p=Path('release/DEFAULT_BUILD_TAGS_OTHERS'); tags=[x for x in p.read_text().strip().split(',') if x]
for t in ('with_mtproxy','with_trusttunnel','with_call','with_sudoku'):
    if t not in tags: tags.append(t)
for t in ('with_manager','with_admin_panel','with_masque'):
    tags=[x for x in tags if x != t]
p.write_text(','.join(tags)+'\n')
PY

git add -A
