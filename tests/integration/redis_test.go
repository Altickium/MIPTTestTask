//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"pollservice/internal/domain"
	"pollservice/internal/redisstore"
)

func redisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("INTEGRATION_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6380"
	}
	c := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not available at %s: %v", addr, err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestConcurrentDuplicateIsCountedOnce(t *testing.T) {
	c := redisClient(t)
	store := redisstore.New(c, 8, time.Hour, 24*time.Hour)
	pollID, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	optionID, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.SetupGates(ctx, pollID, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var cursor uint64
		for {
			keys, next, _ := c.Scan(ctx, cursor, "*"+pollID+"*", 100).Result()
			if len(keys) > 0 {
				c.Del(ctx, keys...)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	})
	cmd := domain.VoteCommand{PollID: pollID, DeviceHash: "fixed-device-hash", OptionIDs: []string{optionID}, EndsAt: time.Now().Add(time.Minute)}
	var recorded atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, err := store.Record(ctx, cmd)
			if err != nil {
				errs <- err
				return
			}
			if status == domain.VoteRecorded {
				recorded.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := recorded.Load(); got != 1 {
		t.Fatalf("recorded=%d want 1", got)
	}
	totals, err := store.Totals(ctx, pollID, []string{optionID})
	if err != nil {
		t.Fatal(err)
	}
	if totals.Participants != 1 || totals.Options[optionID] != 1 {
		t.Fatalf("totals=%+v", totals)
	}
	bucket := redisstore.Bucket(cmd.DeviceHash, 8)
	for _, key := range []string{redisstore.DedupKey(pollID, bucket, cmd.DeviceHash), redisstore.CountsKey(pollID, bucket)} {
		ttl, err := c.TTL(ctx, key).Result()
		if err != nil || ttl <= 0 {
			t.Fatalf("key %s ttl=%v err=%v", key, ttl, err)
		}
	}
}

func TestMultipleChoiceAndClosedGate(t *testing.T) {
	c := redisClient(t)
	store := redisstore.New(c, 4, time.Hour, 24*time.Hour)
	pollID, _ := domain.NewUUID()
	a, _ := domain.NewUUID()
	b, _ := domain.NewUUID()
	ctx := context.Background()
	if err := store.SetupGates(ctx, pollID, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	cmd := domain.VoteCommand{PollID: pollID, DeviceHash: "device", OptionIDs: []string{a, b}, EndsAt: time.Now().Add(time.Minute)}
	if status, err := store.Record(ctx, cmd); err != nil || status != domain.VoteRecorded {
		t.Fatalf("status=%s err=%v", status, err)
	}
	if err := store.CloseGates(ctx, pollID, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	cmd.DeviceHash = "another"
	if _, err := store.Record(ctx, cmd); err != domain.ErrNotActive {
		t.Fatalf("closed vote err=%v", err)
	}
	totals, err := store.Totals(ctx, pollID, []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if totals.Participants != 1 || totals.Options[a] != 1 || totals.Options[b] != 1 {
		t.Fatalf("totals=%s", fmt.Sprint(totals))
	}
}
