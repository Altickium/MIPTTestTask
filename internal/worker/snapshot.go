package worker

import (
	"context"
	"log/slog"
	"time"

	"pollservice/internal/domain"
)

type ActivePolls interface {
	ListActive(context.Context) ([]domain.Poll, error)
}
type Snapshotter interface {
	Snapshot(context.Context, domain.Poll) error
}
type Observer interface {
	SnapshotSuccess()
	SnapshotError()
}
type Snapshot struct {
	repo     ActivePolls
	service  Snapshotter
	interval time.Duration
	log      *slog.Logger
	observer Observer
}

func NewSnapshot(repo ActivePolls, service Snapshotter, interval time.Duration, log *slog.Logger, observer Observer) *Snapshot {
	return &Snapshot{repo: repo, service: service, interval: interval, log: log, observer: observer}
}
func (w *Snapshot) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.run(ctx)
		}
	}
}
func (w *Snapshot) run(ctx context.Context) {
	polls, err := w.repo.ListActive(ctx)
	if err != nil {
		w.observer.SnapshotError()
		w.log.Error("snapshot list failed", "error", err)
		return
	}
	for _, p := range polls {
		if err := w.service.Snapshot(ctx, p); err != nil {
			w.observer.SnapshotError()
			w.log.Error("snapshot failed", "poll_id", p.ID, "error", err)
		} else {
			w.observer.SnapshotSuccess()
			w.log.Debug("snapshot saved", "poll_id", p.ID)
		}
	}
}
