package dns

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/dnstrack"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/service"

	mDNS "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// lx: SPEC 018 — rejecting a cached answer is still a failed resolution and
// must emit exactly one event with the request's process and resolver identity.
func TestResponseRejectionEmitsOneFailure_LX(t *testing.T) {
	for _, path := range []string{"exchange", "async exchange", "lookup"} {
		t.Run(path, func(t *testing.T) {
			for _, cached := range []bool{false, true} {
				name := "fresh"
				if cached {
					name = "cached"
				}
				t.Run(name, func(t *testing.T) {
					manager := dnstrack.NewManager()
					t.Cleanup(func() { _ = manager.Close() })
					owner := new(adapter.ConnectionOwner)
					ctx := service.ContextWithPtr(context.Background(), manager)
					ctx = adapter.WithContext(ctx, &adapter.InboundContext{ProcessInfo: owner})
					client := NewClient(ClientOptions{Context: ctx, Logger: log.NewNOPFactory().Logger()})
					transport := &fakeDNSTransport{
						tag:       "resolver",
						address:   netip.MustParseAddr("192.0.2.1"),
						immediate: true,
					}
					message := new(mDNS.Msg)
					message.SetQuestion("rejected.example.", mDNS.TypeA)
					options := adapter.DNSQueryOptions{Strategy: C.DomainStrategyIPv4Only}
					if cached {
						_, err := client.Exchange(ctx, transport, message.Copy(), options, nil)
						require.NoError(t, err)
					}
					// Subscribe after warming the real cache: only the rejected
					// resolution belongs to this observation window.
					subscription, _, err := manager.SubscribeEvents()
					require.NoError(t, err)
					t.Cleanup(func() { manager.UnSubscribeEvents(subscription) })
					checkerCalls := 0
					checker := func(response *mDNS.Msg) bool {
						checkerCalls++
						return false
					}
					switch path {
					case "exchange":
						_, err = client.Exchange(ctx, transport, message.Copy(), options, checker)
					case "async exchange":
						completed := make(chan error, 1)
						client.ExchangeAsync(ctx, transport, message.Copy(), options, checker, func(_ *mDNS.Msg, exchangeErr error) {
							completed <- exchangeErr
						})
						err = <-completed
					case "lookup":
						_, err = client.Lookup(ctx, transport, "rejected.example", options, checker)
					}
					if cached {
						require.ErrorIs(t, err, ErrResponseRejectedCached)
					} else {
						require.ErrorIs(t, err, ErrResponseRejected)
					}
					require.Equal(t, 1, checkerCalls)
					require.EqualValues(t, 1, transport.queryCount.Load(), "cache rejection must not query the transport again")

					// A FIFO marker drains preceding events without relying on
					// scheduling delays to detect missing or duplicate emissions.
					manager.Emit(dnstrack.QueryEvent{Domain: "end-of-observation"})
					deadline := time.NewTimer(time.Second)
					defer deadline.Stop()
					var events []dnstrack.QueryEvent
					for {
						select {
						case event := <-subscription:
							if event.Domain == "end-of-observation" {
								require.Len(t, events, 1)
								event = events[0]
								require.Equal(t, "rejected.example", event.Domain)
								require.Equal(t, uint16(mDNS.TypeA), event.QueryType)
								require.EqualValues(t, mDNS.RcodeSuccess, event.Rcode)
								require.Equal(t, dnstrack.SourceFailed, event.Source)
								require.True(t, event.Failed)
								require.Same(t, owner, event.ProcessInfo)
								require.Equal(t, transport.Tag(), event.DNSServer)
								require.Equal(t, transport.Type(), event.DNSServerType)
								cause := "rejected"
								if cached {
									cause = "rejected (cached)"
								}
								require.Equal(t, cause, event.Error)
								return
							}
							events = append(events, event)
						case <-deadline.C:
							t.Fatal("DNS observation marker was not delivered")
						}
					}
				})
			}
		})
	}
}
