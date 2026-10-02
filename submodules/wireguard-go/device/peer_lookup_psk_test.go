/* SPDX-License-Identifier: MIT */

package device

import (
	"bytes"
	"context"
	"net/netip"
	"testing"
)

// Exercise the actual Noise transcript and derived transport keys. Lazy peer
// creation must install the PSK before authenticating its first response.
func TestLazyPeerPresharedKey(t *testing.T) {
	psk := NoisePresharedKey{1, 2, 3, 4}
	otherPSK := NoisePresharedKey{5, 6, 7, 8}
	for _, test := range []struct {
		name                 string
		clientPSK, serverPSK NoisePresharedKey
		accept               bool
	}{
		{"matching", psk, psk, true},
		{"mismatching", psk, otherPSK, false},
		{"missing_server_key", psk, NoisePresharedKey{}, false},
		{"no_psk", NoisePresharedKey{}, NoisePresharedKey{}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientPrivate, err := newPrivateKey()
			if err != nil {
				t.Fatal(err)
			}
			serverPrivate, err := newPrivateKey()
			if err != nil {
				t.Fatal(err)
			}
			bindA, bindB := newChanBindPair()
			client := NewDevice(context.Background(), newChanTun(), bindA, NewLogger(LogLevelError, "client: "), 1)
			server := NewDevice(context.Background(), newChanTun(), bindB, NewLogger(LogLevelError, "server: "), 1)
			t.Cleanup(client.Close)
			t.Cleanup(server.Close)
			if err := client.SetPrivateKey(clientPrivate); err != nil {
				t.Fatal(err)
			}
			if err := server.SetPrivateKey(serverPrivate); err != nil {
				t.Fatal(err)
			}
			clientPeer, err := client.NewPeer(serverPrivate.publicKey())
			if err != nil {
				t.Fatal(err)
			}
			clientPeer.SetPresharedKey(test.clientPSK)
			server.SetPeerLookupFunc(func(publicKey NoisePublicKey) (*NewPeerConfig, bool) {
				if publicKey != clientPrivate.publicKey() {
					return nil, false
				}
				return &NewPeerConfig{
					AllowedIPs:   []netip.Prefix{netip.PrefixFrom(testIPA, 32)},
					PresharedKey: test.serverPSK,
				}, true
			})
			initiation, err := client.CreateMessageInitiation(clientPeer)
			if err != nil {
				t.Fatal(err)
			}
			serverPeer := server.ConsumeMessageInitiation(initiation, nil)
			if serverPeer == nil {
				t.Fatal("lazy server peer did not consume initiation")
			}
			response, err := server.CreateMessageResponse(serverPeer)
			if err != nil {
				t.Fatal(err)
			}
			acceptedPeer := client.ConsumeMessageResponse(response)
			if !test.accept {
				if acceptedPeer != nil {
					t.Fatal("authenticated response with a mismatched PSK")
				}
				return
			}
			if acceptedPeer != clientPeer {
				t.Fatal("matching PSK did not authenticate response")
			}
			if err := clientPeer.BeginSymmetricSession(); err != nil {
				t.Fatal(err)
			}
			if err := serverPeer.BeginSymmetricSession(); err != nil {
				t.Fatal(err)
			}
			clientKey := clientPeer.keypairs.Current()
			serverKey := serverPeer.keypairs.next.Load()
			payload := []byte("lazy peer transport payload")
			ciphertext := clientKey.send.Seal(nil, ZeroNonce[:], payload, nil)
			plaintext, err := serverKey.receive.Open(nil, ZeroNonce[:], ciphertext, nil)
			if err != nil || !bytes.Equal(plaintext, payload) {
				t.Fatal("derived client-to-server transport keys do not match")
			}
			ciphertext = serverKey.send.Seal(nil, ZeroNonce[:], payload, nil)
			plaintext, err = clientKey.receive.Open(nil, ZeroNonce[:], ciphertext, nil)
			if err != nil || !bytes.Equal(plaintext, payload) {
				t.Fatal("derived server-to-client transport keys do not match")
			}
		})
	}
}

func TestSetPresharedKeyClearsOptionalKey(t *testing.T) {
	peer := &Peer{}
	peer.SetPresharedKey(NoisePresharedKey{1})
	peer.SetPresharedKey(NoisePresharedKey{})
	peer.handshake.mutex.RLock()
	defer peer.handshake.mutex.RUnlock()
	if peer.handshake.presharedKey != (NoisePresharedKey{}) {
		t.Fatal("zero PSK did not clear the optional key")
	}
}
