/* SPDX-License-Identifier: MIT
 *
 * lx: Regressions for upstream e4d85ef. A paused callback must not hold the
 * timer's running lock or fire after a newer deadline replaces its own.
 */

package device

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sagernet/sing/service/pause"
)

type timerPauseManager struct {
	pause.Manager
	entered chan struct{}
	resume  chan struct{}
}

func (m *timerPauseManager) WaitActive() {
	select {
	case m.entered <- struct{}{}:
	default:
	}
	<-m.resume
}

func TestTimerDelSyncWhilePaused(t *testing.T) {
	manager := &timerPauseManager{entered: make(chan struct{}, 1), resume: make(chan struct{})}
	peer := &Peer{device: &Device{pauseManager: manager}}
	expired := make(chan struct{}, 1)
	timer := peer.NewTimer(func(*Peer) { expired <- struct{}{} })
	defer timer.DelSync()
	defer close(manager.resume)
	timer.Mod(0)
	select {
	case <-manager.entered:
	case <-time.After(time.Second):
		t.Fatal("timer did not enter the paused state")
	}
	stopped := make(chan struct{})
	go func() {
		timer.DelSync()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("DelSync blocked on a paused callback")
	}
	select {
	case <-expired:
		t.Fatal("cancelled timer expired")
	default:
	}
}

func TestTimerResetWhilePaused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := &timerPauseManager{entered: make(chan struct{}, 1), resume: make(chan struct{})}
		peer := &Peer{device: &Device{pauseManager: manager}}
		var expired atomic.Int32
		timer := peer.NewTimer(func(*Peer) { expired.Add(1) })
		defer timer.DelSync()
		timer.Mod(0)
		<-manager.entered
		timer.Mod(time.Hour)
		close(manager.resume)
		synctest.Wait()
		if got := expired.Load(); got != 0 {
			t.Errorf("stale callback expired after reset: %d expirations", got)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		if got := expired.Load(); got != 1 {
			t.Errorf("reset deadline: got %d expirations, want one", got)
		}
	})
}
