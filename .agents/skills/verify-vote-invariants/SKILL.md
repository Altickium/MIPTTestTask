---
name: verify-vote-invariants
description: Verify changes to this repository's Redis Lua vote path, key builder, cookie identity, bucket selection, or result aggregation.
---

Read [the Redis keyspace](references/redis-keyspace.md). Identify the affected invariants and inspect the full write/read path.

Verify that all keys used by one Lua invocation share the exact `{pollID:bucket}` hash tag. Verify duplicate delivery, closed or missing gate, multiple-choice participant semantics, TTL, deterministic buckets, and aggregate totals.

Run `scripts/run-vote-verification.sh`. If integration services are unavailable, report tests as skipped rather than successful. Never replace a failed Redis vote with PostgreSQL persistence.
