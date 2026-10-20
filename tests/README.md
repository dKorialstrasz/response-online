# Тесты

Unit-тесты находятся рядом с Go/TypeScript кодом. `tests/content` проверяет
CardDefinition, field-level evidence, ability/scenario links, различение
unknown/null/zero, права на изображения и транзитивное замыкание. Каталог
также проверяется JSON Schema и единым release gate: негативные fixtures
подтверждают блокировку неполных полей, неверной арифметики, hash drift,
самовольного изменения прав и преждевременной маркировки gameplay-ready.
Каталог `smoke` содержит чёрный ящик для локального диагностического API. Исполняемые
gameplay conformance, integration и E2E наборы добавляются задачами, которые
реализуют соответствующую функциональность; scaffold не подменяет их пустыми
тестами.

Issue #37 добавляет engine/transport coverage реального `playCard`,
mandatory/optional и opponent-owned choices, post-draw allowlists, stale,
duplicate и foreign refs, replay/resync и adversarial privacy обеих seats.
Таблица покрытия находится в
[`docs/reports/issue-37-play-card-choice-audit.md`](../docs/reports/issue-37-play-card-choice-audit.md).

Issue #15 добавляет virtual-clock и race-проверки lifecycle рядом с
`backend/internal/application`: границы heartbeat/detection/reconnect/budget и
idle 1/5/20/60/30/90 секунд, повторные disconnect, accepted-action/timeout,
reconnect/timeout, единственный terminal result и rematch history. Transport
набор повторно проверяет command replay и персональный `PlayerView` после
аутентифицированного resume.

