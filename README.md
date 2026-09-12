# Poll Service

Сервис коротких интерактивных опросов на Go, Redis и PostgreSQL. В проект входят JSON API, публичная HTML-страница голосования, минимальная admin-панель, QR-коды публичных ссылок, Swagger UI, Prometheus monitoring profile, миграции и тесты.

Redis атомарно принимает и дедуплицирует live-голоса, а PostgreSQL хранит опросы, промежуточные снимки и неизменяемые финальные результаты. При cache hit горячий путь голосования не обращается к PostgreSQL.

## Быстрый запуск

Требуется Linux с Docker и Docker Compose.

```bash
docker compose up --build -d
curl -fsS http://localhost:8080/health/ready
```

После запуска доступны:

| Назначение | Адрес |
|---|---|
| Admin-панель | [http://localhost:8080/admin](http://localhost:8080/admin) |
| Публичный опрос | `http://localhost:8080/p/{slug}` |
| Swagger UI | [http://localhost:8081](http://localhost:8081) |
| OpenAPI YAML | [http://localhost:8080/openapi.yaml](http://localhost:8080/openapi.yaml) |
| Метрики API | [http://localhost:8080/metrics](http://localhost:8080/metrics) |
| PostgreSQL | `localhost:5432` |
| Redis | `localhost:6380` (`REDIS_PORT` позволяет изменить порт) |

Миграции выполняются one-shot контейнером до запуска API. Локальные значения секретов из Compose предназначены только для разработки.

Остановить сервисы, сохранив данные:

```bash
docker compose down
```

Команда `docker compose down -v` дополнительно и безвозвратно удалит локальные volumes PostgreSQL, Redis и Prometheus.

## Как работать администратору

### Через HTML-панель

1. Откройте [http://localhost:8080/admin](http://localhost:8080/admin).
2. Введите локальный token `ADMIN_TOKEN` из вашего `.env` окружения. Токен хранится только в памяти открытой вкладки и очищается при её закрытии.
3. После проверки токена блок **Сохранённые опросы** загрузит до 100 последних опросов из PostgreSQL. Нажмите **Открыть** у нужного опроса, чтобы снова управлять черновиком, наблюдать активный опрос или посмотреть финальные результаты закрытого — в том числе после k6-теста и перезагрузки страницы. Кнопка **Обновить список** перечитывает данные.
4. Для нового опроса заполните вопрос, тип, время начала/окончания и варианты — по одному на строку. Для `single_choice` максимум всегда равен одному; для `multiple_choice` задайте допустимый максимум вариантов.
5. Нажмите **Создать черновик**. До публикации зрители не видят опрос.
6. Нажмите **Опубликовать**. Панель покажет публичную ссылку и QR-код, который можно открыть на телефоне или скачать как SVG.
7. Во время активного опроса результаты обновляются раз в секунду. Число участников считается отдельно от суммы выбранных вариантов, поэтому в multiple-choice сумма процентов может превышать 100%.
8. Нажмите **Закрыть**, чтобы закрыть Redis gates и сохранить финальный неизменяемый результат в PostgreSQL. Повторное закрытие безопасно и возвращает тот же результат.

Admin-панель — минимальный demo UI, а не полноценный RBAC. Она не записывает token в URL, cookie или `localStorage`.

### Через JSON API

Список сохранённых опросов (новые первыми, `limit` от 1 до 100, по умолчанию 50):

```bash
curl -sS 'http://localhost:8080/api/v1/admin/polls?limit=100' \
  -H 'Authorization: Bearer ADMIN_TOKEN_HERE'
```

Создание черновика:

```bash
curl -sS -X POST http://localhost:8080/api/v1/admin/polls \
  -H 'Authorization: Bearer ADMIN_TOKEN_HERE' \
  -H 'Content-Type: application/json' \
  -d '{
    "question":"Какой вариант вы выбираете?",
    "type":"single_choice",
    "max_choices":1,
    "starts_at":"2026-01-01T00:00:00Z",
    "ends_at":"2030-01-01T00:00:00Z",
    "options":[{"text":"A"},{"text":"B"}]
  }'
```

Подставьте полученный `id`:

```bash
curl -sS -X POST http://localhost:8080/api/v1/admin/polls/{id}/publish \
  -H 'Authorization: Bearer ADMIN_TOKEN_HERE'

curl -sS http://localhost:8080/api/v1/admin/polls/{id}/results \
  -H 'Authorization: Bearer ADMIN_TOKEN_HERE'

curl -sS -X POST http://localhost:8080/api/v1/admin/polls/{id}/close \
  -H 'Authorization: Bearer ADMIN_TOKEN_HERE'
```

QR-код опубликованного опроса:

```bash
curl -fsS http://localhost:8080/api/v1/admin/polls/{id}/qr.svg \
  -H 'Authorization: Bearer ADMIN_TOKEN_HERE' \
  -o poll.svg
```

## Как голосовать пользователю

1. Администратор передаёт ссылку вида `http://localhost:8080/p/{slug}` или показывает соответствующий QR-код. Для телефона `PUBLIC_BASE_URL` должен указывать на реально доступный телефонному браузеру адрес, а не на его собственный `localhost`.
2. Страница загрузит только активный опрос в интервале `[starts_at, ends_at)`. Для одного варианта отображаются radio buttons, для нескольких — checkboxes с ограничением выбора.
3. Выберите вариант или варианты и нажмите **Отправить голос**.
4. Первый валидный голос покажет «Ваш голос принят». Повтор из того же браузера покажет «Ваш голос уже был учтён» и не изменит счётчики.

Сервер устанавливает подписанную `HttpOnly`, `SameSite=Lax` cookie `poll_device`. Сырой device ID, cookie и IP не попадают в PostgreSQL или логи.

Прямой API-вызов пользователя с сохранением cookie:

```bash
curl -c device.cookie -b device.cookie \
  -H 'Content-Type: application/json' \
  -d '{"option_ids":["{option-id}"]}' \
  http://localhost:8080/api/v1/polls/{poll-id}/votes
```

Public `GET /api/v1/polls/{slug}` возвращает `404` для черновика, закрытого опроса и времени вне разрешённого окна. JSON API отклоняет неизвестные поля и тела больше 32 KiB.

## OpenAPI

Полная OpenAPI 3.1 спецификация admin/client и health endpoints находится в [api/openapi.yaml](api/openapi.yaml) и раздаётся работающим API по адресу [http://localhost:8080/openapi.yaml](http://localhost:8080/openapi.yaml).

Для удобного чтения и интерактивных запросов Compose запускает [Swagger UI на http://localhost:8081](http://localhost:8081). Он читает тот же `api/openapi.yaml`, поэтому отдельной копии контракта нет. Swagger-контейнер проксирует `/api/*` и `/health/*` в Go API по внутренней Compose-сети: кнопка **Try it out** работает без включения CORS в приложении.

## Monitoring и график RPS

Запустите основной stack вместе с opt-in Prometheus profile:

```bash
docker compose --profile observability up --build -d
```

Prometheus доступен на [http://localhost:9090](http://localhost:9090), а [http://localhost:9090/targets](http://localhost:9090/targets) должен показывать target `poll-api` в состоянии `UP`. Он опрашивает `api:8080/metrics` каждые пять секунд и хранит локальные данные семь дней в named volume.

Чтобы построить минимальный граф RPS запросов голосования:

1. Создайте и опубликуйте опрос.
2. Создайте ручной или k6-трафик.
3. В Prometheus откройте **Graph**, задайте диапазон `15m` и resolution `5s`.
4. Выполните PromQL:

```promql
sum(rate(http_requests_total{route="/api/v1/polls/{id}/votes",method="POST"}[1m]))
```

Это число всех HTTP vote requests в секунду: новые, повторные и отклонённые. Только успешно записанные голоса в секунду:

```promql
sum(rate(votes_recorded_total[1m]))
```

Для короткого burst можно заменить `[1m]` на `[15s]`: граф станет отзывчивее, но менее плавным. Route label нормализован и не содержит реальный poll ID. Если одновременно активен один опрос, граф соответствует его RPS; для нескольких опросов он показывает сумму. Не добавляйте неограниченный `poll_id` label в Prometheus.

Полезные дополнительные запросы:

```promql
histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{route="/api/v1/polls/{id}/votes"}[5m])))
rate(votes_duplicate_total[1m])
rate(votes_rejected_total[1m])
rate(snapshot_error_total[5m])
```

## Проверка

При работающем Compose stack:

```bash
go test ./...
go test -race ./...
go test -tags=integration ./tests/integration/...
k6 run tests/load/vote.js
```

Перед запуском k6 пожалуйста убедить, что у вас стоит переменная окружения `ADMIN_TOKEN` такая, которая в Docker-файле.

```bash
ADMIN_TOKEN=EXAMPLE_TOKEN k6 run ...
```

Integration tests по умолчанию используют PostgreSQL `localhost:5432` и Redis `localhost:6380`; адреса меняются через `INTEGRATION_DATABASE_URL` и `INTEGRATION_REDIS_ADDR`. Redis-тест отправляет 200 конкурентных запросов одного device hash и проверяет ровно один инкремент участника и варианта.

k6 сам создаёт и публикует опрос, если не переданы `POLL_ID` и `OPTION_ID`:

```bash
K6_NO_COOKIES_RESET=true VUS=50 DURATION=30s DUPLICATE_RATIO=0.5 k6 run tests/load/vote.js
```

## Конфигурация

| Переменная | Default | Назначение |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Адрес Go HTTP server |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Доверенный origin для публичных ссылок и QR; с `COOKIE_SECURE=true` требуется HTTPS |
| `DATABASE_URL` | обязательно | PostgreSQL DSN |
| `REDIS_ADDR` | `localhost:6379` | Redis endpoint внутри/вне Compose |
| `REDIS_PASSWORD`, `REDIS_DB` | пусто, `0` | Redis authentication/database |
| `ADMIN_TOKEN` | обязательно | Admin bearer secret, минимум 16 байт |
| `COOKIE_SIGNING_KEY` | обязательно | Cookie HMAC key, минимум 32 байта |
| `DEDUP_HMAC_KEY` | обязательно | Отдельный poll-scoped dedup/rate HMAC key |
| `COOKIE_SECURE` | `false` | Передавать device cookie только по HTTPS |
| `VOTE_BUCKETS` | `64` | Неизменяемое число Redis counter/dedup partitions активного опроса |
| `REDIS_RETENTION` | `24h` | Хранение counters/dedup после окончания |
| `MAX_REDIS_TTL` | `720h` | Серверный предел TTL |
| `POLL_CACHE_TTL` | `30s` | TTL immutable poll definition cache |
| `SNAPSHOT_INTERVAL` | `5s` | Частота PostgreSQL snapshots |
| `VOTE_REQUEST_TIMEOUT` | `2s` | Deadline Redis vote path |
| `RATE_LIMIT_PER_MINUTE` | `120` | Запросы на нормализованную сеть в минуту |
| `TRUST_PROXY_HEADERS` | `false` | Разрешить proxy-derived client address |
| `TRUSTED_PROXY_CIDRS` | пусто | Обязательные proxy/LB сети при включённых proxy headers |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` или `error` |
| `REDIS_PORT` | `6380` | Публикуемый Compose-порт Redis |
| `PROMETHEUS_PORT` | `9090` | Публикуемый Compose-порт Prometheus |
| `SWAGGER_PORT` | `8081` | Публикуемый Compose-порт Swagger UI |

Production secrets не имеют безопасных defaults: замените локальные значения через незакоммиченный `.env`, включите TLS и `COOKIE_SECURE=true`. API отказывается стартовать, если signing/dedup keys совпадают, secrets слишком короткие или `VOTE_BUCKETS` отличается от активного опроса.

## Надёжность и приватность

Redis недоступен — голосование отвечает `503`; fallback-записи в PostgreSQL намеренно нет, чтобы не создавать две несовместимые ветки дедупликации. AOF `appendfsync everysec`, периодические snapshots и финализация уменьшают, но не устраняют окно возможной потери при катастрофической потере Redis.

IP используется только как HMAC нормализованной IPv4 `/24` или IPv6 `/56` сети для rate limiting. Authorization, cookie, сырой IP, device ID/hash, User-Agent и тело голоса не логируются.

Подробнее: [архитектура](docs/architecture.md), [реализационная спецификация](artefacts/IMPLEMENTATION_SPEC_REDIS_GO_POSTGRES.md), [ADR Redis-first](docs/decisions/0001-redis-first-bucketed-votes.md).
