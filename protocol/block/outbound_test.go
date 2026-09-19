package block

import (
	"context"
	"errors"
	"syscall"
	"testing"

	"github.com/sagernet/sing-box/log"
	R "github.com/sagernet/sing-box/route/rule"
	M "github.com/sagernet/sing/common/metadata"
)

func TestBlockPreservesPermissionCauseAndMarksPolicyRejection(t *testing.T) {
	outbound := &Outbound{logger: log.NewNOPFactory().Logger()}
	_, tcpErr := outbound.DialContext(context.Background(), "tcp", M.ParseSocksaddr("127.0.0.1:443"))
	_, udpErr := outbound.ListenPacket(context.Background(), M.ParseSocksaddr("127.0.0.1:443"))
	for _, err := range []error{tcpErr, udpErr} {
		if !R.IsRejected(err) || !errors.Is(err, syscall.EPERM) {
			t.Fatalf("expected policy rejection preserving EPERM, got %v", err)
		}
	}
}
