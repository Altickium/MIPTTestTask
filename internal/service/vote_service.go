package service

import (
	"context"
	"time"

	"pollservice/internal/domain"
	"pollservice/internal/identity"
)

type VoteStore interface {
	Record(context.Context, domain.VoteCommand) (domain.VoteStatus, error)
}
type PollLoader interface {
	GetByID(context.Context, string) (domain.Poll, error)
}

type VoteService struct {
	polls    PollLoader
	store    VoteStore
	cache    *PollCache
	dedupKey []byte
}

func NewVoteService(polls PollLoader, store VoteStore, cache *PollCache, dedupKey []byte) *VoteService {
	return &VoteService{polls: polls, store: store, cache: cache, dedupKey: append([]byte(nil), dedupKey...)}
}
func (s *VoteService) Vote(ctx context.Context, pollID, deviceID string, optionIDs []string, now time.Time) (domain.VoteStatus, error) {
	loader := func(ctx context.Context, id string) (domain.Poll, error) {
		p, err := s.polls.GetByID(ctx, id)
		if err != nil {
			return domain.Poll{}, err
		}
		if p.Status != domain.Active {
			return domain.Poll{}, domain.ErrNotActive
		}
		return p, nil
	}
	p, err := s.cache.Get(ctx, pollID, loader)
	if err != nil {
		return "", err
	}
	if p.Status != domain.Active || now.Before(p.StartsAt) || !now.Before(p.EndsAt) {
		return "", domain.ErrNotActive
	}
	if err := p.ValidateSelection(optionIDs); err != nil {
		return "", err
	}
	return s.store.Record(ctx, domain.VoteCommand{PollID: p.ID, DeviceHash: identity.DeviceHash(s.dedupKey, p.ID, deviceID), OptionIDs: optionIDs, EndsAt: p.EndsAt})
}
