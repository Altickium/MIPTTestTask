package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

const CookieName = "poll_device"

type CookieManager struct {
	key    []byte
	secure bool
}

func NewCookieManager(key []byte, secure bool) *CookieManager {
	return &CookieManager{key: append([]byte(nil), key...), secure: secure}
}

func (m *CookieManager) DeviceID(r *http.Request) (string, *http.Cookie, error) {
	if c, err := r.Cookie(CookieName); err == nil {
		if id, err := m.verify(c.Value); err == nil {
			return id, nil, nil
		}
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	value := m.sign(id)
	return id, &http.Cookie{Name: CookieName, Value: value, Path: "/", HttpOnly: true, Secure: m.secure, SameSite: http.SameSiteLaxMode, MaxAge: 365 * 24 * 60 * 60, Expires: time.Now().Add(365 * 24 * time.Hour)}, nil
}

func (m *CookieManager) sign(id string) string {
	payload := "v1." + id
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *CookieManager) verify(value string) (string, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", errors.New("invalid cookie")
	}
	idRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(idRaw) != 16 {
		return "", errors.New("invalid cookie")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errors.New("invalid cookie")
	}
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if subtle.ConstantTimeCompare(sig, mac.Sum(nil)) != 1 {
		return "", errors.New("invalid cookie")
	}
	return parts[1], nil
}

func DeviceHash(key []byte, pollID, deviceID string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(pollID + "\n" + deviceID))
	return hex.EncodeToString(mac.Sum(nil))
}
