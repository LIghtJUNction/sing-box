//go:build with_ebpf && (linux || android)

package ebpf

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sagernet/sing-box/log"

	"golang.org/x/sys/unix"
)

func startSharedTCRetryTest(t *testing.T, reconcile func() error) *sharedTCManager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	manager := &sharedTCManager{
		logger: log.NewNOPFactory().Logger(),
		cancel: cancel,
		done:   make(chan struct{}),
		wake:   make(chan struct{}, 1),
	}
	go manager.loop(ctx, reconcile)
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatal(err)
		}
	})
	synctest.Wait()
	return manager
}

func TestSharedTCRetriesWithBoundedBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts atomic.Int32
		manager := startSharedTCRetryTest(t, func() error {
			attempts.Add(1)
			if attempts.Load() <= 7 {
				return unix.EAGAIN
			}
			return nil
		})
		manager.Wake()
		synctest.Wait()
		if attempts.Load() != 1 {
			t.Fatalf("network event triggered %d refresh attempts, want 1", attempts.Load())
		}
		for index, delay := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second, 10 * time.Second, 2 * time.Minute} {
			time.Sleep(delay - time.Nanosecond)
			synctest.Wait()
			if int(attempts.Load()) != index+1 {
				t.Fatalf("refresh retried too early after attempt %d: got %d attempts", index+1, attempts.Load())
			}
			time.Sleep(time.Nanosecond)
			synctest.Wait()
			if int(attempts.Load()) != index+2 {
				t.Fatalf("refresh did not run after %s: got %d attempts, want %d", delay, attempts.Load(), index+2)
			}
		}
	})
}

func TestSharedTCNetworkWakeReplacesPendingRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts atomic.Int32
		manager := startSharedTCRetryTest(t, func() error {
			attempts.Add(1)
			if attempts.Load() == 1 {
				return unix.EAGAIN
			}
			return nil
		})
		manager.Wake()
		synctest.Wait()
		time.Sleep(500 * time.Millisecond)
		manager.Wake()
		synctest.Wait()
		if attempts.Load() != 2 {
			t.Fatalf("network event did not retry immediately: %d attempts", attempts.Load())
		}
		time.Sleep(2*time.Minute - time.Nanosecond)
		synctest.Wait()
		if attempts.Load() != 2 {
			t.Fatalf("successful refresh retained an old retry deadline: %d attempts", attempts.Load())
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if attempts.Load() != 3 {
			t.Fatalf("successful refresh did not restore health checks: %d attempts", attempts.Load())
		}
	})
}

func TestSharedTCCloseCancelsPendingRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts atomic.Int32
		manager := startSharedTCRetryTest(t, func() error {
			attempts.Add(1)
			return unix.EAGAIN
		})
		manager.Wake()
		synctest.Wait()
		if attempts.Load() != 1 {
			t.Fatalf("network event triggered %d refresh attempts, want 1", attempts.Load())
		}
		if err := manager.Close(); err != nil {
			t.Fatal(err)
		}
		manager.Wake()
		time.Sleep(4 * time.Minute)
		synctest.Wait()
		if attempts.Load() != 1 {
			t.Fatalf("closed manager continued refreshing: %d attempts", attempts.Load())
		}
	})
}

func TestSharedTCCancelDuringRetryDiscardsQueuedWake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts atomic.Int32
		release := make(chan struct{})
		manager := startSharedTCRetryTest(t, func() error {
			attempts.Add(1)
			if attempts.Load() == 2 {
				<-release
			}
			return unix.EAGAIN
		})
		manager.Wake()
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		if attempts.Load() != 2 {
			t.Fatalf("timer did not trigger a retry: %d attempts", attempts.Load())
		}
		manager.Wake()
		manager.cancel()
		close(release)
		if err := manager.Close(); err != nil {
			t.Fatal(err)
		}
		if attempts.Load() != 2 {
			t.Fatalf("canceled manager processed queued network event: %d attempts", attempts.Load())
		}
	})
}
