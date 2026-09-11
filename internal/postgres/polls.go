package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"pollservice/internal/domain"
)

type PollRepository struct {
	db      *sql.DB
	buckets int
}

func NewPollRepository(db *sql.DB, buckets int) *PollRepository {
	return &PollRepository{db: db, buckets: buckets}
}

func (r *PollRepository) Create(ctx context.Context, in domain.CreatePoll) (domain.Poll, error) {
	if err := in.Validate(); err != nil {
		return domain.Poll{}, err
	}
	id, err := domain.NewUUID()
	if err != nil {
		return domain.Poll{}, err
	}
	suffix := strings.ReplaceAll(id, "-", "")[:8]
	slug := "poll-" + suffix
	now := time.Now().UTC()
	p := domain.Poll{ID: id, Slug: slug, Question: strings.TrimSpace(in.Question), Type: in.Type, MaxChoices: in.MaxChoices, Status: domain.Draft, StartsAt: in.StartsAt.UTC(), EndsAt: in.EndsAt.UTC(), CreatedAt: now, UpdatedAt: now}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Poll{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO polls(id,slug,question,poll_type,max_choices,vote_buckets,status,starts_at,ends_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)`, p.ID, p.Slug, p.Question, p.Type, p.MaxChoices, r.buckets, p.Status, p.StartsAt, p.EndsAt, now)
	if err != nil {
		return domain.Poll{}, err
	}
	for i, text := range in.Options {
		optionID, err := domain.NewUUID()
		if err != nil {
			return domain.Poll{}, err
		}
		option := domain.Option{ID: optionID, Text: strings.TrimSpace(text), Position: i}
		if _, err := tx.ExecContext(ctx, `INSERT INTO poll_options(id,poll_id,text,position) VALUES($1,$2,$3,$4)`, option.ID, p.ID, option.Text, option.Position); err != nil {
			return domain.Poll{}, err
		}
		p.Options = append(p.Options, option)
	}
	if err := tx.Commit(); err != nil {
		return domain.Poll{}, err
	}
	return p, nil
}

func (r *PollRepository) GetByID(ctx context.Context, id string) (domain.Poll, error) {
	return r.get(ctx, `p.id=$1`, id)
}
func (r *PollRepository) GetBySlug(ctx context.Context, slug string) (domain.Poll, error) {
	return r.get(ctx, `p.slug=$1`, slug)
}

func (r *PollRepository) ListRecent(ctx context.Context, limit int) ([]domain.Poll, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id,p.slug,p.question,p.poll_type,p.max_choices,p.status,p.starts_at,p.ends_at,
		       p.created_at,p.updated_at,p.published_at,p.closed_at,o.id,o.text,o.position
		FROM (
			SELECT id,slug,question,poll_type,max_choices,status,starts_at,ends_at,
			       created_at,updated_at,published_at,closed_at
			FROM polls
			ORDER BY created_at DESC,id DESC
			LIMIT $1
		) p
		JOIN poll_options o ON o.poll_id=p.id
		ORDER BY p.created_at DESC,p.id DESC,o.position`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	polls := make([]domain.Poll, 0, limit)
	for rows.Next() {
		var p domain.Poll
		var option domain.Option
		if err := rows.Scan(&p.ID, &p.Slug, &p.Question, &p.Type, &p.MaxChoices, &p.Status, &p.StartsAt, &p.EndsAt, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &p.ClosedAt, &option.ID, &option.Text, &option.Position); err != nil {
			return nil, err
		}
		if len(polls) == 0 || polls[len(polls)-1].ID != p.ID {
			p.Options = make([]domain.Option, 0, 2)
			polls = append(polls, p)
		}
		polls[len(polls)-1].Options = append(polls[len(polls)-1].Options, option)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return polls, nil
}

func (r *PollRepository) get(ctx context.Context, predicate, value string) (domain.Poll, error) {
	query := `SELECT p.id,p.slug,p.question,p.poll_type,p.max_choices,p.status,p.starts_at,p.ends_at,p.created_at,p.updated_at,p.published_at,p.closed_at,o.id,o.text,o.position FROM polls p JOIN poll_options o ON o.poll_id=p.id WHERE ` + predicate + ` ORDER BY o.position`
	rows, err := r.db.QueryContext(ctx, query, value)
	if err != nil {
		return domain.Poll{}, err
	}
	defer rows.Close()
	var p domain.Poll
	for rows.Next() {
		var option domain.Option
		if err := rows.Scan(&p.ID, &p.Slug, &p.Question, &p.Type, &p.MaxChoices, &p.Status, &p.StartsAt, &p.EndsAt, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &p.ClosedAt, &option.ID, &option.Text, &option.Position); err != nil {
			return domain.Poll{}, err
		}
		p.Options = append(p.Options, option)
	}
	if err := rows.Err(); err != nil {
		return domain.Poll{}, err
	}
	if p.ID == "" {
		return domain.Poll{}, domain.ErrNotFound
	}
	return p, nil
}

func (r *PollRepository) Publish(ctx context.Context, id string, now time.Time) (domain.Poll, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Poll{}, err
	}
	defer tx.Rollback()
	var status domain.PollStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM polls WHERE id=$1 FOR UPDATE`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return domain.Poll{}, domain.ErrNotFound
	} else if err != nil {
		return domain.Poll{}, err
	}
	if status == domain.Active {
		if err := tx.Commit(); err != nil {
			return domain.Poll{}, err
		}
		return r.GetByID(ctx, id)
	}
	if status != domain.Draft {
		return domain.Poll{}, domain.ErrInvalidTransition
	}
	if _, err := tx.ExecContext(ctx, `UPDATE polls SET status='active',published_at=$2,updated_at=$2,vote_buckets=$3 WHERE id=$1`, id, now.UTC(), r.buckets); err != nil {
		return domain.Poll{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Poll{}, err
	}
	return r.GetByID(ctx, id)
}

func (r *PollRepository) ValidateActiveBuckets(ctx context.Context) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM polls WHERE status='active' AND vote_buckets<>$1`, r.buckets).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%d active polls use a different VOTE_BUCKETS value", count)
	}
	return nil
}

func (r *PollRepository) RevertPublish(ctx context.Context, id string, publishedAt time.Time) error {
	res, err := r.db.ExecContext(ctx, `UPDATE polls SET status='draft',published_at=NULL,updated_at=now() WHERE id=$1 AND status='active' AND published_at=$2`, id, publishedAt.UTC())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: publish compensation", domain.ErrInvalidTransition)
	}
	return nil
}

func (r *PollRepository) ListActive(ctx context.Context) ([]domain.Poll, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM polls WHERE status='active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	polls := make([]domain.Poll, 0, len(ids))
	for _, id := range ids {
		p, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		polls = append(polls, p)
	}
	return polls, nil
}
