# Issue #37: server-authoritative playCard и аудит PendingChoice

Связанные задачи: [#11](https://github.com/dKorialstrasz/response-online/issues/11),
[#12](https://github.com/dKorialstrasz/response-online/issues/12),
[#14](https://github.com/dKorialstrasz/response-online/issues/14),
[#17](https://github.com/dKorialstrasz/response-online/issues/17),
[#37](https://github.com/dKorialstrasz/response-online/issues/37). Архитектурная
основа — [ADR-0007](../adr/0007-effect-resolution-and-pending-choices.md).

## Архитектурное решение

`playCard` является одной clone/apply/validate/commit транзакцией. Сервер
сопоставляет generation-bound `sourceRef` с закреплённой `CardDefinition`,
проверяет фазу, acting seat, revision, владельца, контроллера, руку, тип,
стоимость, оплату, Magic Shield, линию/слот, свободную клетку и действие. Затем
он меняет зону, увеличивает generation и запускает все и только те способности,
чьё точное окно совпадает с розыгрышем: spell resolution, `Призыв` или
continuous enhancement. Порядок `abilityIds` закреплён каталогом; «первая
ability» не имеет особого статуса.

Все решения после изменения state хранятся как `PendingChoice` с закрытым
server-only continuation. Клиент получает только allowlist вариантов, а не
effect bodies или raw state. Успешный `submitChoice` возобновляет тот же
`ActionResolution`; invalid, stale, duplicate и foreign команды откатывают
весь клон. Choice, queue, continuation и их порядок входят в canonical hash и
переживают snapshot/resync/replay.

## Метод аудита 126 closed handlers

Проверены все 126 записей `content/abilities/base-abilities.json` против
`exactAbilityText` и clarifications в закреплённых `CardDefinition`. Отдельно
искались: решение после draw/reveal/shuffle/sacrifice; решение другого игрока;
добровольность («можете», «не более»); порядок; режим; X-оплата; размещение;
выбор раскрытых/добранных карт; замещение. Не попавшие в таблицу 87 handlers не
содержат такого решения в точном тексте: это безусловный эффект, единственная
цель до изменения state или continuous/keyword правило.

## Card/ability → decision window → actor → visibility → implementation → conformance

| Card / ability | Decision window | Actor | Visibility | Implementation | Conformance scenario / evidence |
| --- | --- | --- | --- | --- | --- |
| `baz-004.front.ability` | after opponent top-card reveal: play or decline | controller | public reveal | OQ-02-37-A; no inferred nested-play semantics | `baz-004.front.ability.conformance` |
| `baz-009.front.ability` | after Summon: optional target + free cell on same field | controller | public | `optional_move_other_card` PendingChoice, target+lane option | scenario + `resumeBuiltinChoice/baz-009` |
| `baz-015.front.ability` | beginning of combat: optional unit before source | controller | public | `optional_move_unit_before_source` PendingChoice | scenario + choice continuation test coverage |
| `baz-016.front.ability` | after draw and self-sacrifice: target + destination | controller | public | mandatory post-death PendingChoice | scenario + `TestPotionDeathTargetIsChosenAfterDrawAndSacrifice` |
| `baz-018.front.ability` | after five-card reveal: optional eligible free play; random bottom remainder | controller | public reveal | OQ-02-37-A/B; no inferred nested-play semantics | `baz-018.front.ability.conformance` |
| `baz-020.front.ability` | look four; choose two, then order the other two | controller | owner-only | two persistent choices; second has `orderMatters=true` | `baz-020.front.ability.conformance` |
| `baz-021.front.ability` | choose 0..2 opposing units | controller | public | optional PendingChoice, min 0/max 2 | `baz-021.front.ability.conformance` |
| `baz-023.level-1.ability` | after draw: discard one from current hand | controller | owner-only | mandatory post-draw PendingChoice | `baz-023.level-1.ability.conformance` |
| `baz-025.front.ability` | after opponent order: return source or decline | owner of source | public | optional source-bound PendingChoice | `baz-025.front.ability.conformance` |
| `baz-028.front.ability` | after Summon: opponent chooses grave card; controller then may play | opponent, then controller | public grave | OQ-02-37-A/B; actor split identified, legacy direct target is not exposed as legal action | `baz-028.front.ability.conformance` |
| `baz-029.front.ability` | after Summon: discard current-hand card | controller | owner-only | mandatory PendingChoice | `baz-029.front.ability.conformance` |
| `baz-034.front.ability` | after draw and self-sacrifice: grave unit <=4 + free unit cell | controller | public | mandatory post-death target+placement PendingChoice | scenario + potion regression test family |
| `baz-036.front.ability` | after spell starts: discard hand card or sacrifice own board card | target opponent | owner-only because one branch contains a closed hand | mandatory opponent-owned PendingChoice; activator gets no id/kind/bounds/refs | scenario + `TestBaz036ChoiceBelongsToOpponentAndNeverLeaksHandRefs` |
| `baz-038.front.ability` | look two; choose hand card, other goes grave | controller | owner-only | mandatory choice bound to exact top-two generations | `baz-038.front.ability.conformance` |
| `baz-042.front.ability` | initial grave unit + free unit cell | controller | public | atomic `playCard` target and placement allowlists | `baz-042.front.ability.conformance` |
| `baz-043.front.ability` | after draw three: discard current-hand card | controller | owner-only | mandatory post-draw PendingChoice | `baz-043.front.ability.conformance` |
| `baz-044.front.ability` | opponent discard, then opponent sacrifice | target opponent | first owner-only; second public | two ordered mandatory PendingChoices | `baz-044.front.ability.conformance` |
| `baz-047.front.ability` | after opponent order: optional grave/void card to shuffle | owner | owner-only | optional PendingChoice | `baz-047.front.ability.conformance` |
| `baz-050.front.ability` | death: choose among tied highest-cost opposing cards | target opponent | public | deterministic auto-resolution for one candidate, opponent PendingChoice for tie | `baz-050.front.ability.conformance` |
| `baz-055.front.ability` | spell: choose among tied highest-cost own cards | each opponent | public | same highest-cost continuation; two-seat target opponent | `baz-055.front.ability.conformance` |
| `baz-059.front.ability` | after beginning-combat boost: optional swap target | controller | public | optional post-boost PendingChoice | `baz-059.front.ability.conformance` |
| `baz-060.front.ability` | after draw and self-sacrifice: unit for +1/Charge | controller | public | mandatory post-death PendingChoice | scenario + `TestPotionDeathTargetIsChosenAfterDrawAndSacrifice` |
| `baz-065.front.ability` | choose 0..2 grave/void cards; then draw two | controller | owner-only | optional PendingChoice, draw continuation runs after selection/decline | `baz-065.front.ability.conformance` |
| `baz-066.front.ability` | after shuffle/reveal: optional placement of eligible top unit; otherwise draw two | controller | public reveal | optional placement PendingChoice bound to current top generation | `baz-066.front.ability.conformance` |
| `baz-072.front.ability` | after opponent order: draw two or decline | owner | public | optional binary PendingChoice | `baz-072.front.ability.conformance` |
| `baz-076.front.ability` | action declaration: choose X and target | controller | public | exact `influenceCosts` allowlist and `influencePayment`; transactional validation | `baz-076.front.ability.conformance` |
| `baz-078.front.ability` | after draw and self-sacrifice: unit for +4 | controller | public | mandatory post-death PendingChoice | potion regression test family |
| `baz-086.front.ability` | after draw: optional current-hand free play <=3 | controller | owner-only until played | OQ-02-37-A; post-draw card set identified, nested play not inferred | `baz-086.front.ability.conformance` |
| `baz-088.front.ability` | choose 0..1 unit while ignoring Magic Shield | controller | public | optional PendingChoice, min 0/max 1 | `baz-088.front.ability.conformance` |
| `baz-091.front.ability` | after opponent order: opponent chooses own hand card to void | that opponent | owner-only | mandatory opponent-owned PendingChoice | `baz-091.front.ability.conformance` |
| `baz-099.front.ability` | after draw and self-sacrifice: unit for Resilience | controller | public | mandatory post-death PendingChoice | potion regression test family |
| `baz-102.front.ability` | after reveal four: choose 0..1 revealed unit; random bottom rest | controller | public reveal | optional PendingChoice bound to revealed generations | scenario + `TestBaz102OptionalPostRevealChoiceCanDeclineAndSurvivesResyncSnapshot` |
| `baz-105.front.ability` | opponent chooses hand card to void; then conditional draw | target opponent, then automatic | owner-only | mandatory opponent choice + ordered continuation | `baz-105.front.ability.conformance` |
| `baz-107.front.ability` | choose 0..1 card to void | controller | public for eligible public zones | optional PendingChoice; hidden zones excluded | `baz-107.front.ability.conformance` |
| `baz-109.front.ability` | initial void unit + free unit cell | controller | public | atomic `playCard` target and placement allowlists | `baz-109.front.ability.conformance` |
| `baz-112.front.ability` | Summon: choose 0..1 own grave/void card for topdeck; death draw is automatic | controller | owner-only | optional source-window PendingChoice; separate death window remains ordered | `baz-112.front.ability.conformance` |
| `baz-121.front.ability` | draw two, then choose exactly one card from resulting hand for topdeck | controller | owner-only | mandatory post-draw PendingChoice using current generation refs | scenario + `TestBaz121DrawThenMandatoryCurrentHandChoiceReplayAndPrivacy` |
| `baz-126.front.ability` | after draw and self-sacrifice: opposing unit <=2 + free own cell | controller | public | mandatory post-death target+placement PendingChoice | potion regression test family |
| `baz-127.front.ability` | after +2: optional destination for the same unit | controller | public | optional post-boost PendingChoice bound to boosted target | `baz-127.front.ability.conformance` |

## Доказательства baz-036 и baz-121

`baz-036` не принимает hand/board refs от активатора. После оплаты и перемещения
заклинания в кладбище сервер строит актуальный allowlist для другого seat. Для
него owner-view содержит opaque options; для активатора redacted-view содержит
только `waitingSeat`, `waiting`, `revision`, `redacted`. Попытка активатора
ответить либо передать несуществующий option отклоняется без изменения hash.

`baz-121` сначала выполняет draw 2, затем перечисляет всю актуальную руку. Выбор
обязателен (1..1), owner-only и generation-bound. После submit выбранный объект
переходит на верх колоды с новым generation. Старый ref больше не разрешается;
replay той же пары команд даёт тот же canonical state hash.

## Projection/privacy assertions

| Получатель | Owner-only choice | Public choice |
| --- | --- | --- |
| Seat-владелец | id, kind, explicit mandatory/optional, min/max/order, opaque options | те же поля и options |
| Другой seat | только waitingSeat/waiting/revision/redacted; нет id, kind, mandatory, bounds, refs, continuation | id, kind, explicit mandatory/optional, bounds и optionCount; без option refs и internal effects |

`PlayerView.legalActions` строится сервером отдельно для каждого seat. Он не
содержит refs закрытой руки соперника. Continuation handler, raw `CardObject`,
effect bodies, seed/RNG cursor и внутренние instance IDs не сериализуются.

## OQ-02 blockers

- `OQ-02-37-A` (`baz-004`, `baz-018`, `baz-028`, `baz-086`): нужен ruling для
  вложенного «разыграйте бесплатно», если выбранная карта сама требует mode,
  target, payment, Magic Shield или следующего PendingChoice: является ли это
  единым action resolution родительской карты и кто принимает каждое вложенное
  решение. До ruling сервер не подставляет первый mode/target и не публикует
  такой вариант как полностью разрешимый.
- `OQ-02-37-B` (`baz-004`, `baz-018`, `baz-028`): нужен ruling о controller и
  конечной зоне чужой карты при бесплатном розыгрыше, а также о допустимости
  выбора карты, которую сейчас невозможно разместить. `OwnerSeat` не меняется;
  остальное не выводится по аналогии.

Эти blockers не затрагивают обычный `playCard`, `baz-036`, `baz-121` и остальные
строки таблицы. Спорные nested-play ветви не считаются закрытыми conformance до
owner ruling и не должны расширяться новым поведением по аналогии.

## Browser E2E evidence

Локально подняты реальные backend `127.0.0.1:8080` и Vite UI
`127.0.0.1:5173`; второй seat подключён отдельной session и настоящим WebSocket.
Без mock state пройдены room/join/deck/ready/order/mulligan, после чего UI получил
server-authored legalActions. Через него разыгран `baz-121`, browser получил
owner-only mandatory choice из восьми карт актуальной post-draw руки, не показал
кнопку decline и успешно отправил выбранный option. Возврат к action view
подтвердил завершение continuation. Optional decline отдельно проверен тем же UI
renderer и реальным engine snapshot сценарием `baz-102`.
