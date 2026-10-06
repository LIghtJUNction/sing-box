package group

import (
	"context"
	"testing"
	"time"
)

func TestCloseCancelsManualTestWithIndependentContext(t *testing.T) {
	group, node := newHangingGroup(t)
	t.Cleanup(func() { group.Close() })
	done := make(chan struct{})
	go func() {
		_, _ = group.URLTest(context.Background())
		close(done)
	}()
	select {
	case <-node.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("manual test never reached the node")
	}
	if err := group.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("manual test outlived its group")
	}
}
