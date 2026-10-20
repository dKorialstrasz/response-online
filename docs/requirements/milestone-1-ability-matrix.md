# M1: матрица способностей шести колод

Связанная задача: [#12](https://github.com/dKorialstrasz/response-online/issues/12).
Требования: DATA-05–06, RULE-10–11, AC-02, AC-04–05.
Версии: `rulesVersion: response-rules-2026-10-17`, `effectSpecVersion: effect-spec-v1`, `engineVersion: base-abilities-v2`.

## Решение и граница

Runtime получает подтверждённые CardDefinition и EffectSpec из одного release descriptor.
Русский текст не интерпретируется: каждый abilityId связан ровно с одним handler
закрытого реестра. Неизвестный, дублированный или несовпадающий handler блокирует
создание матча. Сценарии закреплены хешом exact text и содержат positive, negative
и interaction case. Технический статус строк ниже — self-check исполнителя.
Rules/content review принят владельцем при явном разрешении merge PR #34.

## Общая проверка

- `TestBaseAbilityConformance`: 126 × positive/negative/interaction = 378 subtests.
- `TestBaseMatchupInteractionCoverage`: 36 ordered deck pairs, включая 6 mirrors.
- `TestBaseKeywordCrossFactionAndMirrorInteractions`: Обход mirror и cross-faction Яд + Дальняя атака против Стойкости.
- `TestClosedHandlerFailureRollsBackAbilityTransaction`: invalid move не меняет hash.
- `validate_card_definitions.py`: exact-text hash, handler, scenario и matchup closure.
- `validate_content_release.py`: runtime artifacts и SHA-256 закреплены release descriptor.
- Issue #37: все 126 handlers повторно проверены по exact text; полный список
  многошаговых решений, actors, visibility и conformance evidence находится в
  [аудите playCard/PendingChoice](../reports/issue-37-play-card-choice-audit.md).

## Card/ability → implementation → tests → result

| Карта | abilityId | Реализация | Conformance cases | Результат |
| --- | --- | --- | --- | --- |
| `baz-001` Дари, Навигатор | `baz-001.level-1.ability` | `baz-001.level-1.ability` | `baz-001.level-1.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-002` Рыцарь Технологий | `baz-002.front.ability` | `baz-002.front.ability` | `baz-002.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-003` Ускоряющий энобот | `baz-003.front.ability` | `baz-003.front.ability` | `baz-003.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-004` Анализатор потенциала | `baz-004.front.ability` | `baz-004.front.ability` | `baz-004.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-005` Упорный охотник | `baz-005.front.ability` | `baz-005.front.ability` | `baz-005.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-006` Наёмник из Эносити | `baz-006.front.ability` | `baz-006.front.ability` | `baz-006.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-008` Генератор пара | `baz-008.front.ability` | `baz-008.front.ability` | `baz-008.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-009` Патрульный энобот | `baz-009.front.ability` | `baz-009.front.ability` | `baz-009.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-010` Токсичный энобот | `baz-010.front.ability` | `baz-010.front.ability` | `baz-010.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-011` Передислокация | `baz-011.front.ability` | `baz-011.front.ability` | `baz-011.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-012` Выкинуть на свалку | `baz-012.front.ability` | `baz-012.front.ability` | `baz-012.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-013` Резервное копирование | `baz-013.front.ability` | `baz-013.front.ability` | `baz-013.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-014` Неудержимый натиск | `baz-014.front.ability` | `baz-014.front.ability` | `baz-014.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-015` Динамо-ускоритель | `baz-015.front.ability` | `baz-015.front.ability` | `baz-015.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-016` Зелье смещения | `baz-016.front.ability` | `baz-016.front.ability` | `baz-016.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-017` Снаряд плазмы | `baz-017.front.ability` | `baz-017.front.ability` | `baz-017.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-018` Перехват памяти | `baz-018.front.ability` | `baz-018.front.ability` | `baz-018.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-019` Дальнобойное орудие | `baz-019.front.ability` | `baz-019.front.ability` | `baz-019.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-020` Поиск ответа | `baz-020.front.ability` | `baz-020.front.ability` | `baz-020.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-021` Двойной удар | `baz-021.front.ability` | `baz-021.front.ability` | `baz-021.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-022` Отбросить в будущее | `baz-022.front.ability` | `baz-022.front.ability` | `baz-022.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-023` Кассий, Осквернитель | `baz-023.level-1.ability` | `baz-023.level-1.ability` | `baz-023.level-1.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-024` Рыцарь Смерти | `baz-024.front.ability` | `baz-024.front.ability` | `baz-024.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-025` Возвращающийся | `baz-025.front.ability` | `baz-025.front.ability` | `baz-025.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-027` Мёртвый банкир | `baz-027.front.ability` | `baz-027.front.ability` | `baz-027.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-028` Призрак обманщика | `baz-028.front.ability` | `baz-028.front.ability` | `baz-028.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-029` Копающий зомби | `baz-029.front.ability` | `baz-029.front.ability` | `baz-029.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-030` Неудержимый дух | `baz-030.front.ability` | `baz-030.front.ability` | `baz-030.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-031` Заблудшая Душа | `baz-031.front.ability` | `baz-031.front.ability` | `baz-031.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-032` Неуязвимый мертвец | `baz-032.front.ability` | `baz-032.front.ability` | `baz-032.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-033` Взрыв эссенции | `baz-033.front.ability` | `baz-033.front.ability` | `baz-033.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-034` Зелье воскрешения | `baz-034.front.ability` | `baz-034.front.ability` | `baz-034.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-035` Конец пути | `baz-035.front.ability` | `baz-035.front.ability` | `baz-035.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-036` Раскаяние | `baz-036.front.ability` | `baz-036.front.ability` | `baz-036.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-037` Костяной щит | `baz-037.front.ability` | `baz-037.front.ability` | `baz-037.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-038` Глаз мертвеца | `baz-038.front.ability` | `baz-038.front.ability` | `baz-038.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-039` Отравленный клинок | `baz-039.front.ability` | `baz-039.front.ability` | `baz-039.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-040` Дыхание Смерти | `baz-040.front.ability` | `baz-040.front.ability` | `baz-040.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-041` Призрачная форма | `baz-041.front.ability` | `baz-041.front.ability` | `baz-041.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-042` Возвращение к нежизни | `baz-042.front.ability` | `baz-042.front.ability` | `baz-042.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-043` Проклятое золото | `baz-043.front.ability` | `baz-043.front.ability` | `baz-043.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-044` Разложение | `baz-044.front.ability` | `baz-044.front.ability` | `baz-044.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-045` Энитокль, Поджигатель | `baz-045.level-1.ability` | `baz-045.level-1.ability` | `baz-045.level-1.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-046` Рыцарь Хаоса | `baz-046.front.ability` | `baz-046.front.ability` | `baz-046.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-047` Противный гоблин | `baz-047.front.ability` | `baz-047.front.ability` | `baz-047.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-048` Огненный дуэлянт | `baz-048.front.ability` | `baz-048.front.ability` | `baz-048.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-049` Орк-созерцатель | `baz-049.front.ability` | `baz-049.front.ability` | `baz-049.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-050` Бегущий по лаве | `baz-050.front.ability` | `baz-050.front.ability` | `baz-050.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-051` Демон-лучник | `baz-051.front.ability` | `baz-051.front.ability` | `baz-051.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-052` Самоуверенный демон | `baz-052.front.ability` | `baz-052.front.ability` | `baz-052.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-054` Медитирующий Демон | `baz-054.front.ability` | `baz-054.front.ability` | `baz-054.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-055` Демонический крик | `baz-055.front.ability` | `baz-055.front.ability` | `baz-055.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-056` Огненный плевок | `baz-056.front.ability` | `baz-056.front.ability` | `baz-056.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-057` Неизвестные возможности | `baz-057.front.ability` | `baz-057.front.ability` | `baz-057.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-058` Агрессивный взгляд | `baz-058.front.ability` | `baz-058.front.ability` | `baz-058.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-059` Хаотичный обмен | `baz-059.front.ability` | `baz-059.front.ability` | `baz-059.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-060` Зелье упорства | `baz-060.front.ability` | `baz-060.front.ability` | `baz-060.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-061` Гоблинская насмешка | `baz-061.front.ability` | `baz-061.front.ability` | `baz-061.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-062` Огненный шар | `baz-062.front.ability` | `baz-062.front.ability` | `baz-062.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-063` Неудержимая ярость | `baz-063.front.ability` | `baz-063.front.ability` | `baz-063.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-064` Завораживающий поток | `baz-064.front.ability` | `baz-064.front.ability` | `baz-064.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-065` Пылающие воспоминания | `baz-065.front.ability` | `baz-065.front.ability` | `baz-065.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-066` Хаотический призыв | `baz-066.front.ability` | `baz-066.front.ability` | `baz-066.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-067` Медана, Собирательница | `baz-067.level-1.ability` | `baz-067.level-1.ability` | `baz-067.level-1.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-068` Рыцарь Природы | `baz-068.front.ability` | `baz-068.front.ability` | `baz-068.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-070` Распыляющая пыльцу | `baz-070.front.ability` | `baz-070.front.ability` | `baz-070.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-071` Злобная мухоловка | `baz-071.front.ability` | `baz-071.front.ability` | `baz-071.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-072` Дриада нового посева | `baz-072.front.ability` | `baz-072.front.ability` | `baz-072.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-073` Искательница фруктов | `baz-073.front.ability` | `baz-073.front.ability` | `baz-073.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-074` Древесный дракон | `baz-074.front.ability` | `baz-074.front.ability` | `baz-074.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-075` Лаймовая кошка | `baz-075.front.ability` | `baz-075.front.ability` | `baz-075.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-076` Изумрудный ящер | `baz-076.front.ability` | `baz-076.front.ability` | `baz-076.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-077` Корневой покров | `baz-077.front.ability` | `baz-077.front.ability` | `baz-077.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-078` Зелье роста | `baz-078.front.ability` | `baz-078.front.ability` | `baz-078.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-079` Показать силу | `baz-079.front.ability` | `baz-079.front.ability` | `baz-079.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-080` Сфера изобилия | `baz-080.front.ability` | `baz-080.front.ability` | `baz-080.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-081` Восстановление | `baz-081.front.ability` | `baz-081.front.ability` | `baz-081.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-082` Зов корней | `baz-082.front.ability` | `baz-082.front.ability` | `baz-082.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-083` Сонный паралич | `baz-083.front.ability` | `baz-083.front.ability` | `baz-083.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-084` Сотрясающая буря | `baz-084.front.ability` | `baz-084.front.ability` | `baz-084.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-085` Шипастый покров | `baz-085.front.ability` | `baz-085.front.ability` | `baz-085.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-086` Буйный рост | `baz-086.front.ability` | `baz-086.front.ability` | `baz-086.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-087` Ослепляющие споры | `baz-087.front.ability` | `baz-087.front.ability` | `baz-087.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-088` Гнев друида | `baz-088.front.ability` | `baz-088.front.ability` | `baz-088.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-089` Катарина, Вдохновитель | `baz-089.level-1.ability` | `baz-089.level-1.ability` | `baz-089.level-1.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-090` Рыцарь Жизни | `baz-090.front.ability` | `baz-090.front.ability` | `baz-090.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-091` Могучий очиститель | `baz-091.front.ability` | `baz-091.front.ability` | `baz-091.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-093` Жрец легиона | `baz-093.front.ability` | `baz-093.front.ability` | `baz-093.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-094` Освятитель оружия | `baz-094.front.ability` | `baz-094.front.ability` | `baz-094.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-095` Хранитель башни | `baz-095.front.ability` | `baz-095.front.ability` | `baz-095.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-096` Верный рыцарь | `baz-096.front.ability` | `baz-096.front.ability` | `baz-096.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-097` Наставник героев | `baz-097.front.ability` | `baz-097.front.ability` | `baz-097.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-098` Молодой кузнец | `baz-098.front.ability` | `baz-098.front.ability` | `baz-098.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-099` Зелье стойкости | `baz-099.front.ability` | `baz-099.front.ability` | `baz-099.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-100` Сияющее копьё | `baz-100.front.ability` | `baz-100.front.ability` | `baz-100.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-101` Тяжёлый арбалет | `baz-101.front.ability` | `baz-101.front.ability` | `baz-101.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-102` Поиск новобранца | `baz-102.front.ability` | `baz-102.front.ability` | `baz-102.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-103` Отречение | `baz-103.front.ability` | `baz-103.front.ability` | `baz-103.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-104` Ослепляющая стрела | `baz-104.front.ability` | `baz-104.front.ability` | `baz-104.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-105` Познание космоса | `baz-105.front.ability` | `baz-105.front.ability` | `baz-105.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-106` Освящённые клинки | `baz-106.front.ability` | `baz-106.front.ability` | `baz-106.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-107` Спираль космоса | `baz-107.front.ability` | `baz-107.front.ability` | `baz-107.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-108` Золотой покров | `baz-108.front.ability` | `baz-108.front.ability` | `baz-108.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-109` Призыв очистителя | `baz-109.front.ability` | `baz-109.front.ability` | `baz-109.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-110` Разгорающийся огонь | `baz-110.front.ability` | `baz-110.front.ability` | `baz-110.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-111` Бьёрн, Усилитель | `baz-111.level-1.ability` | `baz-111.level-1.ability` | `baz-111.level-1.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-112` Джинн одного желания | `baz-112.front.ability` | `baz-112.front.ability` | `baz-112.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-113` Мистический странник | `baz-113.front.ability` | `baz-113.front.ability` | `baz-113.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-114` Рыцарь Порядка | `baz-114.front.ability` | `baz-114.front.ability` | `baz-114.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-115` Пурпурный ловкач | `baz-115.front.ability` | `baz-115.front.ability` | `baz-115.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-117` Страж Кхазалида | `baz-117.front.ability` | `baz-117.front.ability` | `baz-117.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-118` Статный воевода | `baz-118.front.ability` | `baz-118.front.ability` | `baz-118.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-119` Разрушитель планов | `baz-119.front.ability` | `baz-119.front.ability` | `baz-119.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-120` Адепт регламента | `baz-120.front.ability` | `baz-120.front.ability` | `baz-120.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-121` Разумный выбор | `baz-121.front.ability` | `baz-121.front.ability` | `baz-121.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-122` Падение в шахту | `baz-122.front.ability` | `baz-122.front.ability` | `baz-122.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-123` Морозный клинок | `baz-123.front.ability` | `baz-123.front.ability` | `baz-123.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-124` Закалённый щит | `baz-124.front.ability` | `baz-124.front.ability` | `baz-124.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-125` Лёгкие доспехи | `baz-125.front.ability` | `baz-125.front.ability` | `baz-125.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-126` Зелье контроля | `baz-126.front.ability` | `baz-126.front.ability` | `baz-126.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-127` Молниеносное смещение | `baz-127.front.ability` | `baz-127.front.ability` | `baz-127.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-128` Обжигающий холод | `baz-128.front.ability` | `baz-128.front.ability` | `baz-128.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-129` Стойкий нрав | `baz-129.front.ability` | `baz-129.front.ability` | `baz-129.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-130` Зачарованный оберег | `baz-130.front.ability` | `baz-130.front.ability` | `baz-130.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-131` Узкая тропа | `baz-131.front.ability` | `baz-131.front.ability` | `baz-131.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |
| `baz-132` Ледяной поток | `baz-132.front.ability` | `baz-132.front.ability` | `baz-132.front.ability.conformance.positive` / `.negative` / `.interaction` | implemented; self-check passed |

## Статус приёмки

- Rules/content review и merge PR #34 явно приняты владельцем.
- Полные 72 end-to-end smoke-прогона относятся к #20, не подменяются 36 engine interaction rows.
- Gameplay protocol `1.2` добавляет generation-safe `playCard`, server-authored
  legalActions и устойчивую choice projection; `make m1-release-check` проходит.
- Публичное развёртывание, GamePix и права на публикацию остаются вне M1.
