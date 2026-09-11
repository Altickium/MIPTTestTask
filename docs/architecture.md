# Architecture

The process is stateless with respect to browser sessions. PostgreSQL stores poll definitions and runtime/final snapshots. Redis is the live source of truth while a poll is active.

## Vote path

1. Validate UUID, bounded JSON, poll time/status, unique options, and option ownership. Poll definitions come from a per-ID TTL cache with one loader per cache miss.
2. Verify or mint a signed `HttpOnly`, `SameSite=Lax` device cookie.
3. Derive a poll-scoped device HMAC and choose a bucket from the first eight SHA-256 bytes.
4. Execute one Lua call against gate, dedup marker, and counter hash sharing `{pollID:bucket}`.
5. Return `recorded`, `already_recorded`, a state conflict, or fail closed.

PostgreSQL is not queried on a cache hit. Multiple-choice increments `__participants` once and each selected option once.

## Lifecycle

Publish commits PostgreSQL status, installs every Redis gate, and warms local cache. If gate setup fails, only the transition carrying this request's `published_at` timestamp is compensated to draft.

Close replaces all gates with `closed`. Redis command ordering means earlier Lua calls finish before gate replacement and later calls observe the closed value. Totals are then aggregated and saved together with closed status/finalization in one PostgreSQL transaction. A failed close leaves gates/counters for retry; a completed close returns the stored final result without modifying it.

Each API replica periodically snapshots active polls. A Redis `SET NX PX` lock selects one writer, and a token-checked Lua delete prevents one replica from releasing another replica's lock. Snapshot timestamps prevent an older writer from overwriting a newer snapshot.

## Scaling and limits

Logical buckets avoid one global Redis hash per poll and preserve Redis Cluster multi-key script locality. Reads pipeline all buckets and cost O(bucket count). `VOTE_BUCKETS` is stored with metadata and cannot change while a poll is active.

Cookie identity is basic deduplication, not strong anti-fraud. AOF every-second persistence has a bounded but non-zero loss window. Strong durability or replay would require a durable event log and a materially larger design.

## Web delivery and QR

The Go binary embeds the public poll page, the demo admin panel, CSS, JavaScript, and the OpenAPI contract. The browser calls the existing same-origin JSON endpoints, so there is no Node.js build, separate frontend process, CORS policy, or second application state model. Dynamic poll text is written through DOM text properties rather than HTML injection. The admin bearer token exists only in the tab's JavaScript memory.

The protected recent-polls endpoint reads persisted definitions from PostgreSQL in reverse creation order. The admin panel uses it to restore navigation after a reload; selecting an active poll resumes Redis-backed result polling, while selecting a closed poll reads its PostgreSQL final snapshot through the existing results endpoint. This read path does not alter vote processing or add a PostgreSQL fallback to it.

Public links and QR payloads are derived exclusively from the validated `PUBLIC_BASE_URL`; request host headers are not trusted for link generation. The protected QR endpoint loads the poll by ID, refuses drafts, encodes only the canonical `/p/{slug}` URL, and caches the generated SVG in process memory. QR generation does not touch the vote path or persist new data.

## API contract and observability

`api/openapi.yaml` is the OpenAPI 3.1 contract for client and admin endpoints and is also embedded at `/openapi.yaml`. It documents bearer authentication, device-cookie semantics, lifecycle operations, QR responses, and the common error envelope.

The local Compose stack also serves that file through a pinned Swagger UI container on port 8081. Its Nginx configuration proxies only `/api/` and `/health/` to the API service, allowing Swagger's `Try it out` requests and browser identity cookies to remain same-origin without enabling CORS in the Go application. Swagger is development documentation tooling and is not required by the API at runtime.

The request middleware records bounded route templates, methods, statuses, and latency buckets. Concrete poll IDs are never metric labels. The optional Compose `observability` profile runs a pinned Prometheus image, scrapes `/metrics` every five seconds, and stores a seven-day local TSDB in a named volume. It is intended for local verification; production must keep both the metrics endpoint and Prometheus UI on a protected internal network.
