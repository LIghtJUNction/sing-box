package endpoint

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	M "github.com/sagernet/sing/common/metadata"
)

type lifecycleEndpoint struct {
	Adapter
	initializeErr error
	cleanup       func(context.Context) error
	closed        atomic.Int32
}

func (e *lifecycleEndpoint) Start(stage adapter.StartStage, scope *adapter.Scope) error {
	if stage == adapter.StartStateInitialize {
		scope.Add(func() error {
			e.closed.Add(1)
			if e.cleanup != nil {
				return e.cleanup(scope.Context())
			}
			return nil
		})
		return e.initializeErr
	}
	return nil
}

func (e *lifecycleEndpoint) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	return nil, errors.New("unused test dial")
}

func (e *lifecycleEndpoint) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("unused test listen")
}

func lifecycleManager(t *testing.T, endpoints ...*lifecycleEndpoint) (*Manager, *adapter.Scope) {
	t.Helper()
	registry := NewRegistry()
	Register[struct{}](registry, "test", func(_ context.Context, _ adapter.Router, _ log.ContextLogger, tag string, _ struct{}) (adapter.Endpoint, error) {
		for _, endpoint := range endpoints {
			if endpoint.Tag() == tag {
				return endpoint, nil
			}
		}
		return nil, errors.New("missing test endpoint")
	})
	manager := NewManager(registry)
	logger := log.NewNOPFactory().Logger()
	for _, endpoint := range endpoints {
		if err := manager.Create(context.Background(), nil, logger, endpoint.Tag(), "test", nil); err != nil {
			t.Fatal(err)
		}
	}
	return manager, adapter.NewScope(context.Background(), logger)
}

func TestEndpointScopeCloseJoinsBoundedParallelCleanup(t *testing.T) {
	entered := make(chan struct{}, 12)
	release := make(chan struct{})
	endpoints := make([]*lifecycleEndpoint, 12)
	for i := range endpoints {
		endpoints[i] = &lifecycleEndpoint{
			Adapter: NewAdapter("test", strconv.Itoa(i), nil, nil),
			cleanup: func(ctx context.Context) error {
				if !errors.Is(ctx.Err(), context.Canceled) {
					return errors.New("cleanup context was not canceled")
				}
				entered <- struct{}{}
				<-release
				return nil
			},
		}
	}
	manager, scope := lifecycleManager(t, endpoints...)
	if err := scope.Start("endpoint", manager, adapter.StartStateInitialize); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- scope.Close() }()
	for i := 0; i < 8; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("parallel cleanup did not enter all eight workers")
		}
	}
	select {
	case <-entered:
		close(release)
		t.Fatal("cleanup exceeded its eight-worker limit")
	case <-closed:
		close(release)
		t.Fatal("cleanup returned before workers finished")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range endpoints {
		if actual := endpoint.closed.Load(); actual != 1 {
			t.Fatalf("endpoint %s cleaned %d times", endpoint.Tag(), actual)
		}
	}
}

func TestEndpointInitializationFailureStillOwnsCleanup(t *testing.T) {
	failure := errors.New("initialization failed")
	closeFailure := errors.New("cleanup failed")
	first := &lifecycleEndpoint{Adapter: NewAdapter("test", "first", nil, nil), cleanup: func(context.Context) error { return closeFailure }}
	failed := &lifecycleEndpoint{Adapter: NewAdapter("test", "failed", nil, nil), initializeErr: failure}
	untouched := &lifecycleEndpoint{Adapter: NewAdapter("test", "untouched", nil, nil)}
	manager, scope := lifecycleManager(t, first, failed, untouched)
	if err := scope.Start("endpoint", manager, adapter.StartStateInitialize); !errors.Is(err, failure) {
		t.Fatalf("unexpected initialize result: %v", err)
	}
	if err := scope.Close(); !errors.Is(err, closeFailure) {
		t.Fatalf("cleanup error lost: %v", err)
	}
	if first.closed.Load() != 1 || failed.closed.Load() != 1 || untouched.closed.Load() != 0 {
		t.Fatalf("partial initialization cleanup: first=%d failed=%d untouched=%d", first.closed.Load(), failed.closed.Load(), untouched.closed.Load())
	}
}

func TestEmptyEndpointScopeClosesWithoutWorkers(t *testing.T) {
	manager, scope := lifecycleManager(t)
	if err := scope.Start("endpoint", manager, adapter.StartStateInitialize); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- scope.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("empty endpoint cleanup blocked")
	}
}
