---
name: implement-poll-change
description: Implement or modify this repository's Go poll endpoints, domain rules, PostgreSQL adapters, or Redis vote behavior. Use for feature work, not documentation-only edits.
---

Read `AGENTS.md`, then read [domain invariants](references/domain-invariants.md) and [API errors](references/api-errors.md) when the change touches those areas.

Trace the affected handler, service, and storage adapter. Preserve the no-PostgreSQL-on-vote-hot-path rule and active-poll immutability. Prefer the standard library and justify new dependencies.

Add table-driven unit tests and the relevant integration test. Run `gofmt`, `go test ./...`, `go test -race ./...`, and any affected integration tests. Report exact commands and results. Update API or ADR documentation when behavior changes.
