# ADR 0001: Redis-first bucketed vote path

## Context

Audience voting creates a short write burst. Each valid browser must count once, including atomic updates to several choices, while PostgreSQL remains the durable metadata/result store.

## Decision

Use one Redis Lua invocation for gate validation, poll-scoped device deduplication, participant count, and option counters. Partition each poll into a configurable fixed number of `{pollID:bucket}` hash-tagged buckets. Fail closed when Redis is unavailable and never fall back to a separate PostgreSQL vote path.

Store `vote_buckets` in PostgreSQL. Drafts adopt the current value on publish, and the API refuses startup if an active poll uses another value. PostgreSQL receives periodic and final aggregate snapshots, not raw votes.

## Alternatives

- PostgreSQL-only is simpler and more durable but places unique-index and WAL work on the burst path.
- One Redis counter hash is simpler but remains a hot key and cannot spread across cluster slots.
- Kafka or another durable log supports replay but adds producer/consumer idempotency and infrastructure beyond this task.

## Consequences

The write path is short, retry-safe, and horizontally compatible. Result reads cost one pipelined hash read per bucket. Redis becomes critical live state; AOF every-second persistence still has a possible loss window. Changing bucket count while active is prohibited.

## Validation

Unit tests cover selection, identity, bucket vectors, and TTL boundaries. The Redis integration suite sends 200 concurrent calls with one identity and asserts one participant, tests multiple choice, and rejects a closed gate. Race tests cover in-process state.
