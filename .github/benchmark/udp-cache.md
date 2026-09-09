# UDP client cache sharding

The shared-network UDP cache previously selected one of 16 shards using only
the source port. Clients on different addresses using the same port therefore
contended on the same shard lock.

Non-loopback clients now use a process-seeded hash of the complete endpoint.
Loopback clients retain the inexpensive port-based selection used for local
redirected sockets. Binding ownership, redirect reference counting and session
generation checks are unchanged.

## Local measurement

Measured on Linux amd64, Intel Core i9-12900HX, Go 1.27.1, with
`GOMAXPROCS=4`. Each result is the median of five runs. The baseline already
included the upstream synchronization to `abc21e4b`; only shard selection
changed between the baseline and final measurement.

| Cache access | Before (ns/op) | After (ns/op) | Reduction |
| --- | ---: | ---: | ---: |
| One loopback client | 327.6 | 327.4 | Essentially unchanged |
| Same-port IPv4 clients, parallel | 282.4 | 169.5 | 40.0% |
| Same-port IPv6 clients, parallel | 264.4 | 172.7 | 34.7% |

All cases reported zero bytes and zero allocations per operation. The parallel
benchmark cycles through 256 addresses sharing source port 3478. These are
cache microbenchmarks, not measurements of end-to-end network or voice latency.

Run the isolated cache benchmarks from the repository root:

```sh
GOMAXPROCS=4 go test -tags with_ebpf -run '^$' \
  -bench 'BenchmarkUDPClientTable(CacheHit$|SamePortParallel)' \
  -benchmem -count=5 \
  protocol/ebpf/udp_state.go protocol/ebpf/udp_state_test.go \
  protocol/ebpf/udp_shard_test.go
```

`TestUDPClientTableSamePortIsolation` exercises concurrent creation, lookup and
deletion for distinct IPv4 and IPv6 clients sharing one port. Run it together
with the existing UDP generation, reconnect and reference-lifetime tests under
the race detector:

```sh
go test -race -count=1 -tags with_ebpf ./protocol/ebpf
```
