package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"pollservice/internal/domain"
	"pollservice/internal/identity"
)

type publicPoll struct {
	ID         string          `json:"id"`
	Slug       string          `json:"slug"`
	Question   string          `json:"question"`
	Type       domain.PollType `json:"type"`
	MaxChoices int             `json:"max_choices"`
	EndsAt     time.Time       `json:"ends_at"`
	Options    []domain.Option `json:"options"`
}

func (a *API) getPoll(w http.ResponseWriter, r *http.Request) {
	p, err := a.polls.PublicBySlug(r.Context(), r.PathValue("slug"), time.Now().UTC())
	if err != nil {
		handleDomainError(w, r, err)
		return
	}
	_, cookie, err := a.cookies.DeviceID(r)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "temporarily_unavailable", "service is temporarily unavailable")
		return
	}
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	writeJSON(w, http.StatusOK, publicPoll{ID: p.ID, Slug: p.Slug, Question: p.Question, Type: p.Type, MaxChoices: p.MaxChoices, EndsAt: p.EndsAt, Options: p.Options})
}

type voteRequest struct {
	OptionIDs []string `json:"option_ids"`
}

func (a *API) vote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid poll id")
		a.metrics.Reject("validation")
		return
	}
	if !requireJSON(w, r) {
		a.metrics.Reject("content_type")
		return
	}
	var req voteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		a.metrics.Reject("invalid_json")
		return
	}
	seen := make(map[string]struct{}, len(req.OptionIDs))
	for _, optionID := range req.OptionIDs {
		if !domain.IsUUID(optionID) {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "option_ids must contain UUIDs")
			a.metrics.Reject("validation")
			return
		}
		if _, ok := seen[optionID]; ok {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "option_ids must be unique")
			a.metrics.Reject("validation")
			return
		}
		seen[optionID] = struct{}{}
	}
	ctx, cancel := contextWithTimeout(r, a.voteTimeout)
	defer cancel()
	allowed, err := a.rate.AllowRate(ctx, identity.NetworkHash(r, a.dedupKey, a.trustedProxies), a.rateLimit)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "temporarily_unavailable", "service is temporarily unavailable")
		a.metrics.Reject("dependency_error")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeError(w, r, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
		a.metrics.Reject("rate_limited")
		return
	}
	device, cookie, err := a.cookies.DeviceID(r)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "temporarily_unavailable", "service is temporarily unavailable")
		a.metrics.Reject("identity_error")
		return
	}
	status, err := a.votes.Vote(ctx, id, device, req.OptionIDs, time.Now().UTC())
	if err != nil {
		if !errors.Is(err, domain.ErrNotActive) && !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, domain.ErrInvalidSelection) {
			a.log.Error("vote failed", "request_id", requestID(r.Context()), "poll_id", id, "error", err)
		}
		handleDomainError(w, r, err)
		a.metrics.Reject(voteRejectionReason(err))
		return
	}
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	code := http.StatusCreated
	if status == domain.VoteAlreadyRecorded {
		code = http.StatusOK
		a.metrics.duplicates.Add(1)
	} else {
		a.metrics.recorded.Add(1)
	}
	writeJSON(w, code, map[string]domain.VoteStatus{"status": status})
}

func voteRejectionReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrNotActive):
		return "not_active"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	case errors.Is(err, domain.ErrInvalidSelection):
		return "invalid_selection"
	default:
		return "dependency_error"
	}
}
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
