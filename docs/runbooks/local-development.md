# Локальная разработка M1

Runbook относится только к локальной standalone-сборке. Он не описывает staging,
production, TLS, GamePix или публичный доступ.

## Закреплённые инструменты

- Go 1.26.9 — `backend/go.mod`.
- Node.js 24.x — `frontend/package.json` и Application CI.
- pnpm 11.25.0 — `frontend/package.json` и Application CI.
- Python 3.11+ — проверки схем и manifests используют только standard library.

## Первый запуск

```bash
make bootstrap
cp .env.example .env
make check
```

`.env` загружается только целью `make dev-backend` и игнорируется Git. Все поля
профиля документированы в `.env.example`; пустой `.env` оставляет встроенные
локальные defaults. Не добавляйте в файл session cookies, WebSocket tickets или
другие runtime credentials.

Запустите backend и frontend в отдельных терминалах:

```bash
make dev-backend
make dev-frontend
```

Откройте `http://127.0.0.1:5173`. Vite передаёт локальные `/api` запросы на
`127.0.0.1:8080`. Адрес backend можно изменить только для локального процесса:

```bash
RESPONSE_HTTP_ADDR=127.0.0.1:18080 make dev-backend
```

## Диагностика

- `GET http://127.0.0.1:8080/healthz` — процесс жив.
- `GET http://127.0.0.1:8080/readyz` — процесс принимает локальные запросы.
- `GET http://127.0.0.1:8080/api/v1/build` — commit и закреплённые версии.
- `GET http://127.0.0.1:8080/metrics` — только обезличенные process-local
  counters: комнаты/матчи, reject codes, HTTP latency buckets, reconnect и
  technical abort.
- `bash tests/smoke/local_backend.sh` — чёрный ящик диагностического API.

Backend пишет по одной JSON-записи на HTTP-запрос. Запись содержит только
method, route pattern, status, duration и correlation ID. Query string, IP,
Cookie/Authorization, request/response body, room/match ID, карты руки, seed и
WebSocket ticket не журналируются. Не передавайте секреты в `X-Correlation-Id`.
Локальные журналы следует удалять не позднее 7 дней; закрытые defect traces
также хранятся максимум 7 дней и не публикуются. Обезличенные агрегаты допустимо
хранить до 30 дней.

Быстрая проверка:

```bash
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:8080/metrics
curl --fail http://127.0.0.1:8080/api/v1/build
```

`readyz` меняется на HTTP 503 со статусом `draining` после начала controlled
shutdown, а `healthz` отвечает, пока жив HTTP-процесс.

`rulesVersion: response-rules-2026-10-17` и
`engineVersion: base-abilities-v2` относится к базовому автомату #10,
типизированному pipeline #11, закрытому каталогу 126 способностей #12 и
server-authoritative playCard/PendingChoice #37.
Русский card text не исполняется; матч получает только version-pinned handlers.
`protocolVersion: 1.2` относится к HTTP/WSS contracts, security boundary,
generation-safe refs, legalActions и PlayerView. Lobby работает по HTTP JSON;
активный матч — по WebSocket.
Reconnect outcomes/rematch остаются #15.

Локальный allowlist по умолчанию разрешает origins `127.0.0.1:5173`,
`localhost:5173`, `127.0.0.1:8080` и `localhost:8080`. Другой точный список
задаётся через `RESPONSE_ALLOWED_ORIGINS` (через запятую). Loopback HTTP запускает
cookie без `Secure`; для любой TLS-среды обязательно установить
`RESPONSE_SECURE_COOKIES=true`. Публичная среда не входит в M1.

Стартовые security limits можно уменьшать или увеличивать положительными целыми
значениями окружения; отсутствие переменной сохраняет утверждённый M1 default:

| Переменная | Default |
| --- | ---: |
| `RESPONSE_CREATE_ROOMS_PER_SESSION` | 5/мин |
| `RESPONSE_CREATE_ROOMS_PER_IP` | 20/мин |
| `RESPONSE_JOINS_PER_SESSION` | 10/мин |
| `RESPONSE_JOINS_PER_IP` | 30/мин |
| `RESPONSE_WS_COMMANDS_PER_SECOND` | 20/с |
| `RESPONSE_WS_COMMAND_BURST` | 40 |
| `RESPONSE_HTTP_BODY_BYTES` | 65536 |
| `RESPONSE_WS_FRAME_BYTES` | 32768 |
| `RESPONSE_JSON_DEPTH` | 16 |

Отрицательные и нулевые явные значения считаются ошибкой конфигурации.

## Сборка и manifest

```bash
make manifest
```

Команда сначала выполняет `make check` и `make smoke`, затем создаёт игнорируемые
Git артефакты в `build/artifacts/` и `build/build-manifest.json`. Manifest
содержит commit, состояние source tree (`clean` либо `diffSha256` для честно
помеченной локальной dirty-сборки), protocol/content/deck/effect/rules/engine versions, SHA-256,
локальный QA status и идентификаторы GitHub Actions run (когда запущен в CI).
Build job стартует только после backend/frontend/contracts/security/smoke jobs;
их prerequisite status также фиксируется в manifest. CI загружает каталог как
immutable artifact конкретного workflow run.

## Проверки перед PR

```bash
make check
make dependencies
bash tests/smoke/local_backend.sh
```

Для #37 backend tests дополнительно поднимают настоящий HTTP/WebSocket handler и
проверяют `playCard`, mandatory/optional PendingChoice и resync без test-only
payload shape или клиентского вычисления legal actions.

`make dependencies` обращается к registry для `pnpm audit`. Secret scan работает
локально и не отправляет исходники внешнему сервису.

## Контролируемое завершение

`Ctrl+C` отправляет процессу SIGINT. То же поведение даёт `kill -TERM <pid>`.
Backend атомарно закрывает создание/присоединение к комнатам, переводит readiness
в `draining`, через single-writer каждого незавершённого матча фиксирует
`aborted_technical` без победителя, пишет агрегированный shutdown report и затем
останавливает HTTP/runtime. Общий deadline — 10 секунд; ненулевой
`failed_operations` завершает процесс с ошибкой.

Это технический исход текущего процесса, а не игровой результат. Он виден в
`response_technical_aborts_total` до остановки процесса и в JSON shutdown log.

## Зависший матч и повторный запуск

1. Сохраните `/api/v1/build`, `/metrics`, время и correlation ID проблемной
   команды. Не сохраняйте cookies, tickets, руки или seed.
2. Убедитесь, что процесс жив (`healthz`) и принимает комнаты (`readyz`). Если
   HTTP не отвечает, зафиксируйте crash как потерю всех in-memory матчей.
3. Для зависшего локального матча завершите весь standalone-процесс через
   `Ctrl+C` или `SIGTERM`. В M1 нет публичного per-room administrative endpoint:
   это исключает управление закрытым trace по угадываемому room ID.
4. Проверьте JSON shutdown report: `technical_aborts` должен включать
   незавершённый матч, `failed_operations` должен быть 0.
5. Повторно выполните `make dev-backend`; затем проверьте health/readiness/build
   и запустите frontend при необходимости.

При crash/restart in-memory состояние теряется согласно ADR-0001. Не обещайте
recovery и не приписывайте потерянной партии игровой результат.

## Длительный прогон

Длительность и точный профиль soak утверждаются владельцем в test plan #21 до
запуска. Пока #21 открыт без утверждённого плана, нельзя выбирать длительность
самостоятельно или отмечать этот evidence как пройденный. После утверждения
запишите commit, manifest SHA-256, параметры стенда, команды, начало/окончание,
crash/panic/leak и итог в `docs/reports/`; запускать нужно immutable артефакт из
`make manifest`, а не смешивать backend/frontend разных commits.

