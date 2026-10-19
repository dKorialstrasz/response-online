# M1: матрица reconnect, технических исходов и rematch

Roadmap: [M1-P13 / #15](https://github.com/dKorialstrasz/response-online/issues/15).
Архитектура: [ADR-0010](../adr/0010-match-lifecycle-reconnect-rematch.md).

| Требование | Реализация | Evidence |
| --- | --- | --- |
| FR-19, FR-25 | `surrender`, `disconnect_forfeit`, `aborted_technical` используют один immutable engine result | `TestConcurrentTerminalPathsCommitExactlyOneResult`, `TestCompetingDisconnectAndIdleTimeoutsCommitOneResult` |
| FR-22–26, NET-05–06 | heartbeat 1 с, detection 5 с, окно 20 с, budget 60 с; reconnect и timeout проходят mailbox | `TestRuntimePolicyAndReconnectBoundaries`, `TestReconnectAtDeadlineLosesDeterministically`, `TestReconnectAfterDeadlineLoses`, `TestRepeatedDisconnectsConsumeSixtySecondBudget` |
| FR-27 | ожидание только arrangement/mulligan/action/choice; warning 30 с, abort 90 с; reset только accepted action | `TestIdleWarningAndTimeoutUseApprovedTexts`, `TestAcceptedActionWinsBeforeIdleDeadlineButNotAtDeadline` |
| NET-08, SEC-06/09 | resume строит новый персональный `PlayerView`, command cache остаётся в scope seat/match/commandId | transport integration/security tests и schema validation |
| FR-28, UC-10 | rematch имеет отдельные ready обоих seats, новый `matchId` и immutable history result | `TestRematchRequiresBothPlayersAndPreservesPriorResult` |
| OQ-03 | lifecycle events содержат только восемь утверждённых русских строк | constants и boundary tests application package |
| OQ-04 | state остаётся in memory; crash recovery не добавлен | ADR-0001, ADR-0010 |

## Scope boundary

Реализация не добавляет общий игровой таймер, автопас, новые карточные правила,
durable recovery, staging/production или GamePix. Пользовательская приёмка,
зелёный CI и отдельное разрешение merge остаются обязательными.

