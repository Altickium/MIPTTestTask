# Poll service agent guide

Read `artefacts/IMPLEMENTATION_SPEC_REDIS_GO_POSTGRES.md` and `docs/architecture.md` before architectural changes.

## Commands

- Format: `gofmt -w cmd internal tests`
- Unit tests: `go test ./...`
- Race tests: `go test -race ./...`
- Integration tests: `go test -tags=integration ./tests/integration/...`
- Local stack: `docker compose up --build -d`

## Non-negotiable invariants

- The vote hot path uses Redis only after an in-process poll-definition cache hit; never add a PostgreSQL vote fallback.
- Gate check, device deduplication, participant increment, and option increments stay in one Redis Lua script.
- `participants_count` increments once per accepted device, including multiple-choice votes.
- Active poll definitions are immutable. Never log raw IP addresses, cookie values, device IDs/hashes, authorization values, or vote bodies.
- Changes to Lua, identity hashing, bucket selection, or Redis keys require an integration test for duplicate, closed-gate, multiple-choice, and TTL behavior.
- Prefer the Go standard library. The supported external runtime dependencies are pgx and go-redis.

Do not edit `go.sum` manually. A change is done only after formatting, unit/race tests, relevant integration tests, and documentation updates.
