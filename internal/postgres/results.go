package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"pollservice/internal/domain"
)

func (r *PollRepository) SaveSnapshot(ctx context.Context, poll domain.Poll, totals domain.Totals, finalized bool) (domain.Result, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Result{}, err
	}
	defer tx.Rollback()
	var status domain.PollStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM polls WHERE id=$1 FOR UPDATE`, poll.ID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return domain.Result{}, domain.ErrNotFound
	} else if err != nil {
		return domain.Result{}, err
	}
	var existingFinal *time.Time
	err = tx.QueryRowContext(ctx, `SELECT finalized_at FROM poll_runtime_results WHERE poll_id=$1`, poll.ID).Scan(&existingFinal)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Result{}, err
	}
	if existingFinal != nil {
		if err := tx.Commit(); err != nil {
			return domain.Result{}, err
		}
		return r.LoadResult(ctx, poll.ID)
	}
	var finalAt *time.Time
	if finalized {
		t := totals.AsOf.UTC()
		finalAt = &t
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO poll_runtime_results(poll_id,participants_count,captured_at,finalized_at) VALUES($1,$2,$3,$4) ON CONFLICT(poll_id) DO UPDATE SET participants_count=EXCLUDED.participants_count,captured_at=EXCLUDED.captured_at,finalized_at=EXCLUDED.finalized_at WHERE poll_runtime_results.finalized_at IS NULL AND poll_runtime_results.captured_at <= EXCLUDED.captured_at`, poll.ID, totals.Participants, totals.AsOf.UTC(), finalAt)
	if err != nil {
		return domain.Result{}, err
	}
	for _, option := range poll.Options {
		_, err := tx.ExecContext(ctx, `INSERT INTO poll_option_results(poll_id,option_id,vote_count,captured_at) VALUES($1,$2,$3,$4) ON CONFLICT(poll_id,option_id) DO UPDATE SET vote_count=EXCLUDED.vote_count,captured_at=EXCLUDED.captured_at WHERE poll_option_results.captured_at <= EXCLUDED.captured_at`, poll.ID, option.ID, totals.Options[option.ID], totals.AsOf.UTC())
		if err != nil {
			return domain.Result{}, err
		}
	}
	if finalized {
		if status != domain.Active && status != domain.Closed {
			return domain.Result{}, domain.ErrInvalidTransition
		}
		if _, err := tx.ExecContext(ctx, `UPDATE polls SET status='closed',closed_at=COALESCE(closed_at,$2),updated_at=$2 WHERE id=$1`, poll.ID, totals.AsOf.UTC()); err != nil {
			return domain.Result{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Result{}, err
	}
	return r.LoadResult(ctx, poll.ID)
}

func (r *PollRepository) LoadResult(ctx context.Context, pollID string) (domain.Result, error) {
	var result domain.Result
	err := r.db.QueryRowContext(ctx, `SELECT p.id,p.status,r.participants_count,r.captured_at,r.finalized_at FROM polls p JOIN poll_runtime_results r ON r.poll_id=p.id WHERE p.id=$1`, pollID).Scan(&result.PollID, &result.Status, &result.Participants, &result.AsOf, &result.FinalizedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Result{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Result{}, err
	}
	result.Source = "postgres_snapshot"
	if result.FinalizedAt != nil {
		result.Source = "postgres_final"
	}
	result.Options = make(map[string]int64)
	rows, err := r.db.QueryContext(ctx, `SELECT option_id,vote_count FROM poll_option_results WHERE poll_id=$1`, pollID)
	if err != nil {
		return domain.Result{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int64
		if err := rows.Scan(&id, &count); err != nil {
			return domain.Result{}, err
		}
		result.Options[id] = count
	}
	return result, rows.Err()
}
