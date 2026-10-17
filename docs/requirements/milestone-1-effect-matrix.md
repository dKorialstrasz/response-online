# M1: матрица EffectSpec pipeline

Связанная задача: [#11](https://github.com/dKorialstrasz/response-online/issues/11).  
Архитектурное решение: [ADR-0007](../adr/0007-effect-resolution-and-pending-choices.md).  
Версии: `effectSpecVersion: effect-spec-v1`, `engineVersion: effect-pipeline-v1`.

Матрица закрывает общий engine pipeline. Она намеренно не переводит 126
карточных EffectSpec из `deferred`: их rulings и conformance — #12.

| Requirement | Реализация | Автоматическое доказательство | Результат #11 |
| --- | --- | --- | --- |
| RULE-05 | Typed mode/target/payment validation внутри clone transaction; влияние и sacrifice refs применяются после полной проверки | `TestAbilityActivationValidatesModesTargetsAndPaymentAtomically`, `TestSacrificePaymentOrderIsCanonical` | Invalid payment откатывается без изменения hash; sacrifice input canonicalized; успешная активация фиксируется целиком |
| RULE-06 | Упорядоченный `EffectQueueEntry`; явные before/after insertion; closed handler registry; replacement и delayed trigger states | `TestExplicitQueueOrderHandlesBothPlayersAndNewEffectsWithoutLIFO`, `TestReplacementDelayedTriggerDurationsAndStaleTargets` | Оба места и новые effects разрешаются в зафиксированном порядке без implicit LIFO |
| RULE-09 | Modifier layer + creation sequence; permanent/end-of-combat/end-of-turn/while-source-valid durations | replacement/duration test и существующий recovery test | Временные modifiers снимаются на границе, остальные пересчитываются поверх base value |
| RULE-10 | Framework не объявляет наличие keywords у конкретной карты | content gate сохраняет 126 EffectSpec `deferred` | Keyword semantics остаются #12 |
| RULE-11 | Любой runtime `AbilityDefinition` требует conformance IDs; репозиторный validator связывает все 126 abilityId с scenario/source hash | `TestBlockedAbilityAndCatalogEvidenceAreEnforced`, `validate_card_definitions.py` | Нет ability без scenario; карточные сценарии пока planned-in-issue-12 |
| RULE-13 | Source/target/payment используют `ObjectRef`; смена зоны увеличивает generation | stale part `TestReplacementDelayedTriggerDurationsAndStaleTargets` | Старый ref fizzle-ится и не воздействует на новый объект |
| RULE-14 | Pending choice сохраняется в State; submit — новая команда; private projection не содержит effect bodies | `TestMandatoryAndOptionalChoicesPersistWithPrivateProjection` | Mandatory/optional выбор переживает commit; owner/opponent видят разные allowlists |
| RULE-15 | Инварианты queue IDs, modifiers, delayed triggers, replacements, choice bounds и action resolution проверяются после событий/команд | engine invariant tests + весь package под race detector | Незавершённый выбор/queue не теряется; finished state не сохраняет resolution |
| OQ-02 | `AbilityBlocked` требует ruling reference и возвращает `ErrAbilityBlocked` | `TestBlockedAbilityAndCatalogEvidenceAreEnforced` | Неоднозначность не закрепляется кодом |
| AC-04 | Queue state входит в canonical JSON/replay hash | `TestPendingChoiceCommandsReplayDeterministically`; queue golden hash `391c8baf6fe1622001dcf73c8e75ef1ee0d560f48ab73c8939d35a5a828d45c5` | Commands с паузой выбора воспроизводятся; synthetic queue fixture детерминирован |

## Проекции pending choice

| Получатель | Поля |
| --- | --- |
| Владелец выбора | choiceId, kind, mandatory, revision, option IDs и object refs; без внутренних effect bodies |
| Соперник, public choice | choiceId, kind, mandatory, revision, option count; без вариантов и effect bodies |
| Соперник, owner-only choice | только waiting/waitingSeat/revision/redacted; без причины, optional/mandatory, IDs и вариантов |

## Открытые ворота

- #12: все реальные EffectSpec, keywords, rulings и executable conformance.
- #14: transport schemas, authorisation и персональный PlayerView.
- #17: UI многошагового выбора и анимация очереди.

