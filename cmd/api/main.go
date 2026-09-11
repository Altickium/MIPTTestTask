package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"pollservice/internal/config"
	"pollservice/internal/httpapi"
	"pollservice/internal/identity"
	"pollservice/internal/postgres"
	"pollservice/internal/qrsvg"
	"pollservice/internal/redisstore"
	"pollservice/internal/service"
	"pollservice/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(2)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB, PoolSize: 50})
	defer rdb.Close()
	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := db.PingContext(startupCtx); err != nil {
		cancel()
		log.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	if err := rdb.Ping(startupCtx).Err(); err != nil {
		cancel()
		log.Error("redis unavailable", "error", err)
		os.Exit(1)
	}
	repo := postgres.NewPollRepository(db, cfg.VoteBuckets)
	if err := repo.ValidateActiveBuckets(startupCtx); err != nil {
		cancel()
		log.Error("unsafe VOTE_BUCKETS configuration", "error", err)
		os.Exit(1)
	}
	cancel()
	store := redisstore.New(rdb, cfg.VoteBuckets, cfg.RedisRetention, cfg.MaxRedisTTL)
	cache := service.NewPollCache(cfg.PollCacheTTL)
	pollService := service.NewPollService(repo, store, cache)
	voteService := service.NewVoteService(repo, store, cache, cfg.DedupHMACKey)
	resultService := service.NewResultService(repo, store, cache)
	metrics := &httpapi.Metrics{}
	handler := httpapi.NewRouter(httpapi.Dependencies{Polls: pollService, Votes: voteService, Results: resultService, Rate: store, Cookies: identity.NewCookieManager(cfg.CookieSigningKey, cfg.CookieSecure), DedupKey: cfg.DedupHMACKey, TrustedProxies: cfg.TrustedProxies, RateLimit: cfg.RateLimitPerMinute, AdminToken: cfg.AdminToken, DB: db, Log: log, VoteTimeout: cfg.VoteRequestTimeout, Metrics: metrics, QR: qrsvg.Generator{}, PublicBaseURL: cfg.PublicBaseURL})
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go worker.NewSnapshot(repo, resultService, cfg.SnapshotInterval, log, metrics).Run(ctx)
	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "vote_buckets", cfg.VoteBuckets, "cookie_secure", cfg.CookieSecure)
		errCh <- server.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", "error", err)
		}
	}
}
