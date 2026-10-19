# ADR-0009: Закрытый реестр способностей «Базового» выпуска

- Статус: принято
- Дата: 19 октября 2026 года
- Связанная задача: [#12](https://github.com/dKorialstrasz/response-online/issues/12)
- Требования: DATA-05–06, RULE-10–11, AC-02, AC-04–05
- Учитывает: [ADR-0005](0005-deterministic-match-core.md), [ADR-0007](0007-effect-resolution-and-pending-choices.md)

## Контекст

#6 закрепил 126 exact ability texts и отделил их от машинной семантики. #11
создал общую очередь, choices, modifiers, replacements и durations, но реальные
EffectSpec оставались `deferred`. Исполнение текста либо незарегистрированный
fallback нарушили бы DATA-06 и скрыли неподтверждённые упрощения.

## Решение

Каждая запись `base-abilities.json` имеет один operation вида
`closed-handler`, причём handler обязан точно совпадать с abilityId. Engine
принимает только 126 закреплённых идентификаторов `baz-*`; шесть карт без текста
не получают фиктивных handlers. Неизвестный, дублированный или несовпадающий
handler блокирует создание матча.

`FileMatchPinProvider` проверяет SHA-256 CardDefinition, abilities и conformance,
преобразует только нужные gameplay-поля в immutable engine catalog и копирует
его в новый матч. Exact Russian text не передаётся исполнителю и не парсится.

Общие keyword и card-effect primitives работают поверх generation refs,
транзакционной команды, детерминированного RNG и явных durations #11. Обход,
Стойкость, Яд, Дальняя атака и Натиск интегрированы в бой; card-specific
handlers используют те же zone moves/modifiers и закрытую очередь.

Conformance artifact закрепляет для каждого abilityId positive, negative и
interaction case, а также все 36 ordered deck pairings, включая шесть mirrors.
Полные 72 завершённые smoke-партии остаются отдельным acceptance этапом #20.

## Последствия

- Engine pin повышен до `base-abilities-v1`.
- Content release больше не блокируется 126 deferred EffectSpec; вместе с
  `protocolVersion: 1.0` из #14 gameplay-ready gate проходит.
- Изменение handler, сценария или карточного runtime artifact меняет release
  SHA-256 и требует новых тестов и rules/content review.
- Rules/content review принят владельцем при явном разрешении merge PR #34.
