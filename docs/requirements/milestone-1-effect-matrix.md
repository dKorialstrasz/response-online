# M1: матрица EffectSpec pipeline

Связанная задача: [#11](https://github.com/dKorialstrasz/response-online/issues/11).  
Архитектурное решение: [ADR-0007](../adr/0007-effect-resolution-and-pending-choices.md).  
Исторические версии #11: `effectSpecVersion: effect-spec-v1`,
`engineVersion: effect-pipeline-v1`. После #12 текущий engine pin —
`base-abilities-v2`; карточное покрытие вынесено в
[`milestone-1-ability-matrix.md`](milestone-1-ability-matrix.md).

Матрица закрывает общий engine pipeline в историческом результате #11. Все 126
карточных EffectSpec и conformance реализованы последующей задачей #12.

| Requirement | Реализация | Автоматическое доказательство | Результат #11 |
| --- | --- | --- | --- |
| RULE-05 | Typed mode/target/payment validation внутри clone transaction; влияние и sacrifice refs применяются после полной проверки | `TestAbilityActivationValidatesModesTargetsAndPaymentAtomically`, `TestSacrificePaymentOrderIsCanonical` | Invalid payment откатывается без изменения hash; sacrifice input canonicalized; успешная активация фиксируется целиком |
| RULE-06 | Упорядоченный `EffectQueueEntry`; явные before/after insertion; closed handler registry; replacement и delayed trigger states | `TestExplicitQueueOrderHandlesBothPlayersAndNewEffectsWithoutLIFO`, `TestReplacementDelayedTriggerDurationsAndStaleTargets` | Оба места и новые effects разрешаются в зафиксированном порядке без implicit LIFO |
| RULE-09 | Modifier layer + creation sequence; permanent/end-of-combat/end-of-turn/while-source-valid durations | replacement/duration test и существующий recovery test | Временные modifiers снимаются на границе, остальные пересчитываются поверх base value |
| RULE-10 | Framework не объявляет наличие keywords у конкретной карты | Исторически #11 оставлял 126 EffectSpec `deferred`; #12 добавил закрытый registry и keyword interactions | Реализовано в #12 |
| RULE-11 | Любой runtime `AbilityDefinition` требует conformance IDs; репозиторный validator связывает все 126 abilityId с scenario/source hash | `TestBlockedAbilityAndCatalogEvidenceAreEnforced`, `TestBaseAbilityConformance`, `validate_card_definitions.py` | 126 сценариев имеют positive/negative/interaction cases |
| RULE-13 | Source/target/payment используют `ObjectRef`; смена зоны увеличивает generation | stale part `TestReplacementDelayedTriggerDurationsAndStaleTargets` | Старый ref fizzle-ится и не воздействует на новый объект |
| RULE-14 | Pending choice сохраняется в State; submit — новая команда; private projection не содержит effect bodies | `TestMandatoryAndOptionalChoicesPersistWithPrivateProjection` | Mandatory/optional выбор переживает commit; owner/opponent видят разные allowlists |
| RULE-15 | Инварианты queue IDs, modifiers, delayed triggers, replacements, choice bounds и action resolution проверяются после событий/команд | engine invariant tests + весь package под race detector | Незавершённый выбор/queue не теряется; finished state не сохраняет resolution |
| OQ-02 | `AbilityBlocked` требует ruling reference и возвращает `ErrAbilityBlocked` | `TestBlockedAbilityAndCatalogEvidenceAreEnforced` | Неоднозначность не закрепляется кодом |
| AC-04 | Queue state входит в canonical JSON/replay hash | `TestPendingChoiceCommandsReplayDeterministically`; текущий queue golden hash `155bf8f07f80c0d70d28ea88bc983b9112e610b1fc92cb267adb64fee1c2431c` | Commands с паузой выбора воспроизводятся; synthetic queue fixture детерминирован |
| #37 playCard | `playCard` сверяется с server-authored allowlist и запускает только spell/Summon/continuous-enhancement окна | `TestPlayCardValidatesPinnedCostTypePlacementGenerationAndAction`, `TestPlayCardRunsOnlyExactPlayWindowsNotFirstAbility` | Неверная, stale или duplicate команда не меняет hash; стоимость, action и generation фиксируются атомарно |
| #37 real choices | Закрытые continuation handlers строят варианты после draw/reveal/sacrifice и повторно валидируют текущие refs | `TestBaz121DrawThenMandatoryCurrentHandChoiceReplayAndPrivacy`, `TestBaz036ChoiceBelongsToOpponentAndNeverLeaksHandRefs`, `TestPotionDeathTargetIsChosenAfterDrawAndSacrifice` | Actor, mandatory/optional, min/max, order и visibility соответствуют exact text |

## Проекции pending choice

| Получатель | Поля |
| --- | --- |
| Владелец выбора | choiceId, kind, mandatory, revision, option IDs и object refs; без внутренних effect bodies |
| Соперник, public choice | choiceId, kind, mandatory, revision, option count; без вариантов и effect bodies |
| Соперник, owner-only choice | только waiting/waitingSeat/revision/redacted; без причины, optional/mandatory, IDs и вариантов |

В protocol 1.2 `mandatory`, `minSelections`, `maxSelections` и `orderMatters`
являются nullable-by-absence: владелец и public observer получают явные значения
включая `false`/`0`, redacted observer не получает сами поля.

## Открытые ворота

- #12: выполнено в `base-abilities-v1`; #37 поднимает runtime pin до
  `base-abilities-v2` без изменения закреплённого effectSpecVersion.
- #14: transport schemas, authorisation и персональный PlayerView.
- #17: UI многошагового выбора и анимация очереди.

