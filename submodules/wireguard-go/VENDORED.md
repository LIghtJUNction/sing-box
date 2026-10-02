# WireGuard / AmneziaWG source provenance

This directory is a tracked copy of the former submodule, without Git
metadata, from:

- Repository: <https://github.com/Leadaxe/wireguard-go-awg2-lx>
- Former branch: `lx-awg2-v005`
- Exact base commit: `6383749977b5a9741519540f9d40118bb73045b8`
- Module path: `github.com/sagernet/wireguard-go`
- License: MIT; the original `LICENSE` and source copyright notices are retained.

All files from the pinned commit are retained. Its AmneziaWG header protection,
packet padding, ranged timers, receive classification and bind recovery patches
are preserved while carrying the complete required upstream line below.

## Complete upstream synchronization

The upstream base of the original fork is
`c6c8a831ef7091d564dad5452703c8e82b3c800e`. The full source delta from that base
through SagerNet's required
[`ca3bc60c4ce798c157689a9d653041470e627795`](https://github.com/SagerNet/wireguard-go/commit/ca3bc60c4ce798c157689a9d653041470e627795)
was applied by three-way merge, retaining the AWG and recovery differences.
This covers all six intervening upstream commits, rather than only new API
fields that would make the parent module compile:

| Upstream commit | Carried behavior |
| --- | --- |
| `8bd032a91a3076bb09cb6103861343d973c5e289` | I/O activity callbacks; already equivalent in fork `d842fd55e2e9fe10822e5dbee5746e500824fe8b`. |
| `abd9348cd717a65b3cacbe136db71bd0aa62fdec` | Darwin connected-socket bind recovery; already equivalent in fork `6383749977b5a9741519540f9d40118bb73045b8`, with retained AWG reserved-byte guards. |
| `7aa7121e681cc35748e7f0001c33dc1e2a8d3312` | Device-level endpoint resolver, registered before `IpcSet` can initiate the first handshake. |
| `6731c7387c811275155632546b518c261a54f274` | Darwin zero-length receive reports `ErrRebindRequired` wrapping `io.EOF`. |
| `b3540b366f7e59c55c765ef4b8347ab17544f09c` | Optional PSK is installed in the actual handshake before starting a lazily created peer. |
| `ca3bc60c4ce798c157689a9d653041470e627795` | Blocking per-peer `WritePackets`, separate staged/outbound counters, pipeline drain wakeups and Stop cancellation. |

The complete upstream delta touches these ten paths; every path is accounted
for in the resulting snapshot:

| Path | Merge result |
| --- | --- |
| `conn/bind_std.go` | Already contains I/O callbacks and bind recovery; retained AWG reserved-byte guards. |
| `conn/conn.go` | Already contains `ErrRebindRequired`. |
| `conn/msgx_darwin.go` | Existing recovery plus complete zero-receive guard; retained AWG reserved-byte guards. |
| `conn/msgx_default.go` | Already contains the updated non-Darwin stub contract. |
| `device/device.go` | Existing bind recovery plus device resolver registration and lazy-peer PSK installation. |
| `device/endpoint_resolver_test.go` | Complete upstream resolver tests, including first-handshake lookup during `IpcSet`. |
| `device/peer.go` | Device resolver lookup, handshake PSK setter, complete queue fields and Stop wakeup; AWG UDP-window helpers retained. |
| `device/pools.go` | Upstream batch-capacity adjustment and allocator contract. |
| `device/receive.go` | Already contains receive-triggered bind recovery. |
| `device/send.go` | Complete upstream injection/backpressure lifecycle; shared input allocation retains AWG `s4`, content/trailer tailroom and the fork's zero-byte encapsulation headroom. |

The local replacement is tracked in the parent repository so this synchronized
snapshot is reproducible without modifying or publishing a third-party fork.
The parent transport uses the native blocking peer API; it does not substitute
a separate nonblocking queue for the new upstream behavior.

## Regression evidence

- `device/peer_lookup_psk_test.go` exercises real Noise authentication and both
  derived transport directions, accepting matching/no PSK and rejecting wrong
  or missing PSK.
- `device/write_packets_test.go` gates a real sender after the handshake, fills
  the actual transmission pipeline, and verifies writer blocking, delivery
  after drain, and cancellation on `Stop` before the sender is released.
- Existing AWG tests exercise TUN reads, `InputPackets`, native `WritePackets`,
  header protection, transport padding and the other retained send paths.
- Darwin connection code is cross-compiled; no Darwin runtime result is claimed.
