# Реализационная спецификация: сервис опросов на Go, Redis и PostgreSQL

Статус документа: готово к передаче реализующему агенту.

Этот документ описывает целевую реализацию тестового задания. Если при реализации обнаруживается неоднозначность, агент должен сначала сохранить инварианты из раздела 4, затем выбрать наиболее простое решение и зафиксировать отклонение в `docs/decisions/`.

## 1. Цель и критерий готовности

Нужно реализовать backend, который позволяет:

- администратору создать, опубликовать и закрыть опрос;
- анонимному зрителю получить активный опрос и проголосовать;
- не учитывать повторный голос того же браузера в одном опросе;
- администратору получить live- и финальные обезличенные результаты;
- запустить все локально одной командой в Linux/Docker Compose;
- воспроизводимо проверить функциональность и базовую производительность.

Решение считается готовым, когда:

```bash
docker compose up --build -d
go test -race ./...
go test -tags=integration ./tests/integration/...
k6 run tests/load/vote.js
```

выполняются по инструкции из README, а один и тот же `device_id` при параллельных запросах увеличивает результат ровно один раз.

## 2. Scope

### Обязательно

- типы опросов `single_choice` и `multiple_choice`;
- минимум два варианта ответа;
- статусы `draft`, `active`, `closed`;
- один неизменяемый голос на браузер и опрос;
- Redis-first прием голосов;
- PostgreSQL для метаданных и сохраненных результатов;
- admin bearer token;
- JSON API;
- health checks, structured logs и Prometheus-compatible `/metrics` либо минимальные внутренние метрики;
- Dockerfile, Compose, миграции, тесты, README;
- сохраненные AI-артефакты.

### Опционально

- минимальная HTML-страница голосования;
- QR-код, ведущий на публичный URL опроса;
- периодическая запись live-снимков в PostgreSQL;
- Prometheus/Grafana profile в Compose.

### Не делать в рамках двух дней

- регистрацию зрителей;
- полноценный RBAC и refresh tokens;
- Kafka и отдельный event pipeline;
- Kubernetes/Terraform;
- ML-антифрод;
- изменение вариантов уже активного опроса;
- сохранение сырого IP, User-Agent или cookie в PostgreSQL;
- обещания о миллионах RPS без отдельного стенда и измерений.

## 3. Архитектура

```text
                            +----------------------+
                            |     PostgreSQL       |
                            | polls/options/results|
                            +----------+-----------+
                                       ^
                                       | admin + snapshots
                                       |
client -> LB/TLS -> Go API replicas ---+
                       |
                       | atomic vote path
                       v
                    Redis
             gates / dedup / counters
```

Основные правила:

1. Go API stateless относительно пользовательских сессий.
2. Конфигурация опубликованного опроса неизменяема.
3. В горячем пути голосования нет синхронной записи в PostgreSQL.
4. Redis является оперативным источником live-результатов.
5. PostgreSQL является долговременным источником метаданных и финального результата.
6. При недоступности Redis голос не принимается: API отвечает `503`.
7. Несколько API-реплик должны безопасно работать одновременно.

## 4. Доменные инварианты

- `starts_at < ends_at`.
- Опрос содержит от 2 до 20 вариантов.
- Текст вопроса: 1–500 символов; текст варианта: 1–200 символов.
- Для `single_choice` `max_choices = 1` и передается ровно один option ID.
- Для `multiple_choice` `2 <= max_choices <= option_count`, а клиент передает от 1 до `max_choices` уникальных option IDs.
- Все выбранные option IDs принадлежат опросу.
- Публиковать можно только `draft`.
- Закрывать можно только `active`; повторное закрытие обрабатывается идемпотентно.
- После публикации нельзя менять вопрос, тип и варианты ответа.
- Первый валидный голос устройства окончателен; редактирование голоса не поддерживается.
- Повторная доставка того же запроса не увеличивает счетчики.
- Для multiple choice `participants_count` увеличивается один раз, а счетчики выбранных вариантов — по одному разу каждый.
- Финальный результат после успешной финализации не изменяется.

## 5. Технологические ограничения

По возможности использовать стандартную библиотеку Go:

- `net/http` и шаблоны маршрутов `ServeMux`;
- `encoding/json`;
- `log/slog`;
- `database/sql`;
- `crypto/rand`, `crypto/hmac`, `crypto/sha256`, `crypto/subtle`;
- `encoding/base64`;
- `os`, `time`, `context`, `sync`, `os/signal`;
- `testing`, `httptest`;
- `embed` для SQL/Lua-ресурсов.

Неизбежные внешние зависимости:

- PostgreSQL driver: `github.com/jackc/pgx/v5/stdlib`;
- Redis client: `github.com/redis/go-redis/v9`;
- локальная генерация QR: `github.com/skip2/go-qrcode`.

Допустимо добавить Prometheus client, если `/metrics` реализуется полноценно. Для тестового также допустим небольшой собственный collector на `sync/atomic`, чтобы не расширять dependency graph.

Не использовать ORM, DI framework, web framework и generic repository abstraction. Конкретные PostgreSQL/Redis adapters и небольшие интерфейсы на границе use case достаточно прозрачны.

Минимальная версия Go — 1.22, поскольку спецификация использует method/path routing стандартного `http.ServeMux`.

## 6. Структура репозитория

```text
.
├── api/
│   ├── embed.go
│   └── openapi.yaml
├── cmd/
│   ├── api/main.go
│   └── migrate/main.go
├── internal/
│   ├── config/config.go
│   ├── domain/
│   │   ├── poll.go
│   │   ├── vote.go
│   │   └── errors.go
│   ├── service/
│   │   ├── poll_service.go
│   │   ├── vote_service.go
│   │   └── result_service.go
│   ├── httpapi/
│   │   ├── router.go
│   │   ├── admin_handlers.go
│   │   ├── public_handlers.go
│   │   ├── qr_handler.go
│   │   ├── web.go
│   │   ├── web/templates/{admin,poll}.html
│   │   ├── web/static/{app.css,admin.js,poll.js}
│   │   ├── metrics.go
│   │   ├── middleware.go
│   │   └── response.go
│   ├── postgres/
│   │   ├── polls.go
│   │   ├── results.go
│   │   └── migrations/
│   │       ├── embed.go
│   │       └── 001_init.sql
│   ├── redisstore/
│   │   ├── votes.go
│   │   ├── gates.go
│   │   ├── results.go
│   │   └── scripts/vote.lua
│   ├── identity/
│   │   ├── cookie.go
│   │   └── network.go
│   ├── qrsvg/generator.go
│   └── worker/snapshot.go
├── monitoring/prometheus.yml
├── deploy/swagger/nginx.conf
├── tests/
│   ├── integration/
│   └── load/vote.js
├── docs/
│   ├── architecture.md
│   ├── decisions/
│   └── ai/
├── .agents/skills/
├── AGENTS.md
├── Dockerfile
├── compose.yaml
├── Makefile
├── go.mod
└── README.md
```

Не создавать интерфейс на каждый struct. Нужны только интерфейсы, которые разделяют business use case и I/O, например:

```go
type PollRepository interface {
    Create(ctx context.Context, input CreatePoll) (Poll, error)
    GetByID(ctx context.Context, id string) (Poll, error)
    GetBySlug(ctx context.Context, slug string) (Poll, error)
    Publish(ctx context.Context, id string, now time.Time) (Poll, error)
    Close(ctx context.Context, id string, now time.Time) (Poll, error)
}

type VoteStore interface {
    Record(ctx context.Context, vote VoteCommand) (VoteStatus, error)
    Totals(ctx context.Context, pollID string, buckets int) (Totals, error)
}
```

## 7. PostgreSQL

### 7.1. Начальная миграция

```sql
CREATE TABLE polls (
    id              UUID PRIMARY KEY,
    slug            TEXT NOT NULL UNIQUE,
    question        TEXT NOT NULL CHECK (char_length(question) BETWEEN 1 AND 500),
    poll_type       TEXT NOT NULL CHECK (poll_type IN ('single_choice', 'multiple_choice')),
    max_choices     SMALLINT NOT NULL CHECK (max_choices BETWEEN 1 AND 20),
    status          TEXT NOT NULL CHECK (status IN ('draft', 'active', 'closed')),
    starts_at       TIMESTAMPTZ NOT NULL,
    ends_at         TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    published_at    TIMESTAMPTZ,
    closed_at       TIMESTAMPTZ,
    CHECK (starts_at < ends_at)
);

CREATE TABLE poll_options (
    id          UUID PRIMARY KEY,
    poll_id     UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    text        TEXT NOT NULL CHECK (char_length(text) BETWEEN 1 AND 200),
    position    SMALLINT NOT NULL CHECK (position >= 0),
    UNIQUE (poll_id, position)
);

CREATE TABLE poll_runtime_results (
    poll_id             UUID PRIMARY KEY REFERENCES polls(id) ON DELETE CASCADE,
    participants_count  BIGINT NOT NULL CHECK (participants_count >= 0),
    captured_at         TIMESTAMPTZ NOT NULL,
    finalized_at        TIMESTAMPTZ
);

CREATE TABLE poll_option_results (
    poll_id         UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id       UUID NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    vote_count      BIGINT NOT NULL CHECK (vote_count >= 0),
    captured_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (poll_id, option_id)
);

CREATE INDEX polls_status_time_idx ON polls(status, starts_at, ends_at);
```

UUID генерировать в приложении из 16 байт `crypto/rand`; выставить RFC 4122 version/variant bits и кодировать стандартным текстовым форматом. Это позволяет не подключать отдельную UUID-библиотеку.

### 7.2. Транзакции

- Создание poll + options выполняется одной транзакцией.
- Publish выполняет `SELECT ... FOR UPDATE`, проверяет status и инварианты, затем меняет status.
- Close также использует блокировку строки.
- Snapshot записывает header и все option totals одной транзакцией.
- Snapshot обновляет строку только если новый `captured_at` не старее сохраненного; это защищает от гонки двух workers.
- Final snapshot устанавливает `finalized_at`; последующие попытки должны вернуть существующий результат без изменения totals.

### 7.3. Миграции

Реализовать небольшой `cmd/migrate`:

- SQL-файлы встроены через `//go:embed`;
- таблица `schema_migrations(version, applied_at)`;
- перед применением берется PostgreSQL advisory lock;
- каждый файл применяется в отдельной транзакции;
- команда `migrate up` идемпотентна.

Для одного SQL-файла допустим более простой runner, но README обязан содержать одну надежную команду миграции.

## 8. Redis data model

Количество logical buckets задается `VOTE_BUCKETS`, по умолчанию `64`. После публикации опроса менять значение нельзя, иначе чтение totals потеряет часть счетчиков.

Для bucket `b` используются ключи:

```text
poll-gate:{<pollID>:<b>}                 string "active"
poll-dedup:{<pollID>:<b>}:<deviceHash>   string "1"
poll-counts:{<pollID>:<b>}               hash
```

Поля hash:

```text
__participants -> count
<optionUUID>    -> count
```

Фрагмент `{pollID:bucket}` является Redis Cluster hash tag. Gate, dedup и counters одной корзины попадают в один slot, поэтому их можно использовать в одном Lua script.

TTL:

- gate: до `ends_at + REDIS_RETENTION`;
- dedup: до `ends_at + REDIS_RETENTION`;
- counters: до `ends_at + REDIS_RETENTION`;
- `REDIS_RETENTION` по умолчанию 24 часа, чтобы успеть финализировать опрос после временного сбоя PostgreSQL.

`ttlSeconds` всегда вычисляется сервером и ограничивается минимумом `1` и конфигурируемым максимумом.

## 9. Атомарная запись голоса

### 9.1. Lua script

`internal/redisstore/scripts/vote.lua`:

```lua
-- KEYS[1] gate key
-- KEYS[2] dedup key
-- KEYS[3] counters hash
-- ARGV[1] ttl seconds
-- ARGV[2..n] validated unique option IDs

if redis.call('GET', KEYS[1]) ~= 'active' then
    return -1
end

local inserted = redis.call('SET', KEYS[2], '1', 'NX', 'EX', ARGV[1])
if not inserted then
    return 0
end

redis.call('HINCRBY', KEYS[3], '__participants', 1)
for i = 2, #ARGV do
    redis.call('HINCRBY', KEYS[3], ARGV[i], 1)
end
redis.call('EXPIRE', KEYS[3], ARGV[1])

return 1
```

Возвращаемые значения:

- `1` — голос записан;
- `0` — голос этого устройства уже существует;
- `-1` — опрос неактивен или gate отсутствует.

До вызова Lua приложение обязано проверить уникальность option IDs, их принадлежность опросу и тип опроса. Пространство Redis-ключей принадлежит только приложению: нельзя допускать записи значений другого типа, поскольку Redis Lua atomic, но не откатывает уже выполненные команды при runtime error.

### 9.2. Выбор корзины

```text
bucket = uint64(first_8_bytes(SHA-256(deviceHash))) % VOTE_BUCKETS
```

Можно переиспользовать уже рассчитанный SHA-256. Алгоритм должен быть детерминированным и покрыт unit-тестом с фиксированными vectors.

### 9.3. Gate lifecycle

Publish:

1. PostgreSQL transaction переводит poll в `active`.
2. Сервис создает gate для каждой корзины с нужным TTL.
3. Если Redis setup не удался, publish отвечает ошибкой и выполняет compensating transition обратно в `draft` либо сохраняет явный `activation_failed` вне пользовательской модели.

Для простоты предпочтительнее сначала создать Redis gates, затем перевести PostgreSQL в `active`; при ошибке PostgreSQL удалить только gates этого poll. Во время короткого окна gate существует, но публичная конфигурация еще недоступна, поэтому валидный клиент не сможет проголосовать.

Close:

1. удалить или заменить значением `closed` все bucket gates;
2. дождаться завершения уже исполняющихся Lua calls;
3. прочитать totals;
4. записать final snapshot в PostgreSQL;
5. перевести poll в `closed` с `finalized_at` в той же БД-транзакции.

Операция должна быть повторяемой после частичного сбоя. Не удалять dedup/counters во время close — они очищаются TTL.

Практический упрощенный вариант: status `closed` записывается перед финализацией, а отдельная идемпотентная `Finalize` доводит процесс до конца. API результатов показывает `finalization_pending`, если финального snapshot еще нет.

## 10. Идентификация и дедупликация

### 10.1. Cookie

Имя: `poll_device`.

Payload содержит:

```text
version.deviceID.signature
```

- `deviceID`: 16 случайных байт, base64url без padding;
- `signature`: `HMAC-SHA256(COOKIE_SIGNING_KEY, version + "." + deviceID)`;
- сравнение подписи через `crypto/subtle.ConstantTimeCompare`;
- flags: `HttpOnly`, `Secure` в production, `SameSite=Lax`, `Path=/`, `MaxAge=365d`.

При отсутствующей или неверной cookie создать новый ID. Не логировать payload cookie.

### 10.2. Device hash

Redis key не должен содержать исходный `deviceID`:

```text
deviceHash = hex(HMAC-SHA256(DEDUP_HMAC_KEY, pollID + "\n" + deviceID))
```

Poll ID включен в HMAC, чтобы один и тот же браузер нельзя было связать между опросами по Redis key dump.

### 10.3. IP rate limit

IP не используется как уникальный голос. Он нужен только для ограничения частоты:

- IPv4 нормализовать до `/24`;
- IPv6 — до `/56`;
- нормализованную сеть пропустить через HMAC;
- не хранить и не логировать сырой IP;
- token bucket или fixed window реализовать отдельным Redis Lua script;
- лимиты задавать конфигурацией.

Не доверять произвольному `X-Forwarded-For`. По умолчанию использовать `RemoteAddr`. Поддержку proxy header включать только вместе со списком доверенных proxy/LB.

### 10.4. Ограничение подхода

Удаление cookie, другой браузер или автоматизированный клиент обходят защиту. Это соответствует условию о базовой дедупликации. В README ограничение должно быть сформулировано явно.

## 11. Кэш конфигурации опроса

Нельзя читать PostgreSQL на каждый голос. Реализовать in-process cache опубликованных poll definitions:

- immutable value: poll ID, type, option set, max choices, ends_at;
- map под `sync.RWMutex`;
- TTL 30 секунд;
- при cache miss одна goroutine загружает значение, остальные запросы к тому же ID не должны создавать бесконтрольный stampede;
- при publish локальная реплика прогревает cache;
- stale status безопасен, потому что Lua gate остается окончательной проверкой активности.

Допустимый упрощенный MVP: mutex на время одного DB load для конкретного ключа без внешней `singleflight` зависимости. Не держать глобальный mutex во время сетевого запроса для всех poll IDs.

## 12. HTTP API

Base path: `/api/v1`.

Все timestamp — RFC 3339 UTC. Content-Type — `application/json`. Неизвестные JSON-поля отклонять через `Decoder.DisallowUnknownFields`. Тело ограничить `http.MaxBytesReader`, например 32 KiB.

### 12.1. Public endpoints

#### `GET /api/v1/polls/{slug}`

Возвращает только опубликованный опрос в разрешенном временном окне.

`200`:

```json
{
  "id": "uuid",
  "slug": "summer-2026-a8f3",
  "question": "Какой вариант вы выбираете?",
  "type": "single_choice",
  "max_choices": 1,
  "ends_at": "2026-09-11T18:01:00Z",
  "options": [
    {"id": "uuid", "text": "A"},
    {"id": "uuid", "text": "B"}
  ]
}
```

До начала или после окончания можно вернуть `404`, чтобы не раскрывать draft metadata. Выбрать это поведение единообразно и описать в README.

#### `POST /api/v1/polls/{id}/votes`

Request:

```json
{"option_ids":["uuid"]}
```

Новый голос: `201`.

```json
{"status":"recorded"}
```

Повторная доставка: `200`.

```json
{"status":"already_recorded"}
```

`already_recorded` означает, что предыдущий голос уже учтен. Это делает retry после потерянного HTTP-ответа безопасным.

Ошибки:

- `400 invalid_request` — JSON/UUID/повторяющиеся options;
- `404 poll_not_found`;
- `409 poll_not_active`;
- `422 invalid_selection`;
- `429 rate_limited` с `Retry-After`;
- `503 temporarily_unavailable` при ошибке Redis.

### 12.2. Admin endpoints

Все требуют `Authorization: Bearer <ADMIN_TOKEN>`.

#### `GET /api/v1/admin/polls?limit=50`

Возвращает сохранённые определения опросов из PostgreSQL, новые первыми. `limit` необязателен и ограничен диапазоном `1..100`. Endpoint нужен для восстановления контекста admin UI после перезагрузки вкладки и не читает результаты голосования: live/final counters по-прежнему загружаются отдельной ручкой results.

#### `POST /api/v1/admin/polls`

```json
{
  "question": "Какой вариант вы выбираете?",
  "type": "single_choice",
  "max_choices": 1,
  "starts_at": "2026-09-11T18:00:00Z",
  "ends_at": "2026-09-11T18:01:00Z",
  "options": [
    {"text": "A"},
    {"text": "B"}
  ]
}
```

Ответ `201` содержит poll ID, slug, option IDs и status `draft`.

#### `POST /api/v1/admin/polls/{id}/publish`

- `200` для успешной публикации;
- `200` для уже опубликованного того же неизмененного poll;
- `409` для недопустимого перехода.

#### `POST /api/v1/admin/polls/{id}/close`

Идемпотентно закрывает и финализирует poll. При частичной ошибке возвращает `503` и оставляет состояние, которое можно безопасно довести повторным вызовом.

#### `GET /api/v1/admin/polls/{id}/results`

```json
{
  "poll_id": "uuid",
  "status": "active",
  "source": "redis_live",
  "as_of": "2026-09-11T18:00:42Z",
  "participants_count": 1000,
  "options": [
    {"id": "uuid", "text": "A", "count": 610, "participant_percent": 61.0},
    {"id": "uuid", "text": "B", "count": 390, "participant_percent": 39.0}
  ]
}
```

У multiple choice сумма процентов может превышать 100%, потому что denominator — число участников, а не сумма выбранных ответов.

### 12.3. Реализованный минимальный frontend

Frontend реализован как небольшие same-origin HTML/CSS/JavaScript assets без Node.js и отдельного frontend framework. Он не меняет JSON API или Redis-first vote path:

```text
internal/httpapi/web/
├── templates/
│   ├── poll.html
│   └── admin.html
└── static/
    ├── app.css
    ├── poll.js
    └── admin.js
```

Assets и templates встраиваются в Go-бинарник через `//go:embed`. Go API отдает:

- `GET /p/{slug}` — публичную страницу голосования;
- `GET /admin` — минимальную административную страницу;
- `GET /static/*` — versioned static assets с cache headers.

Публичная страница вызывает существующий `GET /api/v1/polls/{slug}` с same-origin credentials, отображает radio buttons для `single_choice` или checkboxes со счетчиком лимита для `multiple_choice`, затем отправляет `POST /api/v1/polls/{id}/votes`. Нужны состояния loading, unavailable, validation error, `recorded` и `already_recorded`; повторная отправка блокируется после успешного ответа. Все полученные тексты вставляются через `textContent`, а не `innerHTML`.

Admin-страница содержит формы create/publish/close, ссылку на публичную страницу, QR-код и live results с polling не чаще одного раза в секунду. После ввода токена она вызывает `GET /api/v1/admin/polls?limit=100`, показывает сохранённые в PostgreSQL опросы и позволяет открыть черновик, активный или закрытый опрос после перезагрузки страницы. Для закрытого опроса загружается финальный snapshot, для активного возобновляется live polling. Admin token вводится пользователем на время открытой вкладки и хранится только в памяти JavaScript; не помещать его в URL, cookie, localStorage, HTML или логи. Это только локальный/demo UI, а не замена полноценной admin-аутентификации.

Минимальные меры безопасности реализованы: restrictive Content-Security-Policy, `X-Content-Type-Options: nosniff`, `Referrer-Policy`, отсутствие inline scripts и сторонних CDN. Same-origin раздача позволяет не включать CORS. Handler tests проверяют HTML routes, security headers и отсутствие admin token в ответах; браузерный smoke test проверяет основной административный и пользовательский сценарий.

### 12.4. Реализованный QR-код

После публикации admin UI формирует QR-код только для публичного URL:

```text
PUBLIC_BASE_URL + "/p/" + poll.slug
```

Для QR добавлен `PUBLIC_BASE_URL`: конфигурация принимает только абсолютный HTTP(S) origin без credentials, path, query и fragment. При `COOKIE_SECURE=true` требуется HTTPS; для локального запуска допустим `http://localhost:8080`. URL никогда не строится из недоверенного `Host`/`X-Forwarded-Host`.

QR генерируется внутри Go-процесса зафиксированной библиотекой `github.com/skip2/go-qrcode` и преобразуется в SVG локальным адаптером `internal/qrsvg`. Внешний QR SaaS не используется, поэтому URL опроса не передается третьей стороне. Endpoint `GET /api/v1/admin/polls/{id}/qr.svg` требует admin bearer token, возвращает `image/svg+xml`, использует medium error correction и безопасное имя файла. Результат кэшируется в памяти по `(slug, PUBLIC_BASE_URL)`; в PostgreSQL и Redis QR не сохраняется.

Admin HTML показывает preview и ссылку на скачивание. Тесты декодируют сгенерированный QR либо как минимум проверяют формат, content type и точное кодируемое значение. QR не содержит admin token, device ID или персональные данные и не участвует в обработке голоса.

### 12.5. Error envelope

```json
{
  "error": {
    "code": "invalid_selection",
    "message": "selection does not satisfy poll rules",
    "request_id": "..."
  }
}
```

Не возвращать клиенту SQL/Redis errors и stack traces.

### 12.6. OpenAPI

Контракт JSON API и защищенного QR endpoint описан в `api/openapi.yaml` в формате OpenAPI 3.1. Спецификация включает client/admin operations, bearer authentication, cookie-based device identity, request/response schemas и общий error envelope. Она доступна из запущенного приложения по `GET /openapi.yaml`; файл остается первичным источником.

Для локального чтения и `Try it out` Compose запускает зафиксированный образ Swagger UI на `${SWAGGER_PORT:-8081}`. Спецификация монтируется read-only, а отдельный Nginx config проксирует `/api/` и `/health/` к `api:8080`, поэтому Go API не требует CORS. Swagger является dev/documentation tooling и не добавляет зависимостей в production-бинарник.

## 13. Middleware и сервер

Порядок middleware:

1. panic recovery;
2. request ID;
3. access logging;
4. body size/content type;
5. admin auth только на admin subtree;
6. IP rate limit для vote endpoint.

Bearer token проверять не прямым сравнением строк: вычислить SHA-256 от configured token и предоставленного token, затем сравнить два fixed-size digest через `subtle.ConstantTimeCompare`.

Настроить `http.Server`:

- `ReadHeaderTimeout`;
- `ReadTimeout`;
- `WriteTimeout`;
- `IdleTimeout`;
- максимальный размер headers на proxy/LB;
- graceful shutdown через `signal.NotifyContext` и timeout 10–20 секунд.

Не запускать goroutine на каждый голос кроме тех, которые создает `net/http`. Redis вызов синхронный и ограничен request context deadline.

## 14. Результаты и snapshot worker

Чтение live totals:

1. pipeline `HGETALL` по всем bucket counter keys;
2. проверить и сложить `__participants`;
3. сложить каждый option ID;
4. неизвестные поля логировать как warning и игнорировать;
5. проверять переполнение `int64`.

Admin polling результатов может сам создать нагрузку. На один poll допустимо кэшировать агрегированный ответ на 250–1000 мс.

Snapshot worker:

- ticker по умолчанию 5 секунд;
- выбирает active polls;
- берет Redis lock для poll через `SET snapshot-lock:<pollID> <instanceID> NX PX <ttl>`;
- читает totals;
- записывает PostgreSQL snapshot;
- освобождает lock только compare-and-delete Lua script, чтобы не удалить чужой lock;
- ошибки логирует, но не завершает API process.

Если времени мало, worker можно не включать: обязательна финализация при close, Redis AOF и явное описание окна риска. Но интерфейсы следует оставить так, чтобы worker добавлялся без изменения vote path.

## 15. Состояния и частичные сбои

| Сбой | Поведение |
|---|---|
| Redis недоступен при голосовании | `503`, голос не считается принятым |
| PostgreSQL недоступен, poll есть в cache и gate active | голос можно принять в Redis; admin операции недоступны |
| HTTP response потерян после Redis success | retry получает `already_recorded` |
| API instance упал | другие stateless replicas продолжают работу |
| Snapshot не записался | Redis данные сохраняются до retention, worker повторяет |
| Close оборвался | повторный close/finalize завершает операцию |
| Один bucket Redis Cluster недоступен | затронутые запросы получают `503`; не перенаправлять их в другую корзину, иначе сломается dedup |
| Cookie удалена | пользователь технически может проголосовать снова; это известное ограничение |

Не делать fallback «Redis сломан — запишем напрямую в PostgreSQL»: это создает две несовместимые ветки дедупликации и сложное последующее слияние.

## 16. Конфигурация

Минимальные environment variables:

```text
HTTP_ADDR=:8080
PUBLIC_BASE_URL=http://localhost:8080
DATABASE_URL=postgres://poll:poll@postgres:5432/poll?sslmode=disable
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
ADMIN_TOKEN=<secret>
COOKIE_SIGNING_KEY=<at-least-32-random-bytes>
DEDUP_HMAC_KEY=<different-at-least-32-random-bytes>
COOKIE_SECURE=false
VOTE_BUCKETS=64
REDIS_RETENTION=24h
POLL_CACHE_TTL=30s
SNAPSHOT_INTERVAL=5s
VOTE_REQUEST_TIMEOUT=2s
TRUST_PROXY_HEADERS=false
LOG_LEVEL=info
```

Правила:

- production secrets не имеют default values;
- signing и dedup keys должны отличаться;
- при старте валидировать диапазоны и длину секретов;
- логировать эффективную несекретную конфигурацию;
- не использовать `.env` с реальными секретами в Git; предоставить `.env.example`.

## 17. Docker и Linux

### Dockerfile

- multi-stage build;
- builder запускает `go build -trimpath`;
- runtime image содержит CA certificates и непривилегированного пользователя;
- бинарник принимает SIGTERM;
- не копировать исходники и Go toolchain в runtime stage;
- healthcheck вызывает `/health/ready`.

### Compose

Services:

- `api`;
- `migrate` one-shot;
- `postgres` с healthcheck и named volume;
- `redis` с AOF `appendonly yes`, healthcheck и named volume;
- опциональный profile `observability`.

API зависит от успешной миграции и healthy Redis/PostgreSQL. Для локального HTTP `COOKIE_SECURE=false`; README поясняет, что production требует TLS и `true`.

#### Реализованный Prometheus profile

В `compose.yaml` добавлен opt-in service `prometheus` с `profiles: ["observability"]`, фиксированным образом `prom/prometheus:v3.5.0`, read-only конфигурацией `monitoring/prometheus.yml` и named volume `prometheus-data` для TSDB. Prometheus опрашивает `http://api:8080/metrics` каждые 5 секунд; наружу для локальной диагностики публикуется `${PROMETHEUS_PORT:-9090}`.

```yaml
prometheus:
  image: prom/prometheus:v3.5.0
  profiles: ["observability"]
  command:
    - --config.file=/etc/prometheus/prometheus.yml
    - --storage.tsdb.path=/prometheus
  volumes:
    - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
    - prometheus-data:/prometheus
  ports:
    - "9090:9090"
  depends_on:
    api:
      condition: service_healthy
```

Локальный запуск: `docker compose --profile observability up --build -d`; проверка — `http://localhost:9090/targets`. Минимальный граф RPS именно на endpoint голосования строится запросом `sum(rate(http_requests_total{route="/api/v1/polls/{id}/votes",method="POST"}[1m]))`; граф принятых уникальных голосов — `sum(rate(votes_recorded_total[1m]))`. В production `/metrics` и UI Prometheus не должны быть публичными: доступ ограничивается внутренней сетью, ingress policy или отдельной аутентификацией.

## 18. Health и наблюдаемость

### Endpoints

- `GET /health/live`: process event loop работает, без сетевых проверок;
- `GET /health/ready`: короткие `PingContext` к PostgreSQL и Redis;
- `GET /metrics`: счетчики и latency histogram, если реализовано.

### Метрики

- `http_requests_total{route,method,status}`;
- `http_request_duration_seconds{route}`;
- `votes_recorded_total`;
- `votes_duplicate_total`;
- `votes_rejected_total{reason}`;
- `redis_operation_duration_seconds{operation}`;
- `snapshot_success_total`, `snapshot_error_total`;
- `snapshot_age_seconds`.

Не использовать poll ID как metric label: это приведет к высокой cardinality. Poll ID допустим в structured log.

### Логи

JSON через `slog`. Поля: timestamp, level, message, request_id, route, status, duration, poll_id. Не логировать Authorization, cookie, сырой IP, device hash и тело vote request.

## 19. Тестовая стратегия

### Unit tests

- table-driven validation всех poll types;
- status transitions;
- cookie sign/verify и поврежденная подпись;
- HMAC device hash;
- bucket selection с фиксированными vectors;
- TTL boundaries;
- JSON decoder rejects unknown fields;
- auth digest comparison;
- result aggregation, включая multiple choice и overflow.

### Handler tests

Через `httptest` и hand-written fakes:

- happy paths;
- malformed JSON и слишком большое body;
- duplicate vote semantics;
- status mapping domain errors -> HTTP;
- request ID в error response;
- admin auth.

### PostgreSQL integration tests

- create transaction не оставляет poll без options;
- concurrent publish дает один корректный transition;
- snapshot upsert не затирается более старым snapshot;
- finalization идемпотентна.

### Redis integration tests

- 100–1000 параллельных записей с одним device hash дают `participants=1`;
- разные devices учитываются;
- multiple choice увеличивает participant один раз;
- закрытый/missing gate ничего не увеличивает;
- dedup и counters имеют TTL;
- чтение N buckets дает правильную сумму;
- option ID с дублем отбрасывается до Lua.

### Race и load tests

```bash
go test -race ./...
k6 run tests/load/vote.js
```

k6 должен принимать параметры `BASE_URL`, `POLL_ID`, `VUS`, `DURATION`, долю duplicate requests. В отчет сохранить hardware, RPS, p50/p95/p99 и error rate. Не коммитить только красивое итоговое число без сценария и конфигурации.

## 20. Порядок реализации

Рекомендуемые этапы, каждый должен оставлять собираемый код:

1. Инициализировать module, config, `main`, health endpoints и graceful shutdown.
2. Добавить domain structs, validators и unit tests.
3. Добавить миграцию и PostgreSQL poll repository.
4. Реализовать admin create/publish и public get.
5. Реализовать cookie identity и тесты.
6. Реализовать Redis key builder, Lua vote script и integration tests.
7. Собрать vote service и public endpoint.
8. Реализовать aggregation/results.
9. Реализовать close/finalization и тесты частичных повторов.
10. Добавить auth, rate limit, middleware и structured errors.
11. Добавить Compose, Dockerfile и load test.
12. Закончить README, ADR, AI artifacts и выполнить clean-room запуск по документации.
13. Добавить embedded public/admin HTML и браузерные smoke tests — реализовано.
14. Добавить QR endpoint на основе валидированного `PUBLIC_BASE_URL` — реализовано.
15. Добавить Prometheus profile и проверить scrape target/основные PromQL-запросы — реализовано и проверено.

Если срок поджимает, сначала отказаться от HTML и periodic worker, затем от полноценных Prometheus histograms. Нельзя отказываться от atomic dedup, integration test конкурентного дубля, Docker Compose и README.

## 21. Definition of Done для реализующего агента

- [x] Все обязательные endpoints реализованы.
- [x] Hot path не делает запрос в PostgreSQL при cache hit.
- [x] Redis Lua проверяет gate, dedup и увеличивает counters атомарно.
- [x] Counters разбиты на configurable buckets.
- [x] Cookie подписана; ключ Redis не содержит исходный device ID.
- [x] Один IP не считается одним пользователем.
- [x] Закрытие и финализация повторяемы после ошибки.
- [x] Нет data race по `go test -race`.
- [x] Integration test доказывает ровно один инкремент при конкурентных дублях.
- [x] Compose стартует с чистыми volumes по README.
- [x] Секреты и персональные данные отсутствуют в Git/logs.
- [x] Ограничения надежности и дедупликации описаны честно.
- [x] AI artifacts и сведения о ручной проверке добавлены.

## 22. AI engineering features для репозитория

AI-функции здесь разумнее показать не как искусственно встроенный LLM в пользовательский vote path, а как воспроизводимые agentic workflows рядом с кодом. Официальная документация Codex описывает repository skills в `.agents/skills`: каждый skill — отдельная директория с обязательным `SKILL.md` и опциональными `scripts/`, `references/`, `assets/`. Skill может вызываться явно или подбираться по `description`: [Build skills — official OpenAI documentation](https://learn.chatgpt.com/docs/build-skills).

### 22.1. Корневой `AGENTS.md`

Это не skill, а короткая постоянная инструкция для любого coding agent:

- команды build/test/lint;
- карта пакетов;
- обязательные инварианты vote path;
- правило «standard library first»;
- запрет сырых IP/cookie в логах;
- правило добавлять integration test при изменении Lua/keyspace;
- какие сгенерированные файлы нельзя редактировать вручную;
- Definition of Done.

Не копировать в него всю архитектуру: дать ссылки на этот документ и ADR. Чем короче и проверяемее инструкции, тем меньше конфликтов.

### 22.2. Skill `implement-poll-change`

Путь:

```text
.agents/skills/implement-poll-change/
├── SKILL.md
└── references/
    ├── domain-invariants.md
    └── api-errors.md
```

Назначение: безопасно реализовывать новый endpoint или изменение domain logic в соответствии со слоями проекта.

Минимальный `SKILL.md`:

```markdown
---
name: implement-poll-change
description: Implement or modify poll-service Go endpoints, domain rules, PostgreSQL repositories, or Redis vote behavior. Use for feature work in this repository; do not use for documentation-only changes.
---

1. Read AGENTS.md and the referenced domain invariants.
2. Trace the affected HTTP handler, service, and storage adapter.
3. Preserve the no-PostgreSQL-on-vote-hot-path rule.
4. Prefer the Go standard library; justify each new dependency.
5. Add table-driven unit tests and the relevant integration test.
6. Run gofmt, go test, go test -race, and report exact commands/results.
7. Update the API/ADR documentation when behavior changes.
```

Что демонстрирует: умение кодировать не только prompt, но и repository-specific workflow с явными границами и проверяемым выходом.

### 22.3. Skill `verify-vote-invariants`

Путь:

```text
.agents/skills/verify-vote-invariants/
├── SKILL.md
├── references/redis-keyspace.md
└── scripts/run-vote-verification.sh
```

Назначение: проверка изменений Redis Lua, key builder, identity и aggregation.

Workflow:

1. определить затронутые инварианты;
2. проверить одинаковый Redis Cluster hash tag для всех Lua keys;
3. проверить duplicate/closed gate/multiple choice/TTL;
4. поднять integration services;
5. запустить целевые тесты и `go test -race`;
6. вернуть компактный отчет с командами и результатами.

Скрипт должен только оркестрировать детерминированные команды. Не стоит прятать бизнес-логику в shell script.

### 22.4. Skill `run-load-test-report`

Путь:

```text
.agents/skills/run-load-test-report/
├── SKILL.md
├── scripts/run.sh
├── references/scenarios.md
└── assets/report-template.md
```

Назначение: запустить один из заранее описанных k6-сценариев и сформировать воспроизводимый Markdown-отчет.

Inputs:

- scenario: unique, duplicate или burst;
- VUs/duration/target RPS;
- URL тестового окружения.

Outputs:

- дата, commit SHA, hardware/environment;
- параметры Redis/PostgreSQL/API;
- RPS, p50/p95/p99, errors;
- наблюдавшееся ограничение;
- ссылка на raw result artifact.

Этот skill хорошо показывает, что AI используется для повторяемой инженерной операции, а не для генерации неподтвержденных performance claims.

### 22.5. Skill `write-adr`

Небольшой instruction-only skill для создания ADR:

```text
.agents/skills/write-adr/SKILL.md
```

Он должен запросить/извлечь контекст решения и создать следующий номер в `docs/decisions/NNNN-title.md` со структурой: Context, Decision, Alternatives, Consequences, Validation. Подходит для решений о Redis buckets, durability, cookie identity и отказе от Kafka.

### 22.6. Skill evals

Чтобы skills не выглядели декоративно, добавить тестовые prompts и ожидаемые свойства результата:

```text
docs/ai/skill-evals/
├── implement-poll-change.md
├── verify-vote-invariants.md
└── run-load-test-report.md
```

Примеры eval cases:

- «Добавь возможность менять options активного опроса» — skill должен отказаться нарушать инвариант и предложить новый poll/versioning.
- «При падении Redis пиши голос в PostgreSQL» — должен обнаружить split-brain дедупликации и потребовать архитектурного решения.
- «Ускорь Lua, убрав gate check» — должен выявить гонку закрытия.
- «Покажи результат load test» — не должен придумывать числа при отсутствии запуска.

Для каждого eval фиксировать не точный текст ответа, а проверяемые criteria. Это демонстрирует понимание, что agent workflow тоже нужно тестировать.

### 22.7. AI artifacts

```text
docs/ai/
├── README.md
├── prompts.md
├── decisions.md
├── reviews.md
└── skill-evals/
```

`README.md` должен перечислять:

- какие модели/агенты использовались;
- для каких задач;
- какие результаты приняты, изменены или отклонены;
- как человек проверял код;
- какие команды были реально выполнены;
- известные ошибки или ограничения AI output.

Не сохранять chain-of-thought, секреты и приватные данные. Достаточны пользовательские prompts, итоговые ответы/patches, краткие rationale и результаты проверок.

### 22.8. Опциональные AI features продукта

Если после backend остается время, AI можно добавить вне критического vote path:

- генерация черновика вопроса и вариантов ответа в admin API;
- проверка нейтральности формулировки вопроса;
- текстовое резюме финальных агрегированных результатов;
- объяснение аномалий на основании уже рассчитанных метрик.

Эти функции должны быть асинхронными/опциональными, не получать device hashes или IP и не влиять на прием голосов. Для двухдневного тестового репозиторные skills, их eval cases и прозрачный AI audit trail полезнее, чем незавершенная runtime-интеграция с LLM.

## 23. Решения, которые реализующий агент не должен менять молча

Для изменения любого пункта нужен ADR:

- Redis-first hot path;
- отсутствие fallback-записи голосов в PostgreSQL;
- bucketed counters;
- cookie-based dedup, IP только как rate-limit signal;
- immutable active poll;
- fail-closed при недоступности Redis;
- PostgreSQL как durable store финальных результатов;
- отсутствие runtime AI на критическом пути.
