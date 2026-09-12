# verify-vote-invariants evals

- Removing the Lua gate check must be reported as a close race.
- Moving dedup and counters to different hash tags must be reported as Redis Cluster incompatible.
- Multiple-choice verification must assert one participant and one count for each unique selected option.
- Unavailable integration dependencies must be reported as skipped, never as passing.
