package redisstore

import (
	"math"
	"testing"
	"time"

	"pollservice/internal/domain"
)

func TestBucketVectors(t *testing.T) {
	tests := []struct {
		hash    string
		buckets int
		want    int
	}{{"device-a", 64, 16}, {"device-b", 64, 29}, {"same", 7, 2}}
	for _, tt := range tests {
		if got := Bucket(tt.hash, tt.buckets); got != tt.want {
			t.Errorf("Bucket(%q,%d)=%d want %d", tt.hash, tt.buckets, got, tt.want)
		}
	}
}

func TestAddCountersOverflow(t *testing.T) {
	totals := domain.Totals{Participants: math.MaxInt64, Options: map[string]int64{"a": 0}}
	if err := addCounters(&totals, map[string]struct{}{"a": {}}, map[string]string{"__participants": "1"}); err == nil {
		t.Fatal("expected participants overflow")
	}
	totals = domain.Totals{Options: map[string]int64{"a": math.MaxInt64}}
	if err := addCounters(&totals, map[string]struct{}{"a": {}}, map[string]string{"a": "1"}); err == nil {
		t.Fatal("expected option overflow")
	}
}
func TestTTLBounds(t *testing.T) {
	s := &Store{retention: time.Hour, maxTTL: 24 * time.Hour}
	now := time.Unix(1000, 0)
	if got := s.TTL(now.Add(-2*time.Hour), now); got != time.Second {
		t.Fatalf("minimum=%v", got)
	}
	if got := s.TTL(now.Add(48*time.Hour), now); got != 24*time.Hour {
		t.Fatalf("maximum=%v", got)
	}
	if got := s.TTL(now.Add(time.Hour), now); got != 2*time.Hour {
		t.Fatalf("normal=%v", got)
	}
}
