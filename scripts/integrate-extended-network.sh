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
    if git cat-file -e ":2:$f" 2>/dev/null; then
      git checkout --ours -- "$f"
    else
      git checkout --theirs -- "$f"
    fi
    git add -- "$f"
  done
fi

# Keep LIghtJUNction/lx ownership of release policy and the feature families
# already validated on the current testing API. Extended contributes unique
# network protocols, routing groups, DNS transports, limiters and transports.
git restore --source="$before" --staged --worktree -- \
  .github .gitmodules Makefile README.md release \
  common/interrupt \
  protocol/wireguard transport/wireguard option/wireguard.go \
  transport/v2rayxhttp option/v2ray_xhttp.go option/v2ray_xhttp_xmux_range.go \
  protocol/masque transport/masque option/masque.go \
  protocol/chain option/chain_lx.go include/lx_chain.go include/lx_chain_stub.go \
  dns/transport/group dns/transport_adapter.go \
  protocol/vless option/vless.go \
  include/quic.go || true
[ ! -f README.ru.md ] || git restore --source="$before" --staged --worktree -- README.ru.md || true

# Preserve the lx urltest balancing/failover implementation while allowing the
# extended fork to add its independent fallback group files.
for f in \
  protocol/group/urltest.go \
  protocol/group/urltest_balance_lx.go \
  protocol/group/urltest_penalty_lx.go \
  protocol/group/reachability_lx.go; do
  if git cat-file -e "$before:$f" 2>/dev/null; then
    git restore --source="$before" --staged --worktree -- "$f"
  fi
done

# Build a dataplane-focused registry union. We intentionally do not activate
# extended's Admin/Manager/Node service plane or provider control plane because
# MagicNet already owns configuration, subscriptions and privileged control.
python3 - <<'PY'
from pathlib import Path
p = Path('include/registry.go')
s = p.read_text()

def once(old, new):
    global s
    if new in s:
        return
    if old not in s:
        raise SystemExit(f'registry marker missing: {old!r}')
    s = s.replace(old, old + new, 1)

once('"github.com/sagernet/sing-box/dns/transport/fakeip"\n', '\t"github.com/sagernet/sing-box/dns/transport/fallback"\n')
once('"github.com/sagernet/sing-box/protocol/block"\n', '\t"github.com/sagernet/sing-box/protocol/bond"\n')
once('"github.com/sagernet/sing-box/protocol/direct"\n', '\t"github.com/sagernet/sing-box/protocol/failover"\n')
once('"github.com/sagernet/sing-box/protocol/http"\n', '\t"github.com/sagernet/sing-box/protocol/limiter/bandwidth"\n\t"github.com/sagernet/sing-box/protocol/limiter/connection"\n\t"github.com/sagernet/sing-box/protocol/limiter/rate"\n\t"github.com/sagernet/sing-box/protocol/limiter/traffic"\n\t"github.com/sagernet/sing-box/protocol/mieru"\n')
once('"github.com/sagernet/sing-box/protocol/naive"\n', '\t"github.com/sagernet/sing-box/protocol/parser"\n')
once('"github.com/sagernet/sing-box/protocol/vmess"\n', '\t"github.com/sagernet/sing-box/protocol/vpn"\n')

once('\tanytls.RegisterInbound(registry)\n', '\tmieru.RegisterInbound(registry)\n\tbond.RegisterInbound(registry)\n\tfailover.RegisterInbound(registry)\n\tregisterTrustTunnelInbound(registry)\n\tregisterMTProxyInbound(registry)\n\tregisterSudokuInbound(registry)\n\tregisterCallInbound(registry)\n')
once('\tgroup.RegisterSelector(registry)\n', '\tgroup.RegisterFallback(registry)\n')
once('\tanytls.RegisterOutbound(registry)\n', '\tmieru.RegisterOutbound(registry)\n\tbond.RegisterOutbound(registry)\n\tfailover.RegisterOutbound(registry)\n\tregisterTrustTunnelOutbound(registry)\n\tbandwidth.RegisterOutbound(registry)\n\tconnection.RegisterOutbound(registry)\n\ttraffic.RegisterOutbound(registry)\n\trate.RegisterOutbound(registry)\n\tparser.RegisterOutbound(registry)\n\tregisterSudokuOutbound(registry)\n\tregisterCallOutbound(registry)\n')
once('\tregisterWireGuardEndpoint(registry)\n', '\tvpn.RegisterServerEndpoint(registry)\n\tvpn.RegisterClientEndpoint(registry)\n')
once('\ttransport.RegisterHTTPS(registry)\n', '\ttransport.RegisterSDNS(registry)\n')
once('\tfakeip.RegisterTransport(registry)\n', '\tfallback.RegisterTransport(registry)\n')
p.write_text(s)
PY

# Activate every extended network protocol that is build-tag isolated. Keep
# service/control-plane tags disabled; lx MASQUE remains the canonical MASQUE.
python3 - <<'PY'
from pathlib import Path
p = Path('release/DEFAULT_BUILD_TAGS_OTHERS')
tags = [x.strip() for x in p.read_text().strip().split(',') if x.strip()]
for tag in ('with_mtproxy', 'with_trusttunnel', 'with_call', 'with_sudoku'):
    if tag not in tags:
        tags.append(tag)
for tag in ('with_manager', 'with_admin_panel'):
    tags = [x for x in tags if x != tag]
p.write_text(','.join(tags) + '\n')
PY

git add -A
