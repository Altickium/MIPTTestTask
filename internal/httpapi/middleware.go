package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type contextKey int

const requestIDKey contextKey = 1

func requestID(ctx context.Context) string { v, _ := ctx.Value(requestIDKey).(string); return v }

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func middleware(next http.Handler, log *slog.Logger, metrics *Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error("panic recovered", "request_id", requestID(r.Context()), "error", recovered)
				if sw.status == 0 {
					writeError(sw, r, http.StatusInternalServerError, "internal_error", "internal server error")
				}
			}
			duration := time.Since(start)
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			route := routePattern(r)
			metrics.ObserveHTTP(route, r.Method, status, duration)
			log.Info("request", "request_id", requestID(r.Context()), "method", r.Method, "route", route, "status", status, "duration", duration.String())
		}()
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 128 {
			id = randomID()
		}
		sw.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
		next.ServeHTTP(sw, r)
	})
}

func routePattern(r *http.Request) string {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/v1/admin/polls/") && strings.HasSuffix(p, "/publish"):
		return "/api/v1/admin/polls/{id}/publish"
	case strings.HasPrefix(p, "/api/v1/admin/polls/") && strings.HasSuffix(p, "/close"):
		return "/api/v1/admin/polls/{id}/close"
	case strings.HasPrefix(p, "/api/v1/admin/polls/") && strings.HasSuffix(p, "/results"):
		return "/api/v1/admin/polls/{id}/results"
	case strings.HasPrefix(p, "/api/v1/admin/polls/") && strings.HasSuffix(p, "/qr.svg"):
		return "/api/v1/admin/polls/{id}/qr.svg"
	case p == "/api/v1/admin/polls":
		return p
	case strings.HasPrefix(p, "/api/v1/polls/") && strings.HasSuffix(p, "/votes"):
		return "/api/v1/polls/{id}/votes"
	case strings.HasPrefix(p, "/api/v1/polls/"):
		return "/api/v1/polls/{slug}"
	case strings.HasPrefix(p, "/p/"):
		return "/p/{slug}"
	case strings.HasPrefix(p, "/static/"):
		return "/static/{name}"
	case p == "/", p == "/admin", p == "/openapi.yaml", p == "/health/live", p == "/health/ready", p == "/metrics":
		return p
	default:
		return "/unmatched"
	}
}
func randomID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

func adminAuth(token string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		got := sha256.Sum256([]byte(provided))
		if provided == "" || subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "valid bearer token required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
