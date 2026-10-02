/* SPDX-License-Identifier: MIT */

package device

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Block a real transmission after the Noise handshake, then fill the actual
// encryption/transmission pipeline. WritePackets must wait, wake on successful
// drain, and return on Stop even while the blocked sender is still unwinding.
func TestWritePacketsBackpressure(t *testing.T) {
	for _, stop := range []bool{false, true} {
		name := "resume"
		if stop {
			name = "stop"
		}
		t.Run(name, func(t *testing.T) {
			var enabled atomic.Bool
			var releaseOnce sync.Once
			release := make(chan struct{})
			open := func() { releaseOnce.Do(func() { close(release) }) }
			pair := newPaddedDevicePairWithTap(t, func([]byte) {
				if enabled.Load() {
					<-release
				}
			})
			t.Cleanup(open)
			packet := buildIPv4Packet(testIPA, testIPB, 8)
			peer := pair.devA.allowedips.Lookup(testIPB.AsSlice())
			peer.WritePackets([][]byte{packet})
			awaitPacket(t, pair.tunB, packet, func() { peer.WritePackets([][]byte{packet}) })
			enabled.Store(true)
			packets := make([][]byte, maxQueuedOutboundPackets+256)
			for i := range packets {
				packets[i] = packet
			}
			writerDone := make(chan struct{})
			go func() {
				peer.WritePackets(packets)
				close(writerDone)
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				peer.outboundSpace.Lock()
				waiting := peer.outboundSpace.ready != nil
				peer.outboundSpace.Unlock()
				if waiting && peer.queuedOutboundPackets.Load() >= maxQueuedOutboundPackets {
					break
				}
				select {
				case <-writerDone:
					t.Fatal("writer did not wait for the full pipeline")
				default:
				}
				if time.Now().After(deadline) {
					t.Fatal("pipeline did not reach backpressure")
				}
				time.Sleep(time.Millisecond)
			}
			if stop {
				stopped := make(chan struct{})
				go func() {
					peer.Stop()
					close(stopped)
				}()
				select {
				case <-writerDone:
				case <-time.After(time.Second):
					t.Fatal("Stop did not wake the blocked writer")
				}
				open()
				select {
				case <-stopped:
				case <-time.After(5 * time.Second):
					t.Fatal("peer Stop did not finish after sender release")
				}
				if peer.queuedOutboundPackets.Load() != 0 || peer.stagedPackets.Load() != 0 {
					t.Fatal("Stop left queued packet accounting behind")
				}
				return
			}
			open()
			for range packets {
				select {
				case got := <-pair.tunB.fromDevice:
					if !bytes.Equal(got, packet) {
						t.Fatal("corrupted packet after pipeline resume")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("queued packet not delivered after resume")
				}
			}
			select {
			case <-writerDone:
			case <-time.After(time.Second):
				t.Fatal("draining pipeline did not wake writer")
			}
		})
	}
}
