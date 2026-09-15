//go:build go1.24

package tls

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestECHFetchContextTracksNestedConfigs(t *testing.T) {
	first := new(ECHClientConfig)
	second := new(ECHClientConfig)
	ctx := withECHFetchContext(context.Background(), first)
	ctx = withECHFetchContext(ctx, second)
	if !hasECHFetchContext(ctx, first) {
		t.Fatal("first ECH config missing from fetch context")
	}
	if !hasECHFetchContext(ctx, second) {
		t.Fatal("second ECH config missing from fetch context")
	}
	if hasECHFetchContext(ctx, new(ECHClientConfig)) {
		t.Fatal("unrelated ECH config unexpectedly present in fetch context")
	}
}

func TestECHFetchAndHandshakeRejectsRecursiveFetch(t *testing.T) {
	config := new(ECHClientConfig)
	ctx := withECHFetchContext(context.Background(), config)
	result := make(chan error, 1)
	go func() {
		_, err := config.fetchAndHandshake(ctx, nil)
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "recursive ECH config fetch") {
			t.Fatalf("expected recursive ECH fetch error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("recursive ECH fetch blocked instead of failing")
	}
}
