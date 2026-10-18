# response-online

Браузерная онлайн-версия ККИ «Респонс».

## Документация

- [Требования Milestone 1: «Базовый» Bo1, шесть стартовых колод](docs/requirements/milestone-1.md)
- [Roadmap Milestone 1: issues, приоритеты и зависимости](docs/requirements/milestone-1-roadmap.md)
- [Официальные источники составов шести стартовых колод](docs/requirements/milestone-1-deck-sources.md)
- [Машиночитаемые manifests шести стартовых колод](content/decks/base)
- [Verified CardDefinition, evidence и транзитивное замыкание](docs/requirements/milestone-1-card-content.md)
- [Матрица 126 способностей и conformance](docs/requirements/milestone-1-ability-matrix.md)
- [Контентные схемы и release-blocking validator](docs/requirements/milestone-1-content-release.md)
- [Матрица базового игрового ядра](docs/requirements/milestone-1-rule-matrix.md)
- [Журнал продуктовых решений M1](docs/decisions/m1-product-decisions.md)
- [ADR-0001: in-memory состояние матчей M1](docs/adr/0001-in-memory-match-state.md)
- [Правила участия и traceability](CONTRIBUTING.md)
- [Архитектурные решения](docs/adr/README.md)
- [Локальная разработка](docs/runbooks/local-development.md)

Спецификация содержит обязательные критерии приёмки, реестр подтверждённых данных и открытые зависимости. Наличие документа не означает готовности игры или разрешения на публичное использование материалов правообладателя.

## Локальная разработка

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

Клиент доступен на `http://127.0.0.1:5173`. Backend содержит детерминированное
ядро базового жизненного цикла партии; оно пока не подключено к сетевому gameplay
и не реализует карточные EffectSpec. Карточные определения source-matched и
приняты владельцем; effect handlers/pending choices выполняются в #11, полная
ability conformance шести колод — в #12.
