package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pollservice/internal/domain"
)

func TestPollCacheCoalescesSameKeyLoads(t *testing.T) {
	cache := NewPollCache(time.Minute)
	var calls atomic.Int64
	loader := func(context.Context, string) (domain.Poll, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return domain.Poll{ID: "poll"}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := cache.Get(context.Background(), "poll", loader)
			if err != nil || p.ID != "poll" {
				t.Errorf("poll=%+v err=%v", p, err)
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls=%d want 1", got)
	}
}

type countingPollLoader struct {
	calls int
	poll  domain.Poll
}

func (f *countingPollLoader) GetByID(context.Context, string) (domain.Poll, error) {
	f.calls++
	return f.poll, nil
}

type acceptingVoteStore struct{}

func (acceptingVoteStore) Record(context.Context, domain.VoteCommand) (domain.VoteStatus, error) {
	return domain.VoteRecorded, nil
}

func TestVoteDoesNotCacheDraft(t *testing.T) {
	repo := &countingPollLoader{poll: domain.Poll{Status: domain.Draft}}
	svc := NewVoteService(repo, acceptingVoteStore{}, NewPollCache(time.Minute), []byte("abcdefghijklmnopqrstuvwxyz012345"))
	for i := 0; i < 2; i++ {
		if _, err := svc.Vote(context.Background(), "poll", "device", nil, time.Now()); err != domain.ErrNotActive {
			t.Fatalf("err=%v", err)
		}
	}
	if repo.calls != 2 {
		t.Fatalf("draft loader calls=%d want 2", repo.calls)
	}
}
