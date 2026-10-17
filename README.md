# response-online

Браузерная онлайн-версия ККИ «Респонс».

## Документация

- [Требования Milestone 1: «Базовый» Bo1, шесть стартовых колод](docs/requirements/milestone-1.md)
- [Roadmap Milestone 1: issues, приоритеты и зависимости](docs/requirements/milestone-1-roadmap.md)
- [Официальные источники составов шести стартовых колод](docs/requirements/milestone-1-deck-sources.md)
- [Машиночитаемые manifests шести стартовых колод](content/decks/base)
- [Verified CardDefinition, evidence и транзитивное замыкание](docs/requirements/milestone-1-card-content.md)
- [Контентные схемы и release-blocking validator](docs/requirements/milestone-1-content-release.md)
- [Журнал продуктовых решений M1](docs/decisions/m1-product-decisions.md)
- [ADR-0001: in-memory состояние матчей M1](docs/adr/0001-in-memory-match-state.md)
- [Правила участия и traceability](CONTRIBUTING.md)
- [Архитектурные решения](docs/adr/README.md)
- [Локальная разработка](docs/runbooks/local-development.md)

Спецификация содержит обязательные критерии приёмки, реестр подтверждённых данных и открытые зависимости. Наличие документа не означает готовности игры или разрешения на публичное использование материалов правообладателя.

## Локальный scaffold

Требуются Go 1.22.2, Node.js 24 и pnpm 11.25.0.

```bash
make bootstrap
make check
make dev-backend
```

В другом терминале:

```bash
make dev-frontend
```

Клиент доступен на `http://127.0.0.1:5173`. Scaffold не реализует игровые правила.
Карточные определения source-matched и приняты владельцем; строгий owner-review
gate проверяется в CI. EffectSpec и игровые conformance-тесты реализуются в #12.
