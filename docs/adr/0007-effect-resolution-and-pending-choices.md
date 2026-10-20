# ADR-0007: Типизированное разрешение эффектов и устойчивые решения

- Статус: принято к проверке владельцем
- Дата: 18 октября 2026 года
- Решение: реализация для review владельцем проекта dKorialstrasz
- Связанные требования: RULE-05–06, RULE-09–11, RULE-13–15, OQ-02, AC-04
- Связанная задача: [#11](https://github.com/dKorialstrasz/response-online/issues/11)
- Учитывает: [ADR-0003](0003-serialized-match-execution.md), [ADR-0005](0005-deterministic-match-core.md)

## Контекст

Ядро #10 фиксирует транзакционную команду, детерминированный replay и базовый
цикл партии, но не определяет исполняемую семантику карточек. Для #11 нужна
общая модель modes, targets, payments, очереди, choices, replacements, delayed
triggers и durations. Она не должна превращать очередь в неописанный LIFO и не
может подменять rulings конкретных 126 способностей, остающихся в #12.

## Решение

`AbilityDefinition` — вход доверенного version-pinned каталога. Каждая запись
обязана иметь `abilityId`, статус `ready` либо `blocked`, и хотя бы один
`conformanceScenarioId`. `blocked` требует ссылку на OQ/ruling и не исполняется.
Каталог глубоко копируется при создании матча, поэтому изменение входных срезов
после старта не меняет replay.

Активация выполняется внутри обычной clone/apply/validate/commit транзакции:

1. Проверяются действующее место, источник, выбранные modes и target refs.
2. Полностью проверяется payment; влияние и жертвы применяются только к клону.
3. Инструкции превращаются в `ResolvedEffect` с generation-bound ссылками.
4. Эффекты получают монотонные entry/sequence IDs и явную позицию
   `before_existing` либо `after_existing`.
5. Закрытый handler registry исполняет только детерминированные primitives.

Очередь не имеет неявной stack-семантики. Порядок верхнеуровневых и выбранных
mode effects следует за явно сохранённым порядком входа. Эффект, создающий новые
эффекты, обязан указать позицию вставки; golden test подтверждает, что
`after_existing` выполняется после уже стоящих записей. Replacement атомарно
заменяет совпавшую запись явно заданной последовательностью и может быть
одноразовым. Delayed trigger хранит event kind, duration и замороженные effects;
при событии они добавляются в хвост.

`PendingChoice` хранится в `State` между командами: адресат, обязательность,
границы количества, допустимые варианты, значимость порядка, revision и способ
возобновления очереди. Команда `submit_choice` — отдельная транзакция. Для
`owner_only` соперник получает только redacted waiting projection без choiceId,
kind, обязательности, вариантов и последствий; владелец получает варианты без
внутренних effect bodies.

Модификаторы пересчитываются по `(layer, createdSequence)`. Поддержаны duration
`permanent`, `until_end_of_combat`, `until_end_of_turn` и
`while_source_valid`. Старый target ref после смены зоны детерминированно
fizzle-ится событием и никогда не находит новый generation того же instance.

## Граница решения

- Framework и synthetic golden fixtures выполнены в #11.
- Конкретные EffectSpec, keywords, timing/rulings и conformance каждого
  `baz-*` реализуются в #12; текущие 126 записей остаются `deferred`.
- HTTP/WSS payload и полноценные PlayerView определены в #14. Методы projection
  здесь задают engine boundary и не являются готовым сетевым контрактом.
- Неоднозначная способность остаётся `blocked` по OQ-02; handler «по аналогии»
  не добавляется.

## Последствия

- Engine pin повышен с `deterministic-core-v1` до `effect-pipeline-v1`.
- Ошибка mode/target/payment/choice либо blocked ability не меняет state и
  revision.
- Choice не удерживает сетевую транзакцию и переживает reconnect/resync как
  часть authoritative state.
- Queue, modifiers, delayed triggers, replacements и choices входят в canonical
  state hash и replay.

## Расширение issue #37

Protocol 1.2 применяет эту модель к реальному `playCard`. Команда не передаёт
интерпретированный effect: сервер сам строит `legalActions` из закреплённых
CardDefinition/AbilityDefinition, проверяет generation, стоимость, оплату и
placement, затем открывает persisted PendingChoice только в точном окне текста.
Continuation является внутренней частью authoritative state и никогда не
попадает в PlayerView. Полный аудит 126 handlers и OQ-02 границы зафиксированы в
[отчёте issue #37](../reports/issue-37-play-card-choice-audit.md).

