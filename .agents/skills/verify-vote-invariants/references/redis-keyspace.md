# Redis keyspace

For bucket `b`:

- `poll-gate:{<pollID>:<b>}`: lifecycle gate.
- `poll-dedup:{<pollID>:<b>}:<deviceHash>`: one-device marker.
- `poll-counts:{<pollID>:<b>}`: `__participants` and option UUID counters.

The device hash is a poll-scoped HMAC, never the cookie ID. TTL is bounded server-side and extends through retention. `VOTE_BUCKETS` cannot change while any poll is active.
