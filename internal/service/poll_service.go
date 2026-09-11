package service

import (
	"context"
	"time"

	"pollservice/internal/domain"
)

type PollRepository interface {
	Create(context.Context, domain.CreatePoll) (domain.Poll, error)
	ListRecent(context.Context, int) ([]domain.Poll, error)
	GetByID(context.Context, string) (domain.Poll, error)
	GetBySlug(context.Context, string) (domain.Poll, error)
	Publish(context.Context, string, time.Time) (domain.Poll, error)
	RevertPublish(context.Context, string, time.Time) error
}

type GateStore interface {
	SetupGates(context.Context, string, time.Time) error
	DeleteGates(context.Context, string) error
}

type PollService struct {
	repo  PollRepository
	gates GateStore
	cache *PollCache
}

func NewPollService(repo PollRepository, gates GateStore, cache *PollCache) *PollService {
	return &PollService{repo: repo, gates: gates, cache: cache}
}
func (s *PollService) Create(ctx context.Context, in domain.CreatePoll) (domain.Poll, error) {
	return s.repo.Create(ctx, in)
}

func (s *PollService) ListRecent(ctx context.Context, limit int) ([]domain.Poll, error) {
	return s.repo.ListRecent(ctx, limit)
}

func (s *PollService) Publish(ctx context.Context, id string) (domain.Poll, error) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	p, err := s.repo.Publish(ctx, id, now)
	if err != nil {
		return domain.Poll{}, err
	}
	if err := s.gates.SetupGates(ctx, p.ID, p.EndsAt); err != nil {
		if p.PublishedAt != nil && p.PublishedAt.Equal(now) {
			if revertErr := s.repo.RevertPublish(ctx, p.ID, now); revertErr == nil {
				_ = s.gates.DeleteGates(ctx, p.ID)
			}
		}
		return domain.Poll{}, err
	}
	s.cache.Put(p)
	return p, nil
}

func (s *PollService) PublicBySlug(ctx context.Context, slug string, now time.Time) (domain.Poll, error) {
	p, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return domain.Poll{}, err
	}
	if p.Status != domain.Active || now.Before(p.StartsAt) || !now.Before(p.EndsAt) {
		return domain.Poll{}, domain.ErrNotFound
	}
	return p, nil
}
func (s *PollService) GetByID(ctx context.Context, id string) (domain.Poll, error) {
	return s.repo.GetByID(ctx, id)
}
