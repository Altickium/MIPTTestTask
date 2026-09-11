package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"pollservice/internal/domain"
)

const (
	defaultPollListLimit = 50
	maxPollListLimit     = 100
)

type pollListResponse struct {
	Polls []domain.Poll `json:"polls"`
}

func (a *API) listPolls(w http.ResponseWriter, r *http.Request) {
	limit := defaultPollListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxPollListLimit {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	polls, err := a.polls.ListRecent(r.Context(), limit)
	if err != nil {
		handleDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pollListResponse{Polls: polls})
}

type createRequest struct {
	Question   string          `json:"question"`
	Type       domain.PollType `json:"type"`
	MaxChoices int             `json:"max_choices"`
	StartsAt   time.Time       `json:"starts_at"`
	EndsAt     time.Time       `json:"ends_at"`
	Options    []struct {
		Text string `json:"text"`
	} `json:"options"`
}

func (a *API) createPoll(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var req createRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	texts := make([]string, len(req.Options))
	for i, o := range req.Options {
		texts[i] = o.Text
	}
	p, err := a.polls.Create(r.Context(), domain.CreatePoll{Question: req.Question, Type: req.Type, MaxChoices: req.MaxChoices, StartsAt: req.StartsAt, EndsAt: req.EndsAt, Options: texts})
	if err != nil {
		handleDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}
func (a *API) publishPoll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid poll id")
		return
	}
	p, err := a.polls.Publish(r.Context(), id)
	if err != nil {
		handleDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
func (a *API) closePoll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid poll id")
		return
	}
	p, result, err := a.results.Close(r.Context(), id)
	if err != nil {
		handleDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resultResponse(p, result))
}
func (a *API) resultsPoll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid poll id")
		return
	}
	p, result, err := a.results.Results(r.Context(), id)
	if err != nil {
		handleDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resultResponse(p, result))
}

type optionResult struct {
	ID                 string  `json:"id"`
	Text               string  `json:"text"`
	Count              int64   `json:"count"`
	ParticipantPercent float64 `json:"participant_percent"`
}
type resultsResponse struct {
	PollID              string            `json:"poll_id"`
	Status              domain.PollStatus `json:"status"`
	Source              string            `json:"source"`
	AsOf                time.Time         `json:"as_of"`
	Participants        int64             `json:"participants_count"`
	Options             []optionResult    `json:"options"`
	FinalizationPending bool              `json:"finalization_pending,omitempty"`
}

func resultResponse(p domain.Poll, r domain.Result) resultsResponse {
	out := resultsResponse{PollID: p.ID, Status: r.Status, Source: r.Source, AsOf: r.AsOf, Participants: r.Participants}
	for _, o := range p.Options {
		count := r.Options[o.ID]
		pct := 0.0
		if r.Participants > 0 {
			pct = float64(count) * 100 / float64(r.Participants)
		}
		out.Options = append(out.Options, optionResult{ID: o.ID, Text: o.Text, Count: count, ParticipantPercent: pct})
	}
	return out
}
