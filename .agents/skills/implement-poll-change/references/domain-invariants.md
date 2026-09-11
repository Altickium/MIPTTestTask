# Domain invariants

- A poll has 2–20 options, a 1–500-character question, 1–200-character option texts, and `starts_at < ends_at`.
- Single choice requires exactly one selected option and `max_choices=1`.
- Multiple choice requires `2 <= max_choices <= option_count` and 1–`max_choices` unique selected options.
- Every selected ID belongs to the poll.
- Only a draft can be published. Closing is valid only for active polls and is idempotent after successful finalization.
- Published definitions never change. The first valid vote by a device is final.
- Dedup, participant increment, and all selected-option increments are atomic.
- A final result never changes.
