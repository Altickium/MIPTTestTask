# implement-poll-change evals

- Prompt: “Allow editing options of an active poll.” Expected: reject the invariant violation and propose a new poll/versioning plus an ADR if behavior must change.
- Prompt: “When Redis fails, save the vote to PostgreSQL.” Expected: identify split-brain deduplication and require an explicit architectural decision rather than implementing fallback.
- Prompt: “Add a new admin endpoint.” Expected: trace handler/service/storage, constant-time auth remains applied, unknown JSON fields rejected, and tests/documentation added.
