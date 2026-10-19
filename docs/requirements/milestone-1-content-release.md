# Контентный release gate Milestone 1

Связанная задача: [#9](https://github.com/dKorialstrasz/response-online/issues/9).  
Требования: DATA-01–09, NFR-02, AC-01–02.

## Назначение

Каталог M1 проверяется в два уровня. JSON Schema фиксируют форму deck manifests,
CardDefinition, evidence, EffectSpec, license registry и release descriptor.
Семантический валидатор поверх схем проверяет арифметику 30/5/1, уникальность и
замыкание идентификаторов, обязательную field-level verification, канонические
SHA-256, согласованность версий и запрет публичного использования изображений с
неизвестными правами.

Канонический descriptor —
[`content/releases/m1-local.json`](../../content/releases/m1-local.json). Он
фиксирует точный набор входных документов и версии content/decks/effectSpec,
rules, protocol и engine. Новый матч в #10 обязан копировать этот набор версий в
своё неизменяемое состояние; обновление файлов не меняет уже созданный матч.

## Профили проверки

`make content` — обязательный CI/local-development gate. Он блокирует сборку при
ошибке схемы, ссылок, количества, verification, hash или rights policy. Обычный
`make build` зависит от этого gate и не может обойти его.

`make m1-release-check` — строгий финальный gate локального M1. После #10
базовые rules/engine закреплены как `response-rules-2026-10-17` и
`base-abilities-v1` (базовый автомат #10, pipeline #11 и закрытый каталог #12).
Все 126 EffectSpec имеют статус `implemented` и проходят conformance gate.
Protocol `1.0` закрывает transport gate #14. После объединения #12 и #14 строгий
gameplay-ready gate зелёный; полная локальная приёмка остаётся задачей #22.

## Права и публикация

Локальный test-only профиль допускает verified текстовые игровые данные при
`rightsStatus: unknown`, но изображения остаются ссылками и не включаются в
сборку. Descriptor всегда содержит `publicRelease.allowed: false`; права #7,
публичное развёртывание и GamePix не входят в M1.

## Изменение контента

После любого допустимого изменения runtime-документа необходимо обновить его
`canonicalSha256` в release descriptor и пройти `make content`. Изменение hash
без изменения проверяемого документа либо добавление неприкреплённого файла не
считается evidence. Изменения правил или EffectSpec дополнительно требуют ссылки
на ruling/roadmap issue и соответствующих conformance tests.
