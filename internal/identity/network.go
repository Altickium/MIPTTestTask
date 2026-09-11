package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
)

func NetworkHash(r *http.Request, key []byte, trustedProxies []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remoteIP := net.ParseIP(host)
	trusted := false
	for _, network := range trustedProxies {
		if remoteIP != nil && network.Contains(remoteIP) {
			trusted = true
			break
		}
	}
	if trusted {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			host = forwarded
		}
	}
	ip := net.ParseIP(host)
	network := "invalid"
	if ip4 := ip.To4(); ip4 != nil {
		network = net.IP(ip4).Mask(net.CIDRMask(24, 32)).String() + "/24"
	} else if ip16 := ip.To16(); ip16 != nil {
		network = net.IP(ip16).Mask(net.CIDRMask(56, 128)).String() + "/56"
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("rate\n" + network))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
