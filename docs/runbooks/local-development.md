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
make check
```

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
- `bash tests/smoke/local_backend.sh` — чёрный ящик диагностического API.

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

Команда создаёт игнорируемые Git артефакты в `build/artifacts/` и
`build/build-manifest.json`. Manifest содержит commit, версии и SHA-256. CI загружает
этот каталог как immutable artifact конкретного workflow run.

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

`Ctrl+C` отправляет процессу SIGINT; backend прекращает приём запросов и ждёт до
10 секунд. Controlled room/match shutdown остаётся #18. После его появления
shutdown должен помечать незавершённые матчи
`aborted_technical` по OPS-03; это не считается реализованным задачей #14.

При crash/restart in-memory состояние теряется согласно ADR-0001. Не обещайте
recovery и не приписывайте потерянной партии игровой результат.

