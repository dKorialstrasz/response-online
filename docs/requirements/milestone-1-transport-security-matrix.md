# M1: матрица HTTP/WSS и security boundary

Версия протокола: `1.1`. Архитектурное решение:
[ADR-0008](../adr/0008-http-ws-security-boundary.md). Roadmap issue:
[#14](https://github.com/dKorialstrasz/response-online/issues/14).

| Требование | Реализация | Проверка / evidence |
| --- | --- | --- |
| NET-01–02 | HTTP session/lobby, first-message WS auth, cookie + CSRF + exact Origin | `TestSessionCookieCSRFAndOriginBoundary`, OpenAPI |
| NET-03–04 | 12 command payload schemas, 11 server event types, heartbeat вне game revision; принятый ход рассылает персональные views обоим seats | WS JSON Schema, schema-validated examples, `TestAcceptedCommandBroadcastsPersonalizedStateToBothSeats`, Go/TypeScript parity validator |
| NET-05–06 | match owner, expectedRevision, payload fingerprint и command result cache | `TestCommandReplayAndInvalidReferenceDoNotMutateState`, engine/application race tests |
| NET-07 | закрытый enum ошибок и безопасные `messageKey` без внутренних error strings | OpenAPI/WS parity, HTTP adversarial tests |
| NET-08 | `resync(lastEventSeq)` всегда возвращает полный персональный snapshot | WS command test; reconnect budgets/outcomes остаются #15 |
| NET-09 | OpenAPI 3.1.1, JSON Schema 2020-12, examples, TS parity surface | `scripts/validate_contracts.py` |
| NET-10–11 | отдельный `PlayerView`, HMAC object refs, count-only чужая рука/колода | `TestPlayerViewRedactsHiddenStateAndUsesOpaqueReferences` |
| SEC-01–03 | loopback-only committed server, configurable exact origins, host-only cookie, CSRF | ADR-0008, runbook, HTTP tests |
| SEC-04–05 | настраиваемые defaults: 64 KiB body, 32 KiB frame, depth 16, create/join и WS limits; strict fields | `TestHTTPBodyAndUnknownFieldsAreRejected`, `TestConfiguredCreateRoomRateLimit`, `FuzzStrictWebSocketDecode` |
| SEC-06 | connection generation + seat binding; private/internal refs rejected before engine | command adversarial test, application room race tests |
| SEC-07 | JSON only, `nosniff`, restrictive API CSP; UI never receives HTML payload | HTTP header tests/manual audit |
| SEC-09 | public API contains no audit endpoint; trace/token fields absent from serializers | PlayerView leak test, network audit |
| SEC-10 | locked Go/Node dependencies, verify/audit CI | `make dependencies`, Application CI security job |
| AC-06–07 | invalid/replay/stale operations do not mutate state; hidden state absent | [transport security report](../reports/m1-transport-security.md) |
| #37 playCard | `sourceRef` включает generation; typed card kind, placement, sacrifice/Magic Shield refs и X-payment декодируются строго; `legalActions` содержит точные server allowlists | `TestPlayCardPayloadRequiresCurrentGenerationRef`, engine play-card tests, WS schema/examples |
| #37 choice privacy | owner-only projection не содержит для другого seat choiceId, kind, mandatory/optional, bounds, options, refs или continuation; public projection не содержит raw options/effects | `TestOwnerOnlyChoiceAndLegalActionsDoNotLeakForeignHandOrContinuation`, baz-036/baz-121 engine tests |

## Граница соседних задач

`requestRematch` использует отдельный ready каждого игрока и реализован в #15.
Lobby-команды выполняются по HTTP; их WS-имена сохранены
для полной клиентской taxonomy и не обходят cookie/CSRF boundary. Полный gameplay
`playCard` в protocol 1.2 исполняется авторитетно из закреплённых CardDefinition и
AbilityDefinition. Клиент остаётся projection-only и отправляет только значение
из текущего allowlist.
