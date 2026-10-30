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

## Initial tracked synchronization

The upstream base of the original fork is
`c6c8a831ef7091d564dad5452703c8e82b3c800e`. The full source delta from that base
through SagerNet's then-required
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

## Current upstream synchronization

The tracked snapshot now carries the complete source-tree delta from
`ca3bc60c4ce798c157689a9d653041470e627795` through
[`fdf0ad04658666a57f15da1b0eea30c08b0a94db`](https://github.com/SagerNet/wireguard-go/commit/fdf0ad04658666a57f15da1b0eea30c08b0a94db),
as required by the parent core. Upstream rewrote the PSK and blocking-injection
commits; their replacements are patch-equivalent to the already integrated
versions. Comparing the fixed source trees identifies three additional fixes:

| Upstream commit | Carried behavior |
| --- | --- |
| `34ff6c5e8c949dd97978932741760290d4eb767f` | Restore the full IPv6 destination when reusing a pooled address shortened by an earlier IPv4 send. |
| `3a027ec9911537e4fbe0f2346b4dc6eda00658b3` | Include system sleep in key and handshake ages; retain the fork's AWG-configured expiry and rekey intervals. |
| `e4d85ef2ed387cc07233b5324d4a40f3df94566e` | Wait for resume outside timer locks; reject stale callbacks after cancellation or deadline replacement. |

The entire delta spans `conn/bind_std.go`, `device/device.go`,
`device/receive.go`, `device/send.go` and `device/timers.go`. The merge keeps
AWG header protection, padding, reserved-byte guards, recovery behavior and
blocking packet injection intact. Sleep-aware age calculations use the same
AWG timeout accessors as before; they do not substitute upstream constants
for configured intervals. Buffer sizes and allocation pools are unchanged.

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
- `conn/address_reuse_lx_test.go` sends a real IPv6 loopback datagram using
  a pooled address with stale IPv6 tail bytes and an IPv4-length slice.
- `device/timers_pause_lx_test.go` verifies that cancellation completes during
  pause and that an old callback cannot fire before a replacement deadline.
  Both timer cases and the IPv6 case reproduce failures on the previous snapshot.
