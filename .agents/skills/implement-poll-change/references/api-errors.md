# API error mapping

All errors use `{"error":{"code":"...","message":"...","request_id":"..."}}`.

| Status | Code | Meaning |
|---:|---|---|
| 400 | `invalid_request` | Invalid JSON, identifier, or basic input |
| 401 | `unauthorized` | Missing or invalid admin token |
| 404 | `poll_not_found` | Missing or intentionally hidden poll |
| 409 | `poll_not_active` / `invalid_transition` | State conflict |
| 422 | `invalid_selection` | Selection violates poll rules |
| 429 | `rate_limited` | Network rate limit exceeded |
| 503 | `temporarily_unavailable` | Redis/PostgreSQL dependency failure |

Never expose storage errors or stack traces.
