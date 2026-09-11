package redisstore

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"pollservice/internal/domain"
)

//go:embed scripts/*.lua
var scripts embed.FS

type Store struct {
	client       *redis.Client
	buckets      int
	retention    time.Duration
	maxTTL       time.Duration
	voteScript   *redis.Script
	rateScript   *redis.Script
	unlockScript *redis.Script
}

func New(client *redis.Client, buckets int, retention, maxTTL time.Duration) *Store {
	voteLua, _ := scripts.ReadFile("scripts/vote.lua")
	rateLua := `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],ARGV[2]) end; if n>tonumber(ARGV[1]) then return 0 end; return 1`
	unlockLua := `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end; return 0`
	return &Store{client: client, buckets: buckets, retention: retention, maxTTL: maxTTL, voteScript: redis.NewScript(string(voteLua)), rateScript: redis.NewScript(rateLua), unlockScript: redis.NewScript(unlockLua)}
}

func Bucket(deviceHash string, buckets int) int {
	sum := sha256.Sum256([]byte(deviceHash))
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(buckets))
}

func keyTag(pollID string, bucket int) string  { return "{" + pollID + ":" + strconv.Itoa(bucket) + "}" }
func GateKey(pollID string, bucket int) string { return "poll-gate:" + keyTag(pollID, bucket) }
func DedupKey(pollID string, bucket int, deviceHash string) string {
	return "poll-dedup:" + keyTag(pollID, bucket) + ":" + deviceHash
}
func CountsKey(pollID string, bucket int) string { return "poll-counts:" + keyTag(pollID, bucket) }

func (s *Store) TTL(endsAt, now time.Time) time.Duration {
	ttl := endsAt.Sub(now) + s.retention
	if ttl < time.Second {
		ttl = time.Second
	}
	if ttl > s.maxTTL {
		ttl = s.maxTTL
	}
	return ttl
}

func (s *Store) SetupGates(ctx context.Context, pollID string, endsAt time.Time) error {
	ttl := s.TTL(endsAt, time.Now())
	pipe := s.client.Pipeline()
	for b := 0; b < s.buckets; b++ {
		pipe.Set(ctx, GateKey(pollID, b), "active", ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) CloseGates(ctx context.Context, pollID string, endsAt time.Time) error {
	ttl := s.TTL(endsAt, time.Now())
	pipe := s.client.Pipeline()
	for b := 0; b < s.buckets; b++ {
		pipe.Set(ctx, GateKey(pollID, b), "closed", ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) DeleteGates(ctx context.Context, pollID string) error {
	keys := make([]string, 0, s.buckets)
	for b := 0; b < s.buckets; b++ {
		keys = append(keys, GateKey(pollID, b))
	}
	return s.client.Del(ctx, keys...).Err()
}

func (s *Store) Record(ctx context.Context, cmd domain.VoteCommand) (domain.VoteStatus, error) {
	b := Bucket(cmd.DeviceHash, s.buckets)
	ttlSeconds := int64(math.Ceil(s.TTL(cmd.EndsAt, time.Now()).Seconds()))
	args := make([]any, 0, len(cmd.OptionIDs)+1)
	args = append(args, ttlSeconds)
	for _, id := range cmd.OptionIDs {
		args = append(args, id)
	}
	n, err := s.voteScript.Run(ctx, s.client, []string{GateKey(cmd.PollID, b), DedupKey(cmd.PollID, b, cmd.DeviceHash), CountsKey(cmd.PollID, b)}, args...).Int()
	if err != nil {
		return "", err
	}
	switch n {
	case 1:
		return domain.VoteRecorded, nil
	case 0:
		return domain.VoteAlreadyRecorded, nil
	case -1:
		return "", domain.ErrNotActive
	default:
		return "", fmt.Errorf("unexpected vote script result %d", n)
	}
}

func (s *Store) Totals(ctx context.Context, pollID string, optionIDs []string) (domain.Totals, error) {
	pipe := s.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, s.buckets)
	for b := 0; b < s.buckets; b++ {
		cmds[b] = pipe.HGetAll(ctx, CountsKey(pollID, b))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return domain.Totals{}, err
	}
	t := domain.Totals{Options: make(map[string]int64, len(optionIDs)), AsOf: time.Now().UTC()}
	valid := make(map[string]struct{}, len(optionIDs))
	for _, id := range optionIDs {
		valid[id] = struct{}{}
		t.Options[id] = 0
	}
	for _, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil {
			return domain.Totals{}, err
		}
		if err := addCounters(&t, valid, values); err != nil {
			return domain.Totals{}, err
		}
	}
	return t, nil
}

func addCounters(t *domain.Totals, valid map[string]struct{}, values map[string]string) error {
	for field, raw := range values {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid counter %s", field)
		}
		if field == "__participants" {
			if math.MaxInt64-t.Participants < n {
				return errors.New("participants counter overflow")
			}
			t.Participants += n
			continue
		}
		if _, ok := valid[field]; !ok {
			slog.Warn("ignoring unknown Redis counter field", "field", field)
			continue
		}
		if math.MaxInt64-t.Options[field] < n {
			return errors.New("option counter overflow")
		}
		t.Options[field] += n
	}
	return nil
}

func (s *Store) AllowRate(ctx context.Context, networkHash string, limit int64) (bool, error) {
	window := time.Now().UTC().Unix() / 60
	key := "vote-rate:" + networkHash + ":" + strconv.FormatInt(window, 10)
	n, err := s.rateScript.Run(ctx, s.client, []string{key}, limit, 65).Int()
	return n == 1, err
}

func (s *Store) Ping(ctx context.Context) error { return s.client.Ping(ctx).Err() }

func (s *Store) AcquireSnapshotLock(ctx context.Context, pollID, token string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, "snapshot-lock:"+pollID, token, ttl).Result()
}
func (s *Store) ReleaseSnapshotLock(ctx context.Context, pollID, token string) error {
	return s.unlockScript.Run(ctx, s.client, []string{"snapshot-lock:" + pollID}, token).Err()
}
