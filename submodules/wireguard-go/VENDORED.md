# WireGuard / AmneziaWG source provenance

This directory is a tracked copy of the former submodule, without Git
metadata, from:

- Repository: <https://github.com/Leadaxe/wireguard-go-awg2-lx>
- Former branch: `lx-awg2-v005`
- Exact base commit: `6383749977b5a9741519540f9d40118bb73045b8`
- Module path: `github.com/sagernet/wireguard-go`
- License: MIT; the original `LICENSE` and source copyright notices are retained.

All files from the pinned commit are retained. The AmneziaWG implementation,
including its header protection, packet padding, ranged timers, endpoint
resolver and bind recovery behavior, remains at that commit.

## Local compatibility patch

`device/device.go` and `device/peer.go` backport the optional pre-shared key
support for lazily created peers from SagerNet's exact commit
[`ca3bc60c4ce798c157689a9d653041470e627795`](https://github.com/SagerNet/wireguard-go/commit/ca3bc60c4ce798c157689a9d653041470e627795):

- `NewPeerConfig.PresharedKey` carries the initial optional key.
- `LookupPeer` installs it before starting the new peer.
- `Peer.SetPresharedKey` updates the actual Noise handshake key under its mutex;
  a zero key preserves ordinary WireGuard operation without the optional layer.

The updated Tailscale dependency uses this API. Merely adding the field would
compile while silently omitting the key from authentication. The backport
keeps the existing AWG implementation and the local module replacement usable
without modifying or publishing a separate third-party repository.

`device/peer_lookup_psk_test.go` verifies real Noise authentication and derived
transport keys for matching keys and no key, rejection of mismatched or missing
keys, and clearing the optional key.
