package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var durationBuckets = [...]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type requestKey struct{ route, method, status string }
type durationHistogram struct {
	buckets [len(durationBuckets)]uint64
	count   uint64
	sum     float64
}

type Metrics struct {
	mu                   sync.Mutex
	requests             map[requestKey]uint64
	durations            map[string]*durationHistogram
	rejections           map[string]uint64
	recorded             atomic.Uint64
	duplicates           atomic.Uint64
	snapshots            atomic.Uint64
	snapshotErrors       atomic.Uint64
	lastSnapshotUnixNano atomic.Int64
}

func (m *Metrics) Reject(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rejections == nil {
		m.rejections = make(map[string]uint64)
	}
	m.rejections[reason]++
}

func (m *Metrics) ObserveHTTP(route, method string, status int, duration time.Duration) {
	seconds := duration.Seconds()
	key := requestKey{route: route, method: method, status: strconv.Itoa(status)}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.requests == nil {
		m.requests = make(map[requestKey]uint64)
	}
	if m.durations == nil {
		m.durations = make(map[string]*durationHistogram)
	}
	m.requests[key]++
	h := m.durations[route]
	if h == nil {
		h = &durationHistogram{}
		m.durations[route] = h
	}
	h.count++
	h.sum += seconds
	for i, upper := range durationBuckets {
		if seconds <= upper {
			h.buckets[i]++
		}
	}
}

func (m *Metrics) SnapshotSuccess() {
	m.snapshots.Add(1)
	m.lastSnapshotUnixNano.Store(time.Now().UnixNano())
}
func (m *Metrics) SnapshotError() { m.snapshotErrors.Add(1) }

func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	m.mu.Lock()
	defer m.mu.Unlock()
	fmt.Fprintln(w, "# HELP http_requests_total Completed HTTP requests by normalized route, method, and status.")
	fmt.Fprintln(w, "# TYPE http_requests_total counter")
	requestKeys := make([]requestKey, 0, len(m.requests))
	for key := range m.requests {
		requestKeys = append(requestKeys, key)
	}
	sort.Slice(requestKeys, func(i, j int) bool {
		a, b := requestKeys[i], requestKeys[j]
		if a.route != b.route {
			return a.route < b.route
		}
		if a.method != b.method {
			return a.method < b.method
		}
		return a.status < b.status
	})
	for _, key := range requestKeys {
		fmt.Fprintf(w, "http_requests_total{route=%q,method=%q,status=%q} %d\n", promLabel(key.route), promLabel(key.method), promLabel(key.status), m.requests[key])
	}
	fmt.Fprintln(w, "# HELP http_request_duration_seconds HTTP request duration by normalized route.")
	fmt.Fprintln(w, "# TYPE http_request_duration_seconds histogram")
	routes := make([]string, 0, len(m.durations))
	for route := range m.durations {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		h := m.durations[route]
		for i, upper := range durationBuckets {
			fmt.Fprintf(w, "http_request_duration_seconds_bucket{route=%q,le=%q} %d\n", promLabel(route), strconv.FormatFloat(upper, 'g', -1, 64), h.buckets[i])
		}
		fmt.Fprintf(w, "http_request_duration_seconds_bucket{route=%q,le=\"+Inf\"} %d\n", promLabel(route), h.count)
		fmt.Fprintf(w, "http_request_duration_seconds_sum{route=%q} %s\n", promLabel(route), strconv.FormatFloat(h.sum, 'g', -1, 64))
		fmt.Fprintf(w, "http_request_duration_seconds_count{route=%q} %d\n", promLabel(route), h.count)
	}
	writeCounter := func(name, help string, value uint64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, value)
	}
	writeCounter("votes_recorded_total", "Votes recorded for new browser identities.", m.recorded.Load())
	writeCounter("votes_duplicate_total", "Idempotent duplicate vote requests.", m.duplicates.Load())
	fmt.Fprintln(w, "# HELP votes_rejected_total Vote requests rejected by a bounded reason.")
	fmt.Fprintln(w, "# TYPE votes_rejected_total counter")
	reasons := make([]string, 0, len(m.rejections))
	for reason := range m.rejections {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		fmt.Fprintf(w, "votes_rejected_total{reason=%q} %d\n", promLabel(reason), m.rejections[reason])
	}
	writeCounter("snapshot_success_total", "Successful snapshot worker passes.", m.snapshots.Load())
	writeCounter("snapshot_error_total", "Failed snapshot worker operations.", m.snapshotErrors.Load())
	age := 0.0
	if last := m.lastSnapshotUnixNano.Load(); last > 0 {
		age = time.Since(time.Unix(0, last)).Seconds()
		if age < 0 {
			age = 0
		}
	}
	fmt.Fprintln(w, "# HELP snapshot_age_seconds Seconds since the latest successful snapshot pass.")
	fmt.Fprintln(w, "# TYPE snapshot_age_seconds gauge")
	fmt.Fprintf(w, "snapshot_age_seconds %s\n", strconv.FormatFloat(age, 'g', -1, 64))
}

func promLabel(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"").Replace(value)
}
