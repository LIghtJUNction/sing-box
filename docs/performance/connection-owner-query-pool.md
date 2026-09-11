# Connection-owner query contention (MagicNet)

## Change and limits

The Linux/Android searcher previously serialized exact queries on one netlink
socket per address-family/protocol pair. Four independent, lazily opened lanes
per pair allow unrelated queries to progress while another lane waits. Each lane
retains its request/reply lock, reconnect handling and pooled receive buffer.
There are at most 16 persistent exact-query descriptors per searcher (previously
4); temporary dump sockets are separate and unchanged. This is not a total FD or
memory cap, and round-robin scheduling does not eliminate all head-of-line waits.

Read/write helpers retry an EINTR operation on the same descriptor up to three
times. They do not resend a request when its reply read was interrupted. EAGAIN,
permission errors and successful/partial results are not retried. A signal storm
still fails after four interrupted attempts. The existing 100 ms socket timeout
is per syscall, not an exchange deadline; EINTR retries can extend total wait.
See the Go 1.14 runtime release notes for asynchronous-preemption/EINTR behavior.

No changes to DNS, MTU, packet routing, process-path matching, proxy nodes, TUN or
eBPF selection. This does not address concurrent /proc scans or payload copying.

## Controlled local benchmark

Go 1.23.2, linux/amd64, AMD EPYC 9V74 host, five repetitions, 250 ms per sample.
`SetParallelism(8)` gives 8 workers at cpu=1 and 32 at cpu=4. Real Unix datagram
socketpairs carry controlled netlink-format replies, not live kernel lookups.
Both single-lane and four-lane cases include the EINTR fix, isolating contention.
The optional 100 us server sleep is synthetic; actual sleep includes scheduling
and timer granularity. Numbers are amortized wall time per operation, not request
latency percentiles, Android measurements, or internet throughput.

| CPUs / workers | Requested reply delay | Single lane ns/op | Four lanes ns/op |
| --- | --- | ---: | ---: |
| 1 / 8 | none | 18,198 | 18,586 |
| 4 / 32 | none | 24,556 | 12,979 |
| 1 / 8 | 100 us | 1,183,843 | 339,337 |
| 4 / 32 | 100 us | 1,141,112 | 285,823 |

Values are five-run medians. The single-CPU, no-delay case regressed slightly;
this is not a universal speedup. All cases used two allocations per operation,
including the fixture's reply allocation. Pool retention and fixture setup/GC
make B/op noisy, particularly in short, delayed runs. Failed exploratory runs
exposed EINTR handling problems and were excluded, then the complete final matrix
was rerun successfully without disabling asynchronous preemption.

## Validation

A standalone harness compiled the exact production netlink file plus new pool
and I/O helpers/tests. The original blob was verified as
`b2e163043fb956e775e4b624a376aaa104d9d28b`. Compatibility shims supply only exception
formatting, network-name constants, and ErrNotFound. The existing buffer-reply
fixture constructor is reused. This does NOT compile the complete searcher or
full module dependencies, and does not run the existing live-netlink tests.

The new tests passed 20 repetitions, and 10 repetitions under the race detector:
32-worker reply isolation for all four family/protocol pairs, blocked-lane
progress, counter wraparound, lazy initialization, repeated close, datagram I/O,
and bounded EINTR retry-policy cases. Source-harness go vet and gofmt passed.

Full-checkout commands (require dependencies and appropriate host permissions):

```sh
go test -race ./common/process
go test ./common/process -run '^$' -bench '^BenchmarkSocketDiagPool$' \
  -benchmem -benchtime=250ms -count=5 -cpu=1,4
```

Before release, verify full CI and Android TCP/UDP owner attribution, then measure
connection-start latency, throughput, CPU, RSS and power with 1/8/32/64 concurrent
connections on the same device, node and network. No device-level gain is claimed.

MagicNet refreshes its sing-box submodule from `testing` during builds. An
unmerged PR pin alone is therefore not proof that a build contains this change;
merge the kernel PR only after review/CI, then verify the build revision manifest.
No merge, release or running-device configuration change is part of this patch.
