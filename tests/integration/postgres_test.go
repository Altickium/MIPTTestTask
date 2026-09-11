//go:build integration

package integration

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"pollservice/internal/domain"
	pgstore "pollservice/internal/postgres"
)

func postgresRepo(t *testing.T) (*pgstore.PollRepository, *sql.DB) {
	t.Helper()
	url := os.Getenv("INTEGRATION_DATABASE_URL")
	if url == "" {
		url = "postgres://poll:poll@localhost:5432/poll?sslmode=disable"
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Skipf("postgres not available: %v", err)
	}
	if err := pgstore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return pgstore.NewPollRepository(db, 8), db
}

func TestConcurrentPublishAndFinalization(t *testing.T) {
	repo, db := postgresRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	p, err := repo.Create(ctx, domain.CreatePoll{Question: "integration?", Type: domain.SingleChoice, MaxChoices: 1, StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour), Options: []string{"A", "B"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM polls WHERE id=$1`, p.ID) })
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := repo.Publish(ctx, p.ID, time.Now().UTC()); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.Active {
		t.Fatalf("status=%s", loaded.Status)
	}
	totals := domain.Totals{Participants: 3, Options: map[string]int64{p.Options[0].ID: 2, p.Options[1].ID: 1}, AsOf: time.Now().UTC()}
	first, err := repo.SaveSnapshot(ctx, loaded, totals, true)
	if err != nil {
		t.Fatal(err)
	}
	totals.Participants = 99
	totals.AsOf = totals.AsOf.Add(time.Second)
	second, err := repo.SaveSnapshot(ctx, loaded, totals, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.Participants != 3 || second.Participants != 3 {
		t.Fatalf("finalization changed: first=%d second=%d", first.Participants, second.Participants)
	}
}

func TestListRecentPolls(t *testing.T) {
	repo, db := postgresRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	first, err := repo.Create(ctx, domain.CreatePoll{Question: "older integration poll", Type: domain.SingleChoice, MaxChoices: 1, StartsAt: now, EndsAt: now.Add(time.Hour), Options: []string{"A", "B"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Create(ctx, domain.CreatePoll{Question: "newer integration poll", Type: domain.MultipleChoice, MaxChoices: 2, StartsAt: now, EndsAt: now.Add(time.Hour), Options: []string{"A", "B", "C"}})
	if err != nil {
		db.ExecContext(ctx, `DELETE FROM polls WHERE id=$1`, first.ID)
		t.Fatal(err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM polls WHERE id IN ($1,$2)`, first.ID, second.ID) })

	polls, err := repo.ListRecent(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	firstIndex, secondIndex := -1, -1
	for i, poll := range polls {
		switch poll.ID {
		case first.ID:
			firstIndex = i
			if len(poll.Options) != 2 {
				t.Fatalf("older poll options=%d", len(poll.Options))
			}
		case second.ID:
			secondIndex = i
			if len(poll.Options) != 3 {
				t.Fatalf("newer poll options=%d", len(poll.Options))
			}
		}
	}
	if firstIndex < 0 || secondIndex < 0 {
		t.Fatalf("created polls not found: older=%d newer=%d", firstIndex, secondIndex)
	}
	if secondIndex >= firstIndex {
		t.Fatalf("recent order is wrong: older=%d newer=%d", firstIndex, secondIndex)
	}
}
