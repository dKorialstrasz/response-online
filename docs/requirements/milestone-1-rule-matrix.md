# M1: матрица базового игрового ядра

Матрица относится к задаче [#10](https://github.com/dKorialstrasz/response-online/issues/10)
и версии `deterministic-core-v1`. Нормативный источник — официальная страница
[«Правила»](https://response-game.ru/rules), сверенная 17.10.2026. У страницы нет
публичного номера редакции; используемый pin — `response-rules-2026-10-17`.

Матрица #10 не закрывает EffectSpec и карточные неоднозначности. Общий pipeline
очереди/choices/modifiers реализован в #11 и вынесен в
[`milestone-1-effect-matrix.md`](milestone-1-effect-matrix.md); конкретные
способности и спорные rulings остаются #12. По OQ-02 универсальные ответы и
механики других ККИ не добавляются.

| Requirement | Норма/инвариант | Реализация | Проверка | Результат #10 |
| --- | --- | --- | --- | --- |
| RULE-01 | Только S1 и подтверждённые тексты | Автомат базового цикла без выдуманных card handlers | `TestSetupMulliganAndReplayAreDeterministic` | Базовый цикл выполнен; abilities остаются #11/#12 |
| RULE-02 | Инициатива, порядок Приказов, shuffle, рука 7, одна пересдача | Injected seed, order lock, выбор 0..7 карт, новые generation refs | setup/mulligan/replay tests | Пройдено |
| RULE-03 | start → draw → action → combat → end → recovery; 5 решений; два паса | Серверные переходы, budgets и auto-pass | lifecycle и exhausted-budget tests | Пройдено |
| RULE-04 | Зоны, пять линий, одна занятая клетка | Zone/slot model, owner/controller, invariant checks | double-cell и generation tests | Базовая модель пройдена; типовые card restrictions — #11 |
| RULE-05 | Ошибка откатывает операцию целиком | Clone/apply/validate/commit | rollback hash test | Транзакционная основа пройдена; play/payment — #11 |
| RULE-07 | Бой по пяти линиям относительно инициативы | Ascending/descending physical lanes; simultaneous unit collision; location; hero hit | combat table tests | Базовый бой пройден; interrupts/Обход — #11 |
| RULE-08 | Слева направо раскрываются 5 Приказов; шестое требование — поражение | Ordered `OrderState`, single immutable result | hero-order и missing-order tests | Пройдено; эффекты Приказов — #11/#12 |
| RULE-09 | Recovery восстанавливает базовые значения, разворачивает карты и передаёт инициативу | `recoverState` и новый turn | lifecycle test | Базовая recovery пройдена; continuous/temporary effects — #11 |
| RULE-12 | Технический предел цепочки; `aborted_technical` без победителя | 10 000 events/command, trace, atomic abort | runaway-guard test | Пройдено |
| RULE-13 | owner/controller различны; смена зоны инвалидирует старую ссылку | Stable audit instance + generation-bound zone ref | stale-generation test | Пройдено |
| RULE-14 | validate → calculate → invariants → commit; deterministic order | Transactional `Handle`, ordered slices, captured RNG | rollback и replay golden hash | Пройдено |
| RULE-15 | Один объект/зона, одна карта/клетка, неотрицательные actions, один result, choice не теряется | `ValidateState` после событий и команды | invariant tests | Пройдено для model foundation; choice handlers — #11 |

## Golden evidence

- Setup + полная пересдача Seat A + нулевая пересдача Seat B при seed `42`:
  `f7ebf0fdfae4ee50bdb16de00aca6f7e1e2f43fa337fb2dd3e051ef5ba1429c9`
  для исторического pin `deterministic-core-v1`; после добавления сериализуемого
  effect state в #11 текущий pin `effect-pipeline-v1` имеет hash
  `3a390ad4affd4ae802ba39cd1ace26996d29cfbdfc1f3927ae969de64526995c`.
- Replay проверяет тот же RNG cursor и canonical state hash.
- Race-test `MatchSession` допускает ровно одну команду на ожидаемой revision;
  конкурент получает `ErrStaleRevision` без второй мутации.
- Runaway fixture с тестовым cap `1` завершается `aborted_technical` без
  `winnerSeat` и сохраняет diagnostic trace.

## Открытые ворота

- #11: общий play/activation pipeline, очередь, триггеры, pending choices,
  targets и payment semantics реализованы; карточные записи не считаются
  исполняемыми без #12.
- #12: все verified abilityId и keyword/conformance scenarios шести колод.
- #14: публичные protocol commands и персональные PlayerView; внутренний
  `commit_action` не экспортируется клиенту.

