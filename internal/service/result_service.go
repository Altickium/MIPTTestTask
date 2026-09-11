package service

import (
	"context"
	"time"

	"pollservice/internal/domain"
)

type ResultRepository interface {
	GetByID(context.Context, string) (domain.Poll, error)
	SaveSnapshot(context.Context, domain.Poll, domain.Totals, bool) (domain.Result, error)
	LoadResult(context.Context, string) (domain.Result, error)
}
type ResultStore interface {
	Totals(context.Context, string, []string) (domain.Totals, error)
	CloseGates(context.Context, string, time.Time) error
	AcquireSnapshotLock(context.Context, string, string, time.Duration) (bool, error)
	ReleaseSnapshotLock(context.Context, string, string) error
}
type ResultService struct {
	repo  ResultRepository
	store ResultStore
	cache *PollCache
}

func NewResultService(repo ResultRepository, store ResultStore, cache *PollCache) *ResultService {
	return &ResultService{repo: repo, store: store, cache: cache}
}
func optionIDs(p domain.Poll) []string {
	ids := make([]string, len(p.Options))
	for i, o := range p.Options {
		ids[i] = o.ID
	}
	return ids
}

func (s *ResultService) Results(ctx context.Context, id string) (domain.Poll, domain.Result, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Poll{}, domain.Result{}, err
	}
	if p.Status == domain.Closed {
		r, err := s.repo.LoadResult(ctx, id)
		return p, r, err
	}
	if p.Status != domain.Active {
		return domain.Poll{}, domain.Result{}, domain.ErrInvalidTransition
	}
	t, err := s.store.Totals(ctx, id, optionIDs(p))
	if err != nil {
		return domain.Poll{}, domain.Result{}, err
	}
	return p, domain.Result{PollID: id, Status: p.Status, Source: "redis_live", AsOf: t.AsOf, Participants: t.Participants, Options: t.Options}, nil
}

func (s *ResultService) Close(ctx context.Context, id string) (domain.Poll, domain.Result, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Poll{}, domain.Result{}, err
	}
	if p.Status == domain.Closed {
		r, err := s.repo.LoadResult(ctx, id)
		return p, r, err
	}
	if p.Status != domain.Active {
		return domain.Poll{}, domain.Result{}, domain.ErrInvalidTransition
	}
	if err := s.store.CloseGates(ctx, p.ID, p.EndsAt); err != nil {
		return domain.Poll{}, domain.Result{}, err
	}
	t, err := s.store.Totals(ctx, p.ID, optionIDs(p))
	if err != nil {
		return domain.Poll{}, domain.Result{}, err
	}
	r, err := s.repo.SaveSnapshot(ctx, p, t, true)
	if err != nil {
		return domain.Poll{}, domain.Result{}, err
	}
	p.Status = domain.Closed
	p.ClosedAt = r.FinalizedAt
	s.cache.Delete(id)
	return p, r, nil
}

func (s *ResultService) Snapshot(ctx context.Context, p domain.Poll) error {
	token, err := domain.NewUUID()
	if err != nil {
		return err
	}
	locked, err := s.store.AcquireSnapshotLock(ctx, p.ID, token, 4*time.Second)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer s.store.ReleaseSnapshotLock(context.Background(), p.ID, token)
	t, err := s.store.Totals(ctx, p.ID, optionIDs(p))
	if err != nil {
		return err
	}
	_, err = s.repo.SaveSnapshot(ctx, p, t, false)
	return err
}
