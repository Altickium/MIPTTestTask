package identity

import (
	"net/http/httptest"
	"testing"
)

func TestCookieSignVerifyAndTamper(t *testing.T) {
	m := NewCookieManager([]byte("01234567890123456789012345678901"), false)
	r := httptest.NewRequest("GET", "/", nil)
	id, c, err := m.DeviceID(r)
	if err != nil || c == nil {
		t.Fatalf("DeviceID err=%v cookie=%v", err, c)
	}
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.AddCookie(c)
	got, next, err := m.DeviceID(r2)
	if err != nil || next != nil || got != id {
		t.Fatalf("got=%q next=%v err=%v", got, next, err)
	}
	c.Value = c.Value[:len(c.Value)-1] + "x"
	r3 := httptest.NewRequest("GET", "/", nil)
	r3.AddCookie(c)
	tampered, newCookie, err := m.DeviceID(r3)
	if err != nil || newCookie == nil || tampered == id {
		t.Fatalf("tampered cookie was accepted")
	}
}

func TestDeviceHashSeparatedByPoll(t *testing.T) {
	key := []byte("abcdefghijklmnopqrstuvwxyz012345")
	a := DeviceHash(key, "poll-a", "device")
	b := DeviceHash(key, "poll-b", "device")
	if a == b {
		t.Fatal("hash must be poll scoped")
	}
	if a != DeviceHash(key, "poll-a", "device") {
		t.Fatal("hash is not deterministic")
	}
}
