package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	AdminToken         string
	CookieSigningKey   []byte
	DedupHMACKey       []byte
	CookieSecure       bool
	VoteBuckets        int
	RedisRetention     time.Duration
	MaxRedisTTL        time.Duration
	PollCacheTTL       time.Duration
	SnapshotInterval   time.Duration
	VoteRequestTimeout time.Duration
	TrustProxyHeaders  bool
	TrustedProxies     []*net.IPNet
	LogLevel           slog.Level
	ShutdownTimeout    time.Duration
	RateLimitPerMinute int64
	PublicBaseURL      string
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr: env("HTTP_ADDR", ":8080"), DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisAddr: env("REDIS_ADDR", "localhost:6379"), RedisPassword: os.Getenv("REDIS_PASSWORD"),
		AdminToken: os.Getenv("ADMIN_TOKEN"), CookieSigningKey: []byte(os.Getenv("COOKIE_SIGNING_KEY")),
		DedupHMACKey: []byte(os.Getenv("DEDUP_HMAC_KEY")), RedisRetention: 24 * time.Hour,
		MaxRedisTTL: 30 * 24 * time.Hour, PollCacheTTL: 30 * time.Second,
		SnapshotInterval: 5 * time.Second, VoteRequestTimeout: 2 * time.Second,
		ShutdownTimeout: 15 * time.Second, VoteBuckets: 64, RateLimitPerMinute: 120,
		PublicBaseURL: strings.TrimRight(env("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
	}
	var err error
	if c.RedisDB, err = intEnv("REDIS_DB", 0); err != nil {
		return c, err
	}
	if c.VoteBuckets, err = intEnv("VOTE_BUCKETS", 64); err != nil {
		return c, err
	}
	if c.RateLimitPerMinute, err = int64Env("RATE_LIMIT_PER_MINUTE", 120); err != nil {
		return c, err
	}
	if c.CookieSecure, err = boolEnv("COOKIE_SECURE", false); err != nil {
		return c, err
	}
	if c.TrustProxyHeaders, err = boolEnv("TRUST_PROXY_HEADERS", false); err != nil {
		return c, err
	}
	if c.TrustProxyHeaders {
		for _, raw := range strings.Split(os.Getenv("TRUSTED_PROXY_CIDRS"), ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			_, network, parseErr := net.ParseCIDR(raw)
			if parseErr != nil {
				return c, fmt.Errorf("TRUSTED_PROXY_CIDRS: %w", parseErr)
			}
			c.TrustedProxies = append(c.TrustedProxies, network)
		}
		if len(c.TrustedProxies) == 0 {
			return c, errors.New("TRUSTED_PROXY_CIDRS is required when TRUST_PROXY_HEADERS=true")
		}
	}
	for key, target := range map[string]*time.Duration{
		"REDIS_RETENTION": &c.RedisRetention, "MAX_REDIS_TTL": &c.MaxRedisTTL,
		"POLL_CACHE_TTL": &c.PollCacheTTL, "SNAPSHOT_INTERVAL": &c.SnapshotInterval,
		"VOTE_REQUEST_TIMEOUT": &c.VoteRequestTimeout, "SHUTDOWN_TIMEOUT": &c.ShutdownTimeout,
	} {
		if *target, err = durationEnv(key, *target); err != nil {
			return c, err
		}
	}
	switch strings.ToLower(env("LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		return c, errors.New("LOG_LEVEL must be debug, info, warn, or error")
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if len(c.AdminToken) < 16 {
		return c, errors.New("ADMIN_TOKEN must contain at least 16 bytes")
	}
	if len(c.CookieSigningKey) < 32 || len(c.DedupHMACKey) < 32 {
		return c, errors.New("COOKIE_SIGNING_KEY and DEDUP_HMAC_KEY must contain at least 32 bytes")
	}
	if string(c.CookieSigningKey) == string(c.DedupHMACKey) {
		return c, errors.New("signing and dedup keys must differ")
	}
	if c.VoteBuckets < 1 || c.VoteBuckets > 4096 {
		return c, errors.New("VOTE_BUCKETS must be between 1 and 4096")
	}
	if c.RedisRetention <= 0 || c.MaxRedisTTL <= 0 || c.PollCacheTTL <= 0 || c.VoteRequestTimeout <= 0 {
		return c, errors.New("durations must be positive")
	}
	if c.RateLimitPerMinute < 1 {
		return c, errors.New("RATE_LIMIT_PER_MINUTE must be positive")
	}
	publicURL, err := url.Parse(c.PublicBaseURL)
	if err != nil || publicURL.Host == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.User != nil || (publicURL.Path != "" && publicURL.Path != "/") || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return c, errors.New("PUBLIC_BASE_URL must be an absolute http(s) origin without credentials, path, query, or fragment")
	}
	if c.CookieSecure && publicURL.Scheme != "https" {
		return c, errors.New("PUBLIC_BASE_URL must use https when COOKIE_SECURE=true")
	}
	return c, nil
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func intEnv(k string, fallback int) (int, error) {
	v := env(k, strconv.Itoa(fallback))
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", k, err)
	}
	return n, nil
}
func int64Env(k string, fallback int64) (int64, error) {
	v := env(k, strconv.FormatInt(fallback, 10))
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", k, err)
	}
	return n, nil
}
func boolEnv(k string, fallback bool) (bool, error) {
	v := env(k, strconv.FormatBool(fallback))
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", k, err)
	}
	return b, nil
}
func durationEnv(k string, fallback time.Duration) (time.Duration, error) {
	v := env(k, fallback.String())
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", k, err)
	}
	return d, nil
}
