from pathlib import Path
import re, subprocess
root = Path.cwd()
assert subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip() == 'fd34958b2daf9b6a8898414a2ed29ccc6ffd58fe'
changed = set()
def edit(fn, old, new, n=1):
 p=root/fn; s=p.read_text(); assert s.count(old)==n, (fn,s.count(old),old[:70]); p.write_text(s.replace(old,new)); changed.add(fn)
# Lint the actual idle-suspend implementation rather than deleting its state.
edit('.golangci.yml','    - with_wireguard\n','    - with_wireguard\n    - with_lx_idle_suspend\n')
edit('common/trafficcontrol/group_network_test.go','type splitTraceGroup struct{ legacyTraceGroup }','type splitTraceGroup struct{}')
edit('protocol/tailscale/endpoint.go','\tmagicHostsUnrouted atomic.Bool // lx\n','')
edit('protocol/masque/outbound.go','func (o *Outbound) connectH3(ctx context.Context) (io.Closer, masque.IpConn, error) {\n\treturn o.connectH3WithBudget(ctx, 0)\n}\n\n','')
# Return errors last at every caller of the internal helper.
edit('dns/transport/group/group.go','response, err, rtt := t.timedExchange','response, rtt, err := t.timedExchange',2)
edit('dns/transport/group/fan.go','response, err, rtt := t.timedExchange','response, rtt, err := t.timedExchange')
edit('dns/transport/group/group.go','(*mDNS.Msg, error, time.Duration) {','(*mDNS.Msg, time.Duration, error) {')
edit('dns/transport/group/group.go','return response, err, time.Since(started)','return response, time.Since(started), err')
# Avoid allocating a boxed slice on every pooled write; retain the wire format.
edit('protocol/vless/encryption/common.go','return make([]byte, 5+8192+16)','return new([5 + 8192 + 16]byte)')
edit('protocol/vless/encryption/common.go','outBytes := OutBytesPool.Get().([]byte)\n\tdefer OutBytesPool.Put(outBytes)','buffer := OutBytesPool.Get().(*[5 + 8192 + 16]byte)\n\tdefer OutBytesPool.Put(buffer)\n\toutBytes := buffer[:]')
# The signal asserts acquisition; keep it inside the critical section.
edit('transport/wireguard/client_bind_dial_timeout_lx_test.go','bind.connAccess.Lock()\n\t\tbind.connAccess.Unlock()\n\t\tclose(reacquired)','bind.connAccess.Lock()\n\t\tclose(reacquired)\n\t\tbind.connAccess.Unlock()')
for fn in ['transport/v2rayxhttp/dial_deadlock_test.go','transport/v2rayxhttp/raise_failure_test.go','transport/v2rayxhttp/short_exchange_test.go']:
 edit(fn,'\t"golang.org/x/net/http2/h2c"\n','')
 edit(fn,'&http.Server{Handler: h2c.NewHandler(handler, &http2.Server{})}','newH2CTestServer(handler)')
fn='transport/v2rayxhttp/h2c_server_test.go'
(root/fn).write_text('''package v2rayxhttp

import "net/http"

// newH2CTestServer serves the same prior-knowledge HTTP/2 used by the test
// clients. Native Protocols replace the deprecated Upgrade-based h2c wrapper.
func newH2CTestServer(handler http.Handler) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{Handler: handler, Protocols: protocols}
}
'''); changed.add(fn)
edit('adapter/endpoint/manager_lifecycle_lx_test.go','for i := 0; i < 8; i++ {','for range 8 {')
edit('common/sniff/lx_sniffers_test.go','for i := 0; i < len(b); i++ {','for i := range b {')
edit('common/sniff/lx_sniffers_test.go','for j := 0; j < 2; j++ {','for j := range 2 {')
edit('common/sniff/sip_lx.go','n := len(b)\n\t\tif n > len(ms) {\n\t\t\tn = len(ms)\n\t\t}','n := min(len(b), len(ms))')
edit('experimental/clashapi/tailscale.go','if index := strings.Index(message, "https://login.tailscale.com/a/"); index >= 0 {\n\t\treturn message[:index] + "[private Tailscale login URL]"','if prefix, _, found := strings.Cut(message, "https://login.tailscale.com/a/"); found {\n\t\treturn prefix + "[private Tailscale login URL]"')
edit('protocol/chain/clone.go','period := c.idleTimeout / 4\n\tif period < minEvictTick {\n\t\tperiod = minEvictTick\n\t}','period := max(c.idleTimeout/4, minEvictTick)')
edit('protocol/group/urltest.go','size := g.balancer.poolSize\n\tif size > len(g.outbounds) {\n\t\tsize = len(g.outbounds)\n\t}\n\treturn size','return min(g.balancer.poolSize, len(g.outbounds))')
for fn in ['protocol/group/urltest.go','protocol/group/urltest_balance_lx.go']:
 edit(fn,'slotCount := size\n\tif len(current) > slotCount {\n\t\tslotCount = len(current)\n\t}','slotCount := max(size, len(current))')
edit('protocol/group/urltest.go','end := start + size\n\t\t\tif end > len(candidates) {\n\t\t\t\tend = len(candidates)\n\t\t\t}','end := min(start+size, len(candidates))')
edit('transport/wireguard/endpoint.go','for _, line := range strings.Split(ipc, "\n") {','for line := range strings.SplitSeq(ipc, "\n") {',0) if False else None
edit('transport/wireguard/endpoint.go','for _, line := range strings.Split(ipc, "\\n") {','for line := range strings.SplitSeq(ipc, "\\n") {')
edit('protocol/chain/chain_test.go','for _, it := range list {\n\t\tif it == item {\n\t\t\treturn true\n\t\t}\n\t}\n\treturn false','return slices.Contains(list, item)')
edit('protocol/group/urltest_penalty_lx.go','for _, n := range detour.Network() {\n\t\tif n == network {\n\t\t\treturn true\n\t\t}\n\t}\n\treturn false','return slices.Contains(detour.Network(), network)')
edit('transport/v2rayxhttp/meta.go','for _, a := range allowed {\n\t\tif value == a {\n\t\t\treturn nil\n\t\t}\n\t}','if slices.Contains(allowed, value) {\n\t\treturn nil\n\t}')
for fn in ['protocol/chain/chain_test.go','protocol/group/urltest_penalty_lx.go','transport/v2rayxhttp/meta.go']:
 if '"slices"' not in (root/fn).read_text(): edit(fn,'import (\n','import (\n\t"slices"\n')
for fn,var in [('common/process/socket_diag_pool_linux_test.go','workers'),('dns/transport/group/fan_test.go','wg'),('experimental/libbox/command_types_race_test.go','wg')]:
 p=root/fn; s=p.read_text(); pattern=r'(?m)^(\t*)'+var+r'.Add\(1\)\n\1go func\(\) {\n\1\tdefer '+var+r'.Done\(\)\n(.*?)^\1}\(\)'
 s,n=re.subn(pattern,lambda m:m[1]+var+'.Go(func() {\n'+m[2]+m[1]+'})',s,flags=re.S)
 assert n==(2 if fn.startswith('experimental') else 1),(fn,n)
 p.write_text(s); changed.add(fn)
for fn in sorted(changed|{'common/process/searcher_linux.go','protocol/vless/encryption/client.go'}):
 if not fn.endswith('.go'): continue
 p=root/fn; s=p.read_text(); m=re.search(r'(?ms)^import \(\n(.*?)^\)',s)
 if m:
  groups=[[],[],[]]
  for line in m[1].splitlines():
   if not line.strip(): continue
   q=re.search(r'"([^"]+)"',line); assert q,(fn,line)
   imp=q[1]; category=1 if imp.startswith('github.com/sagernet/') else 2 if '.' in imp.split('/')[0] else 0
   groups[category].append(line)
  content='\n\n'.join('\n'.join(sorted(g,key=lambda l:re.search(r'"([^"]+)"',l)[1])) for g in groups if g)
  new=s[:m.start(1)]+content+'\n'+s[m.end(1):]
  if new!=s: p.write_text(new); changed.add(fn)
changed.add('common/process/searcher_linux.go')
subprocess.run(['gofmt','-w',*[str(root/f) for f in sorted(changed) if f.endswith('.go')]],check=True)
subprocess.run(['git','diff','--check'],check=True)
print('Reviewed CI repairs applied to',len(changed),'files.')
