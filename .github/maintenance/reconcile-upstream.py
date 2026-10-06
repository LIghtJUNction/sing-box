"""Reproduce the reviewed merge of immutable fork/upstream commits.

Preserve Android/eBPF controls, group policy, TLS detour, mixed/gvisor
compatibility and WireGuard/AWG lifecycle. Accept upstream forward NAT.
This script performs no network operation or branch update.
"""
from pathlib import Path
import re
import subprocess

FORK = 'b7eb313f92587b92c417ef02a6c3a80b6aaf738a'
UPSTREAM = 'a4331b8d745971fdc1be33bba5513ab16105e2d3'
COUNTS = {
 'box.go':1, 'cmd/internal/build_libbox/main.go':1,
 'common/trafficcontrol/manager.go':2, 'common/trafficcontrol/tracker.go':2,
 'daemon/started_service.go':1, 'docs/installation/build-from-source.md':1,
 'docs/installation/build-from-source.zh.md':1, 'experimental/clashapi/proxies.go':1,
 'option/masque.go':1, 'protocol/group/selector.go':1, 'protocol/group/urltest.go':2,
 'protocol/http/outbound.go':1, 'protocol/masque/client.go':3,
 'protocol/masque/server.go':1, 'protocol/tailscale/endpoint.go':1,
 'protocol/tailscale/system_binding.go':4, 'protocol/wireguard/endpoint.go':1,
 'release/DEFAULT_BUILD_TAGS_OTHERS':1, 'route/route.go':5,
 'test/go.mod':5, 'test/go.sum':4, 'test/masque_test.go':3,
 'transport/device/device.go':1, 'transport/device/stack.go':3,
 'transport/http/client.go':3, 'transport/wireguard/device.go':2,
 'transport/wireguard/device_peer_router.go':2,
 'transport/wireguard/device_stack.go':7, 'transport/wireguard/device_system.go':1,
 'transport/wireguard/device_system_stack.go':5, 'transport/wireguard/endpoint.go':6,
}
# These hunks add upstream interfaces rather than replace downstream policy.
TAKE_UPSTREAM = {
 ('protocol/masque/client.go',3), ('protocol/masque/server.go',1),
 ('transport/device/device.go',1), ('transport/device/stack.go',3),
 ('transport/wireguard/device_system.go',1),
}
PATTERN = re.compile(r'^<<<<<<< HEAD\n(.*?)^=======\n(.*?)^>>>>>>> '+UPSTREAM+r'\n', re.M|re.S)

def git(*args):
 return subprocess.check_output(['git',*args], text=True).strip()

def main():
 if git('rev-parse','HEAD') != FORK or git('rev-parse','MERGE_HEAD') != UPSTREAM:
  raise SystemExit('refusing an unreviewed pair of commits')
 files = git('diff','--name-only','--diff-filter=U').splitlines()
 if set(files) != set(COUNTS):
  raise SystemExit('unexpected conflict file set')
 resolved = {}
 for fn in files:
  text = Path(fn).read_text()
  if len(list(PATTERN.finditer(text))) != COUNTS[fn]:
   raise SystemExit('unexpected conflict hunk count: '+fn)
  i = 0
  def choose(m):
   nonlocal i
   i += 1
   ours, theirs = m.groups()
   if (fn,i) in TAKE_UPSTREAM:
    return theirs
   if fn == 'transport/wireguard/device_stack.go' and i == 7:
    # Preserve active-stream accounting and the local setPeers signature.
    return 'func (w *stackDevice) UpstreamPort() any {\n\treturn w.stack\n}\n\n'+ours
   return ours
  text = PATTERN.sub(choose,text)
  if re.search(r'^(<<<<<<<|=======|>>>>>>>)',text,re.M):
   raise SystemExit('unresolved marker in '+fn)
  resolved[fn]=text
 for fn,text in resolved.items():
  Path(fn).write_text(text)
 p=Path('test/go.mod')
 old='github.com/sagernet/sing-tun v0.9.7-0.20261002083955-3f8acd9da65b'
 new='github.com/sagernet/sing-tun v0.9.7-0.20261006124248-d769a7080ca2'
 assert p.read_text().count(old)==1
 p.write_text(p.read_text().replace(old,new))
 # Preserve Go-generated ordering and the fork's newer dependency pins.
 sums=Path('test/go.sum')
 previous=git('show',FORK+':test/go.sum')+'\n'
 entries=[line for line in Path('go.sum').read_text().splitlines() if line.startswith(new)]
 assert len(entries)==2
 for entry in entries:
  suffix='/go.mod' if entry.split()[1].endswith('/go.mod') else ''
  previous,count=re.subn(r'^'+re.escape(old+suffix)+r' [^\n]+$',entry,previous,flags=re.M)
  assert count==1
 sums.write_text(previous)
 # Textual auto-merge removed an import still required by the detour policy.
 p=Path('protocol/http/outbound.go')
 text=p.read_text()
 assert 'tls.DialedThroughDetour(' in text
 if '"github.com/sagernet/sing-box/common/tls"' not in text:
  text=text.replace('"github.com/sagernet/sing-box/common/dialer"',
   '"github.com/sagernet/sing-box/common/dialer"\n\t"github.com/sagernet/sing-box/common/tls"')
  p.write_text(text)
 # MagicNet #261 still requires device A/B before removing mixed support.
 p=Path('release/DEFAULT_BUILD_TAGS')
 if 'with_gvisor' not in p.read_text().strip().split(','):
  p.write_text('with_gvisor,'+p.read_text())
 subprocess.run(['gofmt','-w',*[fn for fn in files if fn.endswith('.go')]],check=True)
 subprocess.run(['git','add','--',*files,'release/DEFAULT_BUILD_TAGS'],check=True)
 subprocess.run(['git','diff','--cached','--check'],check=True)
 assert not git('diff','--name-only','--diff-filter=U')
 print('Resolved all 31 reviewed conflict files; formatting and diff checks pass.')

if __name__=='__main__':
 main()
