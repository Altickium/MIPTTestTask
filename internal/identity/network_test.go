package identity

import (
	"net"
	"net/http/httptest"
	"testing"
)

func TestNetworkHashOnlyTrustsConfiguredProxy(t *testing.T) {
	key := []byte("abcdefghijklmnopqrstuvwxyz012345")
	direct := httptest.NewRequest("POST", "/", nil)
	direct.RemoteAddr = "192.0.2.10:1234"
	direct.Header.Set("X-Forwarded-For", "203.0.113.8")
	without := NetworkHash(direct, key, nil)
	_, cidr, _ := net.ParseCIDR("192.0.2.0/24")
	with := NetworkHash(direct, key, []*net.IPNet{cidr})
	if without == with {
		t.Fatal("trusted proxy should use forwarded address")
	}
	other := httptest.NewRequest("POST", "/", nil)
	other.RemoteAddr = "192.0.2.99:9999"
	if got := NetworkHash(other, key, nil); got != without {
		t.Fatal("IPv4 addresses in one /24 must share rate key")
	}
}
