package config

import "testing"

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ADMIN_TOKEN", "0123456789abcdef")
	t.Setenv("COOKIE_SIGNING_KEY", "01234567890123456789012345678901")
	t.Setenv("DEDUP_HMAC_KEY", "abcdefghijklmnopqrstuvwxyz012345")
	t.Setenv("TRUST_PROXY_HEADERS", "false")
	t.Setenv("COOKIE_SECURE", "false")
}

func TestPublicBaseURLValidation(t *testing.T) {
	tests := []struct {
		name, value     string
		secure, wantErr bool
	}{{"local", "http://localhost:8080", false, false}, {"production", "https://poll.example", true, false}, {"path", "https://poll.example/base", false, true}, {"credentials", "https://user@poll.example", false, true}, {"secure cookie over http", "http://poll.example", true, true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("PUBLIC_BASE_URL", tt.value)
			if tt.secure {
				t.Setenv("COOKIE_SECURE", "true")
			}
			_, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
