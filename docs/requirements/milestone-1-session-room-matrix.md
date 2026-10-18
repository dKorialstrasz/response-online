# Матрица сессий и комнат M1

Связанная задача: [#13](https://github.com/dKorialstrasz/response-online/issues/13).
Архитектурное решение: [ADR-0006](../adr/0006-anonymous-sessions-and-rooms.md).

Матрица фиксирует реализацию application boundary. Публичные HTTP/WSS contracts,
cookie/CSRF/Origin, rate limits и PlayerView реализованы в #14; reconnect и технические
исходы — #15; frontend UC-01–03 — #16.

| Требование | Реализация | Автоматическое доказательство |
| --- | --- | --- |
| FR-01, FR-02, SEC-02 | `RoomService.CreateSession`: 256-bit secret, digest-only storage, sliding 24h TTL; room code не аутентифицирует место | `TestAnonymousSessionsAndRoomCodesUseSeparateSecrets`, `TestSuccessfulAuthenticationExtendsSessionTTL` |
| FR-03 | Один mutex для create/join, session-scoped idempotency, ровно два места | `TestCreateAndJoinAreIdempotentAndAtomic` и Go race detector |
| FR-04 | 13 символов RFC 4648 base32, crypto entropy, collision check, без room listing | `TestAnonymousSessionsAndRoomCodesUseSeparateSecrets` |
| FR-05 | Application snapshot содержит code, но не session secret; invitation URL формирует #16 | JSON leak assertion в `TestAnonymousSessionsAndRoomCodesUseSeparateSecrets` |
| FR-06 | `SetSelection` сбрасывает ready; `SetReady` staged commit; release hashes, versions и manifests закрепляются до active | `TestSelectionResetsReadyAndStartPinsOneAtomicMatch`, `TestFailedStartCommitsNoPartialReadyOrMatch`, `TestFileMatchPinProviderLoadsApprovedReleaseAndAllowsMirrors` |
| FR-07 | Две действующие сессии, оба выбора/ready, manifest validation; одинаковые deckId допустимы | mirror/start tests в `room_service_test.go` и `content_pins_test.go` |
| FR-08 | Места равноправны; `LeaveRoom` очищает только своё место | `TestRoomTTLsAndCleanupWakePendingRequests` и room service invariants |
| FR-09 | 15m empty / 30m inactive lobby; active match исключён; waiters получают expired | `TestRoomTTLsAndCleanupWakePendingRequests`, `TestActiveMatchIsNotDeletedByLobbyTTL` |
| FR-10, SEC-02 | 30s one-time ticket; connection generation; новый lease вытесняет старый; revoke/expiry прекращают доступ | `TestOneTimeTicketsAndConnectionReplacement` |
| UC-01–03, AC-03 (backend slice) | Две сессии создают/занимают room, выбирают manifests/orders и стартуют; конкурентный третий получает full | integration/concurrency tests `room_service_test.go` |

## Threat-focused review

| Угроза | Контроль | Остаток/следующая задача |
| --- | --- | --- |
| Захват места по коду | Отдельный 256-bit session secret; code только занимает свободное место | Перебор ограничивает #14 по SEC-04 |
| Race третьего join или двух ready | Одна транзакция admission/start; negative path не фиксирует state | Load/fault profiling — #21 |
| Replay create/join/ticket | Idempotency fingerprint; ticket удаляется до выдачи lease | HTTP command cache — #14 |
| Утечка secret/seed/opponent choice | Digest-only storage и allowlisted `RoomSnapshot`; нет raw engine state | Transport serialization audit — #14 |
| Команда старого соединения | Exact connectionId+generation authorization; replacement/revoke немедленно инвалидируют lease | Фактическое закрытие WS — #14/#15 |
| Подмена manifest после ready | SHA-256 при startup и immutable копия обоих manifests/versions в start result/engine config | Release/content gate продолжает блокировать незавершённые EffectSpec #11/#12 |
| Различение room states раскрывает секрет | Требуемые safe categories invalid/expired/full/session-loss без session/seat identifiers | Rate limit и локализованные messageKey — #14 |
