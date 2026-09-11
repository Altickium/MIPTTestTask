package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pollservice/internal/domain"
	"pollservice/internal/identity"
)

type fakePollAPI struct {
	poll       domain.Poll
	polls      []domain.Poll
	createErr  error
	listErr    error
	listedWith int
}

func (f *fakePollAPI) Create(_ context.Context, in domain.CreatePoll) (domain.Poll, error) {
	if f.createErr != nil {
		return domain.Poll{}, f.createErr
	}
	return f.poll, nil
}
func (f *fakePollAPI) ListRecent(_ context.Context, limit int) ([]domain.Poll, error) {
	f.listedWith = limit
	return f.polls, f.listErr
}
func (f *fakePollAPI) Publish(context.Context, string) (domain.Poll, error) { return f.poll, nil }
func (f *fakePollAPI) PublicBySlug(context.Context, string, time.Time) (domain.Poll, error) {
	return f.poll, nil
}
func (f *fakePollAPI) GetByID(context.Context, string) (domain.Poll, error) { return f.poll, nil }

type fakeVoteAPI struct {
	status domain.VoteStatus
	err    error
}

func (f *fakeVoteAPI) Vote(context.Context, string, string, []string, time.Time) (domain.VoteStatus, error) {
	return f.status, f.err
}

type fakeResults struct{}

func (fakeResults) Results(context.Context, string) (domain.Poll, domain.Result, error) {
	return domain.Poll{}, domain.Result{}, domain.ErrNotFound
}
func (fakeResults) Close(context.Context, string) (domain.Poll, domain.Result, error) {
	return domain.Poll{}, domain.Result{}, domain.ErrNotFound
}

type fakeRate struct{}

func (fakeRate) AllowRate(context.Context, string, int64) (bool, error) { return true, nil }
func (fakeRate) Ping(context.Context) error                             { return nil }

type fakeQR struct{ content string }

func (f *fakeQR) GenerateSVG(content string) ([]byte, error) {
	f.content = content
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), nil
}

func testRouter(p *fakePollAPI, v *fakeVoteAPI) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(Dependencies{Polls: p, Votes: v, Results: fakeResults{}, Rate: fakeRate{}, Cookies: identity.NewCookieManager([]byte("01234567890123456789012345678901"), false), DedupKey: []byte("abcdefghijklmnopqrstuvwxyz012345"), RateLimit: 10, AdminToken: "abcdefghijklmnop", DB: &sql.DB{}, Log: log, VoteTimeout: time.Second, QR: &fakeQR{}, PublicBaseURL: "https://poll.example"})
}

func TestWebPagesAssetsAndOpenAPI(t *testing.T) {
	h := testRouter(&fakePollAPI{}, &fakeVoteAPI{})
	tests := []struct{ path, contains, contentType string }{{"/admin", "create-form", "text/html"}, {"/p/demo", "vote-form", "text/html"}, {"/static/app.css", ":root", "text/css"}, {"/static/poll.js", "option_ids", "text/javascript"}, {"/openapi.yaml", "openapi: 3.1.0", "application/yaml"}}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", tt.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Content-Type"), tt.contentType) {
				t.Fatalf("content-type=%q", rec.Header().Get("Content-Type"))
			}
			if !strings.Contains(rec.Body.String(), tt.contains) {
				t.Fatalf("body does not contain %q", tt.contains)
			}
			if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("security headers missing")
			}
			if strings.Contains(rec.Body.String(), "abcdefghijklmnop") {
				t.Fatal("admin token leaked into page")
			}
		})
	}
}

func TestQRCodeUsesConfiguredPublicURL(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440000"
	p := &fakePollAPI{poll: domain.Poll{ID: id, Slug: "poll-demo", Status: domain.Active}}
	qr := &fakeQR{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewRouter(Dependencies{Polls: p, Votes: &fakeVoteAPI{}, Results: fakeResults{}, Rate: fakeRate{}, Cookies: identity.NewCookieManager([]byte("01234567890123456789012345678901"), false), DedupKey: []byte("abcdefghijklmnopqrstuvwxyz012345"), RateLimit: 10, AdminToken: "abcdefghijklmnop", DB: &sql.DB{}, Log: log, VoteTimeout: time.Second, QR: qr, PublicBaseURL: "https://poll.example"})
	req := httptest.NewRequest("GET", "/api/v1/admin/polls/"+id+"/qr.svg", nil)
	req.Header.Set("Authorization", "Bearer abcdefghijklmnop")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if qr.content != "https://poll.example/p/poll-demo" {
		t.Fatalf("QR content=%q", qr.content)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("content type=%q", rec.Header().Get("Content-Type"))
	}
}

func TestMetricsContainBoundedRouteLabels(t *testing.T) {
	h := testRouter(&fakePollAPI{}, &fakeVoteAPI{})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/admin", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/random-one", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/random-two", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/v1/polls/not-a-uuid/votes", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `http_requests_total{route="/admin",method="GET",status="200"} 1`) {
		t.Fatalf("route counter missing:\n%s", body)
	}
	if !strings.Contains(body, `http_request_duration_seconds_bucket{route="/admin",le="+Inf"} 1`) {
		t.Fatal("histogram missing")
	}
	if strings.Contains(body, "poll_id=") {
		t.Fatal("high-cardinality poll_id label found")
	}
	if !strings.Contains(body, `http_requests_total{route="/unmatched",method="GET",status="404"} 2`) {
		t.Fatal("unmatched routes are not collapsed")
	}
	if strings.Contains(body, "/random-one") || strings.Contains(body, "/random-two") {
		t.Fatal("unmatched raw paths leaked into metric labels")
	}
	if !strings.Contains(body, `votes_rejected_total{reason="validation"} 1`) {
		t.Fatal("bounded rejection reason metric missing")
	}
}

func TestAdminAuthAndUnknownJSON(t *testing.T) {
	p := &fakePollAPI{}
	h := testRouter(p, &fakeVoteAPI{})
	body := []byte(`{"question":"q","type":"single_choice","max_choices":1,"starts_at":"2030-01-01T00:00:00Z","ends_at":"2030-01-01T00:01:00Z","options":[{"text":"a"},{"text":"b"}],"unknown":true}`)
	req := httptest.NewRequest("POST", "/api/v1/admin/polls", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without auth status=%d", rec.Code)
	}
	req = httptest.NewRequest("POST", "/api/v1/admin/polls", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer abcdefghijklmnop")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListRecentPolls(t *testing.T) {
	polls := []domain.Poll{
		{ID: "550e8400-e29b-41d4-a716-446655440000", Slug: "poll-new", Question: "new", Status: domain.Closed},
		{ID: "550e8400-e29b-41d4-a716-446655440001", Slug: "poll-old", Question: "old", Status: domain.Draft},
	}
	tests := []struct {
		name      string
		query     string
		wantCode  int
		wantLimit int
		listErr   error
	}{
		{name: "default limit", wantCode: http.StatusOK, wantLimit: defaultPollListLimit},
		{name: "explicit limit", query: "?limit=2", wantCode: http.StatusOK, wantLimit: 2},
		{name: "zero", query: "?limit=0", wantCode: http.StatusBadRequest},
		{name: "too large", query: "?limit=101", wantCode: http.StatusBadRequest},
		{name: "not a number", query: "?limit=many", wantCode: http.StatusBadRequest},
		{name: "database unavailable", wantCode: http.StatusServiceUnavailable, wantLimit: defaultPollListLimit, listErr: errors.New("database unavailable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &fakePollAPI{polls: polls, listErr: tt.listErr}
			h := testRouter(p, &fakeVoteAPI{})
			req := httptest.NewRequest("GET", "/api/v1/admin/polls"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer abcdefghijklmnop")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				if tt.wantLimit != 0 && p.listedWith != tt.wantLimit {
					t.Fatalf("repository limit=%d want=%d", p.listedWith, tt.wantLimit)
				}
				return
			}
			if p.listedWith != tt.wantLimit {
				t.Fatalf("repository limit=%d want=%d", p.listedWith, tt.wantLimit)
			}
			var response pollListResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Polls) != 2 || response.Polls[0].ID != polls[0].ID {
				t.Fatalf("unexpected polls: %+v", response.Polls)
			}
		})
	}
}

func TestDuplicateVoteResponse(t *testing.T) {
	pollID := "550e8400-e29b-41d4-a716-446655440000"
	h := testRouter(&fakePollAPI{}, &fakeVoteAPI{status: domain.VoteAlreadyRecorded})
	req := httptest.NewRequest("POST", "/api/v1/polls/"+pollID+"/votes", bytes.NewBufferString(`{"option_ids":["550e8400-e29b-41d4-a716-446655440001"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Set-Cookie") == "" {
		t.Fatal("device cookie not set")
	}
}
