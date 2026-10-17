# Локальная разработка M1

Runbook относится только к локальной standalone-сборке. Он не описывает staging,
production, TLS, GamePix или публичный доступ.

## Закреплённые инструменты

- Go 1.22.2 — `backend/go.mod`.
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
`engineVersion: deterministic-core-v1` относятся к базовому автомату #10.
Они не означают готовность карточных EffectSpec: эти ворота остаются в #11/#12.
`protocolVersion: scaffold-v1` сохраняется до сетевого контракта gameplay.

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

`make dependencies` обращается к registry для `pnpm audit`. Secret scan работает
локально и не отправляет исходники внешнему сервису.

## Контролируемое завершение

`Ctrl+C` отправляет процессу SIGINT; backend прекращает приём запросов и ждёт до
10 секунд. Комнат и матчей в scaffold пока нет. После их появления shutdown должен
помечать незавершённые матчи `aborted_technical` по OPS-03; это не считается
реализованным данной задачей.

При crash/restart in-memory состояние теряется согласно ADR-0001. Не обещайте
recovery и не приписывайте потерянной партии игровой результат.

