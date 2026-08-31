package v2ray

import (
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
)

func TestNewClientTransportErrorReturnsNilClient(t *testing.T) {
	client, err := NewClientTransport(
		t.Context(),
		nil,
		M.ParseSocksaddr("example.com:80"),
		option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeHTTPUpgrade,
			HTTPUpgradeOptions: option.V2RayHTTPUpgradeOptions{
				Path: "/%zz",
			},
		},
		nil,
	)
	if err == nil {
		t.Fatal("NewClientTransport() error = nil, want an invalid path error")
	}
	if client != nil {
		t.Fatalf("NewClientTransport() client = %#v (%T), want a nil interface", client, client)
	}
}
