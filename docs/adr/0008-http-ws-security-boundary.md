# ADR-0008: HTTP/WSS security boundary и opaque PlayerView refs

- Статус: принято к проверке
- Дата: 18 октября 2026 года
- Решение: реализация утверждённых требований M1; пользовательская приёмка владельца обязательна
- Связанные требования: NET-01–11, SEC-01–07, SEC-09–10, AC-06–07
- Связанная задача: [#14](https://github.com/dKorialstrasz/response-online/issues/14)
- Учитывает: [ADR-0004](0004-protocol-and-player-views.md),
  [ADR-0006](0006-anonymous-sessions-and-rooms.md)

## Контекст

Application service хранит digest анонимного секрета и одноразового WS-ticket, а
engine содержит seed, внутренние instance IDs, порядок колод, приватные руки,
выборы и audit trace. Transport должен публиковать HTTP/WSS API, не превращая
структуры application/engine в сетевой контракт и не разрешая клиенту выбирать
seat, итог или внутреннюю ссылку.

## Решение

- Protocol `1.0` описан OpenAPI 3.1 и JSON Schema 2020-12. HTTP обслуживает
  сессию/лобби, WSS — снимки активного матча и игровые команды. Полные
  персональные снимки используются вместо delta.
- Recovery secret находится только в host-only `HttpOnly`, `SameSite=Strict`
  cookie без `Domain`. `Secure` обязателен вне явно настроенного loopback
  development. Мутирующий cookie-auth HTTP требует HMAC CSRF token.
- Любой API/upgrade требует точного `Origin` из allowlist. Пустой и неизвестный
  Origin отклоняется; `X-Forwarded-For` не считается доверенным источником IP.
- Одноразовый 30-секундный ticket передаётся только первым WS JSON-сообщением за
  пять секунд. Query string на upgrade запрещён. После redeem transport хранит
  только connection lease; новое поколение немедленно инвалидирует старое.
- `PlayerView` строится отдельным allowlist serializer. Клиентский object ref —
  HMAC-псевдоним текущего `zoneObjectId/generation`; он меняется при переходе
  зоны и не содержит `instanceId`, seed или позицию карты. Входящий ref
  разрешается только поиском по текущему состоянию.
- Command cache имеет scope room/seat/match/commandId. Точный повтор возвращает
  прежний результат, другой payload даёт `COMMAND_ID_REUSED`; записи живут весь
  матч и 15 минут после завершения.
- HTTP body ограничен 64 KiB, WS frame — 32 KiB, JSON depth — 16. Неизвестные
  поля запрещены. Create/join ограничены одновременно по сессии и socket IP;
  WS использует token bucket 20 команд/с с burst 40. Запись медленному клиенту
  имеет deadline.
- Ошибки возвращают только стабильный code/retryable/correlationId/messageKey.
  Stack trace, внутренний error и закрытая причина легальности не сериализуются.

## Последствия

- Внутренние Go-типы можно менять без неявного protocol break.
- UI обязан хранить CSRF token только в памяти страницы и использовать
  опубликованные opaque refs; восстановление места по room code невозможно.
- Loopback HTTP остаётся допустимым только для локального M1. Любая публичная
  среда требует HTTPS/WSS, `Secure` cookie и отдельный operations gate.
- Gorilla WebSocket является единственной новой runtime-зависимостью; версия
  закреплена в `go.mod/go.sum` и проходит dependency verification.

## Рассмотренные альтернативы

- Ticket в query string отклонён из-за утечки через URL и access logs.
- Cookie без CSRF и отключение Origin ради будущего iframe отклонены.
- Передача engine `State` с полями `hidden` отклонена: поле уже покинуло
  security boundary и может попасть в devtools/log.
- Публичный `zoneObjectId` engine отклонён: текущая форма содержит стабильный
  внутренний instance ID и позволяет коррелировать закрытый объект.
