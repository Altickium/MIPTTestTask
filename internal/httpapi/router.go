package httpapi

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"pollservice/internal/domain"
	"pollservice/internal/identity"
)

type PollAPI interface {
	Create(context.Context, domain.CreatePoll) (domain.Poll, error)
	ListRecent(context.Context, int) ([]domain.Poll, error)
	Publish(context.Context, string) (domain.Poll, error)
	PublicBySlug(context.Context, string, time.Time) (domain.Poll, error)
	GetByID(context.Context, string) (domain.Poll, error)
}
type VoteAPI interface {
	Vote(context.Context, string, string, []string, time.Time) (domain.VoteStatus, error)
}
type ResultAPI interface {
	Results(context.Context, string) (domain.Poll, domain.Result, error)
	Close(context.Context, string) (domain.Poll, domain.Result, error)
}
type RateLimiter interface {
	AllowRate(context.Context, string, int64) (bool, error)
	Ping(context.Context) error
}
type QRGenerator interface{ GenerateSVG(string) ([]byte, error) }

type API struct {
	polls          PollAPI
	votes          VoteAPI
	results        ResultAPI
	rate           RateLimiter
	cookies        *identity.CookieManager
	dedupKey       []byte
	trustedProxies []*net.IPNet
	rateLimit      int64
	db             *sql.DB
	log            *slog.Logger
	metrics        *Metrics
	voteTimeout    time.Duration
	qr             QRGenerator
	publicBaseURL  string
	qrCache        sync.Map
}
type Dependencies struct {
	Polls          PollAPI
	Votes          VoteAPI
	Results        ResultAPI
	Rate           RateLimiter
	Cookies        *identity.CookieManager
	DedupKey       []byte
	TrustedProxies []*net.IPNet
	RateLimit      int64
	AdminToken     string
	DB             *sql.DB
	Log            *slog.Logger
	VoteTimeout    time.Duration
	Metrics        *Metrics
	QR             QRGenerator
	PublicBaseURL  string
}

func NewRouter(d Dependencies) http.Handler {
	metrics := d.Metrics
	if metrics == nil {
		metrics = &Metrics{}
	}
	a := &API{polls: d.Polls, votes: d.Votes, results: d.Results, rate: d.Rate, cookies: d.Cookies, dedupKey: d.DedupKey, trustedProxies: d.TrustedProxies, rateLimit: d.RateLimit, db: d.DB, log: d.Log, metrics: metrics, voteTimeout: d.VoteTimeout, qr: d.QR, publicBaseURL: d.PublicBaseURL}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("GET /p/{slug}", a.pollPage)
	mux.HandleFunc("GET /admin", a.adminPage)
	mux.HandleFunc("GET /static/{name}", a.staticAsset)
	mux.HandleFunc("GET /openapi.yaml", a.openAPISpec)
	mux.HandleFunc("GET /health/live", a.live)
	mux.HandleFunc("GET /health/ready", a.ready)
	mux.Handle("GET /metrics", a.metrics)
	mux.HandleFunc("GET /api/v1/polls/{slug}", a.getPoll)
	mux.HandleFunc("POST /api/v1/polls/{id}/votes", a.vote)
	admin := http.NewServeMux()
	admin.HandleFunc("GET /api/v1/admin/polls", a.listPolls)
	admin.HandleFunc("POST /api/v1/admin/polls", a.createPoll)
	admin.HandleFunc("POST /api/v1/admin/polls/{id}/publish", a.publishPoll)
	admin.HandleFunc("POST /api/v1/admin/polls/{id}/close", a.closePoll)
	admin.HandleFunc("GET /api/v1/admin/polls/{id}/results", a.resultsPoll)
	admin.HandleFunc("GET /api/v1/admin/polls/{id}/qr.svg", a.qrPoll)
	mux.Handle("/api/v1/admin/", adminAuth(d.AdminToken, admin))
	return middleware(securityHeaders(mux), d.Log, a.metrics)
}

func (a *API) live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "not_ready", "database unavailable")
		return
	}
	if err := a.rate.Ping(ctx); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "not_ready", "redis unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
