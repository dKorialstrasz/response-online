# Карточный контент Milestone 1

Связанная задача: [#6](https://github.com/dKorialstrasz/response-online/issues/6).  
Требования: DATA-02–06, DATA-09, DEP-02, OQ-01, OQ-02, OQ-07, AC-02.

## Зафиксированный источник

17 октября 2026 года официальный раздел [«Галерея карт»](https://response-game.ru/spoiler)
загрузил данные из JSON API `https://response-league.ru/api/v1/cards`. Все пять
страниц ответа имели одну версию каталога
`6aa6a9a1fe7984148c7209427fb125053d190b2f1671149f9dc83bace02f0d35`.
Из 427 записей выбраны 132 основные печати выпуска «Базовый» с идентификаторами
`baz-001`–`baz-132`. API-ответ перенесён без OCR; его нормализованный снимок и
SHA-256 закреплены в
[`content/evidence`](../../content/evidence/base-gallery-evidence.json).

Снимок подтверждает опубликованные поля карточек, но не права на цифровое
использование. Изображения не скачиваются и не входят в сборку: реестр хранит
только ссылки, статус прав `unknown` и запрет публичного использования.

## Артефакты

| Артефакт | Назначение |
| --- | --- |
| [`card-definitions.json`](../../content/cards/base/card-definitions.json) | 132 определения: идентификатор, точное название галереи, типовая строка, фракция, стоимость, характеристики, лицо/уровень, точный текст, отдельные пояснения и field-level evidence |
| [`base-abilities.json`](../../content/abilities/base-abilities.json) | 126 стабильных abilityId, источник, хеш точного текста, состояние ambiguity review и отдельная ссылка на будущий effectSpec |
| [`base-ability-scenarios.json`](../../content/conformance/base-ability-scenarios.json) | По одному обязательному conformance-сценарию на abilityId; игровая реализация и исполняемые проверки относятся к #12 |
| [`base-content-closure.json`](../../content/closure/base-content-closure.json) | Шесть manifests как корни, 132 разрешённых определения и отсутствие дополнительных создаваемых объектов |
| [`registry.json`](../../content/licenses/registry.json) | 132 ссылки на изображения со статусом прав `unknown`; отсутствие bundled fonts, sounds и advertising assets |
| [`base-name-discrepancies.json`](../../content/reviews/base-name-discrepancies.json) | Различия написания между product pages и галереей без молчаливой нормализации |
| [`base-card-validation.json`](../../content/reports/base-card-validation.json) | Детерминированный отчёт проверки структуры, evidence, ссылок и замыкания |

## Семантика значений

Числа хранятся как `kind: fixed` только вместе со статусом `source-matched`.
Подтверждённое отсутствие свойства хранится как `kind: notApplicable` без
`value`. Неизвестное значение представляется как `kind: unknown`, `value: null`
и не смешивается с нулём. В текущем каталоге обязательных полей со статусом
`unknown` нет; статус `unknown` сохранён только для прав на изображения.

Точный русский текст находится только в `faces[].exactAbilityText`.
`clarifications[]` содержит отдельное поле «Пояснение» из галереи. Машинная
семантика не выводится из текста: `effectSpec.status` остаётся `deferred` и
ссылается на #12. Это сохраняет границу между источником, будущей реализацией и
решениями rules owner.

## Транзитивное замыкание

Объединение шести утверждённых manifests содержит все `baz-001`–`baz-132` и не
содержит иных definitionId. В точных текстах этих карт нет маркеров создания
отдельной карты, жетона или объекта. Поэтому реестр замыкается на 132
CardDefinition, а `transitiveObjectIds` пуст. Валидатор повторно проверяет корни,
ссылки и маркеры создаваемого контента; появление такого текста без явной ссылки
делает проверку красной.

## Различия названий

OQ-07 уже задаёт решение: CardDefinition сохраняет точное написание галереи, а
manifest — точное написание product page; варианты не переписываются молча.

| cardId | Product page | Галерея |
| --- | --- | --- |
| baz-029 | Копающий Зомби | Копающий зомби |
| baz-030 | Неудержимый Дух | Неудержимый дух |
| baz-031 | Заблудшая душа | Заблудшая Душа |
| baz-054 | Медитирующий демон | Медитирующий Демон |
| baz-120 | Адепт Регламента | Адепт регламента |

## Проверка и review gate

```bash
python3 scripts/validate_card_definitions.py \
  --check-report content/reports/base-card-validation.json
python3 -m unittest discover -s tests/content -p 'test_*.py'
```

Структурная проверка проходит. Отдельный owner verification pass намеренно
остаётся `pending`: исполнитель не выдаёт self-check за проверку владельца.
После явной приёмки владелец переводит evidence и определения в `approved`,
добавляет дату проверки и запускает строгий режим
`--require-owner-reviewed`. До этого issue и Project status не переводятся в
`Done`, даже если обычный CI зелёный.

Публичное развёртывание, передача сборки и утверждение прав запрещены независимо
от статуса локальной проверки. Эти ворота остаются в #7 и не входят в M1.
