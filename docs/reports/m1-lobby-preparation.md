# Lobby, выбор колоды и подготовка матча — evidence #16

Дата self-check: 2026-10-19. Scope: локальная M1-реализация FR-05–07,
FR-11–16, UC-01–04 и NFR-05–06. Gameplay UI из #17, GamePix, iframe и
публичный deployment не входят в этот отчёт.

## Реализованный путь

- При открытии создаётся анонимная сессия; recovery secret остаётся только в
  host-only HttpOnly SameSite cookie действующего backend-контракта, а CSRF token
  живёт только в памяти вкладки.
- Можно создать приватную комнату или присоединиться по нормализованному коду из
  поля/ссылки. Invitation строится заново и содержит единственный query-параметр
  `room`; поля кода и ссылки остаются доступными для ручного выделения без
  Clipboard API.
- Клиент показывает ровно шесть approved manifests из `content/decks/base` и
  source-matched owner-approved поля из `CardDefinition`. Неподтверждённые
  числовые поля не превращаются в нули или вымышленные значения.
- Выбор отправляет `deckId` и перестановку пяти `orderIds`. Mirror match
  разрешён. Порядок меняется кнопками или клавиатурой, без обязательного
  drag-and-drop. Любое изменение selection получает свежий room snapshot, где
  backend автоматически снимает ready.
- После ready обоих игроков клиент получает одноразовый WebSocket ticket по
  защищённому HTTP endpoint и отправляет его только первым WebSocket frame, без
  query string. Выбранный порядок фиксируется существующей
  `setOrderArrangement`, после чего показываются server-authoritative initiative
  и собственная начальная рука.
- Mulligan отправляет только opaque `objectRef` собственных выбранных карт.
  Состояние ожидания показывает лишь публичный `handCount` соперника и не читает
  его `hand`, выбор или скрытые идентификаторы. После решения обоих игроков
  отображается минимальное завершение подготовки без реализации #17.
- Lobby обновляется безопасным polling `GET /rooms/{roomId}`. Локальный Vite
  proxy восстанавливает точный allowlisted Origin для same-origin GET, у которого
  браузер не формирует заголовок Origin, и отдельно проксирует WebSocket upgrade.

## Accessibility notes

- Все основные действия доступны обычными кнопками/полями с программными
  labels; динамические ошибки, подключение, ожидание и перестановка объявляются
  через `role=status`, `role=alert` или `aria-live`.
- Интерактивные цели имеют высоту около 44 CSS px. `:focus-visible` даёт
  контрастный трёхпиксельный контур; состояние не передаётся только цветом — рядом
  всегда есть текст (`Готов`, `Ожидаем`, `Связь установлена`, `Выбрана`).
- Приказы можно поднять/опустить отдельными кнопками или `Alt+↑/↓`; изменение
  объявляет новое место. Просмотр состава и увеличенной карточки открывается
  нажатием/касанием, а не hover. Диалог получает начальный focus и закрывается
  кнопкой, кликом по фону или Escape.
- Поля invitation read-only, но фокусируемы и выделяют значение для ручного
  копирования. Кнопка Clipboard — только progressive enhancement и скрыта при
  отсутствии API.
- Компоновка вручную просмотрена в 640×480, 360×640, 640×360 и 1366×768.
  Узкие/низкие окна используют вертикальную прокрутку, не скрывая обязательные
  действия.
- Учитывается `prefers-reduced-motion`; анимация загрузки сокращается системной
  настройкой.

## Privacy и security review

- В URL отсутствуют session secret, CSRF token, WebSocket ticket и connection ID.
- Код не использует `localStorage`/`sessionStorage` и не пишет credentials в
  console. Ticket очищается из локальной переменной сразу после auth frame.
- UI соперника использует только allowlisted `PlayerView`; тест дополнительно
  передаёт в компонент искусственное лишнее opponent `hand` и подтверждает, что
  имя и opaque reference не попадают в HTML.
- Controlled reconnect проверен заменой активного соединения вторым валидным
  ticket той же сессии: UI показал восстановление, получил новый ticket и вернулся
  к свежему personal snapshot.

## Tracked evidence

- [Browser E2E result](evidence/issue-16/e2e-results.json) — два независимых
  игрока на реальном HTTP/WebSocket backend, третий игрок, mirror match,
  keyboard reorder, ready reset, initiative, mulligan privacy, reconnect и
  fallback без Clipboard API.
- [1366×768](evidence/issue-16/lobby-1366x768.png)
- [640×480](evidence/issue-16/lobby-640x480.png)
- [360×640](evidence/issue-16/lobby-360x640.png)
- [640×360](evidence/issue-16/lobby-640x360.png)
- [Private mulligan waiting, 1366×768](evidence/issue-16/mulligan-private-waiting-1366x768.png)

Screenshot содержат только случайные room codes и invitation с room code; ни
один credential в изображение не попал.

## Проверки

Все обязательные команды выполнены на ветке `feature/16-lobby-preparation`:

| Проверка | Результат |
| --- | --- |
| `pnpm --dir frontend format:check` | passed |
| `pnpm --dir frontend lint` | passed |
| `pnpm --dir frontend typecheck` | passed |
| `pnpm --dir frontend test` | 3 files, 12 tests passed |
| `pnpm --dir frontend build` | passed; 98.63 kB gzip JS, non-blocking 500 kB raw chunk warning |
| Browser E2E, real backend | passed; tracked matrix above |
| `make check` | passed, включая Go race tests, contracts, 13 content tests, docs и secret scan |
| `make dependencies` | passed; `govulncheck` и `pnpm audit` не нашли известных уязвимостей |
| `bash tests/smoke/local_backend.sh` | passed |
| `make manifest` | passed; локальные ignored build artifacts созданы |

## Ограничения

- Перезагрузка вкладки создаёт новую анонимную сессию: контракт не предоставляет
  безопасный endpoint восстановления CSRF token для существующей HttpOnly
  сессии. Reconnect внутри открытой вкладки поддержан; обещание recovery после
  reload не заявляется.
- После завершения mulligan показан только минимальный экран окончания
  подготовки. Поле, ходы и карточные действия относятся к #17.
- Публичная эксплуатация, TLS-профиль, права на материалы и GamePix остаются за
  пределами локальной M1-проверки.
