# Self-check: issue #17 — игровой UI UC-05–12

Дата: 2026-10-20. Ветка: `feature/17-gameplay-ui`.

## Реализовано

- Постоянная шапка показывает фазу, ход, инициативу, действующего игрока,
  оставшиеся действия и состояние соединения.
- PlayerView protocol 1.2 содержит только allowlisted публичные `queue` и
  `journal`; поле показывает пять линий, публичные карты, Приказы, кладбища,
  собственную руку и только счётчики закрытых зон соперника.
- `playCard` и `activateAbility` собираются исключительно из server-authored
  `legalActions`: режим, цель, линия/слот, оплата, просмотр команды, явная отправка
  и локальная отмена. После отправки отмена хода не предлагается.
- Каждая команда получает новый `commandId` и текущий `expectedRevision`. Stale
  revision, истёкший choice и разрыв порядка блокируют действия и требуют свежий
  snapshot.
- Mandatory/optional PendingChoice имеет серверные min/max/options, локальный
  сброс и подтверждение. «Не активировать» существует только при `minSelections=0`.
  Решение выводится выше анимаций и не зависит от них.
- Очередь/анимации — presentation layer; доступны обычная, ускоренная и выключенная
  анимация. Журнал читается независимо и сохраняет серверный порядок событий.
- Zoom, справка, focus-visible, keyboard и touch не требуют hover или drag-and-drop.
- Реализованы точные OQ-03 connection/idle строки, background resync, раздельные
  русские состояния invalid/expired room, full room, version mismatch,
  unavailable server, session loss и protocol error.
- Сдача требует подтверждения. Result semantics приходят от сервера; после
  результата действия заблокированы. Два согласия на rematch создают новый матч.
- Исправлена обнаруженная E2E серверная регрессия `baz-009`: исходная линия
  разыгрываемого юнита больше не отклоняет его optional post-summon choice.

## Privacy и authority review

- Клиент не вычисляет легальность, эффекты, победителя или скрытое состояние.
- Raw MatchState, seed, закрытая рука, скрытые object refs и event Detail не входят
  в PlayerView/WS schema и не рендерятся.
- Queue исключает эффекты с непубличным источником; journal строится только из
  allowlist типов и русских шаблонов, не передаёт Detail, внутренние ID и сам
  факт private choice.
- Adversarial transport tests проверяют скрытый источник, приватный choice,
  отсутствие null-массивов и отсутствие закрытых деталей.
- Более старые revision/eventSeq не заменяют свежую проекцию. Повторный commandId
  обрабатывается серверной идемпотентностью; UI не симулирует optimistic result.

## Автоматические проверки

- Unit/integration: reducer/order, command composition/cancel, choices, privacy,
  stale/resync, result/rematch, русские error states и WebSocket rematch broadcast.
- Go regression: полный `baz-009` play → optional choice → decline.
- Browser E2E: реальный backend, две независимые сессии, mandatory и optional
  choice, privacy, journal/order, reconnect, background return, surrender/result,
  rematch, keyboard, touch и viewport 1366×768, 640×480, 360×640, 640×360.
- Подробное browser evidence: `docs/reports/evidence/issue-17/README.md` и
  `browser-e2e.json`.

Фактически выполнено и прошло:

- `make check-go` (включая `go test -race ./...`);
- frontend Prettier, ESLint, TypeScript, 26 Vitest tests и production build;
- `make check-data` (contracts, 13 content tests, decks/cards/release, docs,
  secrets);
- `make dependencies` (`go mod verify`, govulncheck: уязвимости не найдены,
  licenses, pnpm audit: уязвимости не найдены);
- `make m1-release-check`, `make smoke` и двухсессионный browser E2E.

`make manifest` выполняется после создания финальных коммитов, чтобы manifest
содержал их SHA.

## Известные границы

- Browser E2E выполнен в Chromium 151.0.7922.34. Touch эмулирован Chromium;
  физические устройства, Firefox, Safari и Edge не проверялись.
- В production bundle Vite может предупреждать о chunk больше 500 kB до gzip;
  это предупреждение производительности, не ошибка сборки.
- Публичное развёртывание, iframe/GamePix и вопросы OQ-02 вне утверждённых
  server-provided действий остаются вне scope этой задачи.
