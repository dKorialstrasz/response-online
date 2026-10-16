# Milestone 1 roadmap

Источник требований: [RO-M1-SRS v1.0](milestone-1.md). Канонический рабочий backlog: [GitHub Project #1](https://github.com/users/dKorialstrasz/projects/1/views/1). Родительский epic: [#1](https://github.com/dKorialstrasz/response-online/issues/1).

Roadmap не задаёт выдуманные календарные обещания. Порядок определяется зависимостями и воротами; даты появляются только после оценки назначенными владельцами.

## Приоритеты

- **P0** — блокирует начало зависимой работы или приёмку M1.
- **P1** — обязательная часть M1, выполняется после соответствующих P0 gates.
- **P2** — обязательная работа без текущего влияния на критический путь.
- **P3** — улучшение, не блокирующее M1; изменение scope требует решения продукта.

## Этапы и зависимости

| ID | Issue | Приоритет | Основные Requirement IDs | Зависит от |
| --- | --- | --- | --- | --- |
| M1-P01 | [#3 Traceability и review policy](https://github.com/dKorialstrasz/response-online/issues/3) | P0 | PROC-01–07, AC-10 | — |
| M1-P02 | [#4 Роли и решения OQ-01–07](https://github.com/dKorialstrasz/response-online/issues/4) | P0 | DEP-04–06, OQ-01–07, AC-11 | #3 |
| M1-P03 | [#5 Составы шести колод](https://github.com/dKorialstrasz/response-online/issues/5) | P0 | DATA-01/04/07, DEP-01, AC-01 | #3 |
| M1-P04 | [#6 Verified CardDefinition](https://github.com/dKorialstrasz/response-online/issues/6) | P0 | DATA-02–06/09, DEP-02, AC-02 | #5 |
| M1-P05 | [#7 Права на публикацию](https://github.com/dKorialstrasz/response-online/issues/7) | P0 | DATA-09, GP-08–09, DEP-03 | #5 |
| M1-P06 | [#8 Scaffold, ADR и CI](https://github.com/dKorialstrasz/response-online/issues/8) | P0 | NFR-01–03, PROC-02–07, OPS-02 | #3, #4 |
| M1-P07 | [#9 Content schemas и validator](https://github.com/dKorialstrasz/response-online/issues/9) | P0 | DATA-01–09, AC-01–02 | #5, #6 |
| M1-P08 | [#10 Детерминированное ядро](https://github.com/dKorialstrasz/response-online/issues/10) | P0 | RULE-01–05/07–09/12–15 | #4, #8 |
| M1-P09 | [#11 Effects и pending choices](https://github.com/dKorialstrasz/response-online/issues/11) | P0 | RULE-05–06/09–11/13–15 | #6, #10 |
| M1-P10 | [#12 Шесть колод: abilities/conformance](https://github.com/dKorialstrasz/response-online/issues/12) | P0 | DATA-05–06, RULE-10–11, AC-02/04/05 | #6, #9, #11 |
| M1-P11 | [#13 Сессии и комнаты](https://github.com/dKorialstrasz/response-online/issues/13) | P1 | FR-01–10, UC-01–03, SEC-02 | #8 |
| M1-P12 | [#14 HTTP/WSS, PlayerView и security](https://github.com/dKorialstrasz/response-online/issues/14) | P0 | NET-01–11, SEC-01–07/09–10 | #8, #13 |
| M1-P13 | [#15 Reconnect, outcomes и rematch](https://github.com/dKorialstrasz/response-online/issues/15) | P1 | FR-19–28, NET-05–08, AC-08 | #4, #10, #13, #14 |
| M1-P14 | [#16 Lobby и подготовка](https://github.com/dKorialstrasz/response-online/issues/16) | P1 | FR-05–07/11–16, UC-01–04 | #9, #13, #14 |
| M1-P15 | [#17 Gameplay UI](https://github.com/dKorialstrasz/response-online/issues/17) | P1 | FR-12–21, UC-05–12, NFR-04–07/14 | #10, #11, #14, #16 |
| M1-P16 | [#18 Operations и runbooks](https://github.com/dKorialstrasz/response-online/issues/18) | P1 | NFR-11–13, SEC-08–09, OPS-01–06 | #4, #8, #14, #15 |
| M1-P17 | [#19 GamePix adapter/spike](https://github.com/dKorialstrasz/response-online/issues/19) | P1 | GP-01–07, DEP-04, OQ-05 | #7, #14, #16 |
| M1-P18 | [#20 Rules/card QA и 72 smoke](https://github.com/dKorialstrasz/response-online/issues/20) | P0 | RULE-02–11, AC-04–05 | #12, #15, #17 |
| M1-P19 | [#21 NFR/security QA](https://github.com/dKorialstrasz/response-online/issues/21) | P1 | NFR-04–13, SEC-01–10, AC-06–09 | #14, #17, #18 |
| M1-P20 | [#22 Core acceptance](https://github.com/dKorialstrasz/response-online/issues/22) | P0 | AC-01–11, UC-01–12 | #4–#18, #20–#21 |
| M1-P21 | [#23 GamePix submission](https://github.com/dKorialstrasz/response-online/issues/23) | P1 | GP-01–09, AC-12 | #7, #19, #22 |

## Исполнительный поток

1. Governance и источники: M1-P01–P06.
2. Content/engine foundation: M1-P07–P10.
3. Online product path: M1-P11–P16.
4. Platform и verification: M1-P17–P19.
5. Core acceptance: M1-P20.
6. GamePix channel gate: M1-P21.

Работа может идти параллельно только после закрытия указанных зависимостей. GamePix approval не блокирует разработку standalone, но блокирует AC-12 и заявление «опубликовано».

## Traceability contract

Каждая задача, ветка, PR и коммит ссылаются на одну или несколько конкретных issues M1-P01–P21. Ссылка только на epic #1 недостаточна. Формат и review gate определены в [CONTRIBUTING.md](../../CONTRIBUTING.md).

Issue считается завершённой только после приложения evidence из её acceptance criteria. Project status `Done` не заменяет review, CI или merge в `main`. Изменения scope, правил или контента требуют отдельного решения и обновления этой карты.
