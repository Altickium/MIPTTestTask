# Load scenarios

- `unique`: `DUPLICATE_RATIO=0`; measures new accepted votes. For true uniqueness, ensure k6 iterations have independent cookie jars or use pre-issued identities.
- `duplicate`: `DUPLICATE_RATIO=1`; validates idempotent retry overhead within each VU cookie jar.
- `burst`: start with a short duration and raise VUs gradually; stop if the error threshold is exceeded.

Record `VOTE_BUCKETS`, API replica count, Redis persistence settings, PostgreSQL version, and whether the run shares the host with dependencies. A local result is not a production capacity claim.
