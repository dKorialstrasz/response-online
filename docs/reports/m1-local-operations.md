# Local operations self-check для issue #18

- Дата: 2026-10-20
- Scope: local standalone M1; staging/production/TLS/public SLO исключены.
- Requirements: NFR-11–13, SEC-08–09, OPS-01–06, AC-09–10.
- Проверенный base commit: `a3222b773987e1b978d9e4767865c59eed485183`.

## Реализованный operational contract

- `.env.example` задаёт воспроизводимый loopback profile; `make dev-backend`
  загружает локальный `.env`.
- HTTP access logs — JSON и используют route pattern вместо URL; query, IP,
  cookies, bodies, tickets, руки и seed не записываются.
- `/metrics` отдаёт только агрегаты rooms/matches, reject codes, latency buckets,
  reconnect и technical abort; публичного audit/trace endpoint нет.
- SIGINT/SIGTERM сначала закрывает admission/readiness, затем сериализованно
  завершает незаконченные матчи как `aborted_technical` без победителя и только
  после этого останавливает runtime.
- Crash/restart не обещает recovery: весь активный in-memory матч теряется по
  ADR-0001.
- Build manifest содержит commit, dirty-tree digest при необходимости, все
  version pins, artifact hashes, локальный QA status и GitHub Actions metadata.

## Выполненные проверки

```text
make check    PASS
make smoke    PASS
make manifest PASS
```

`make check` включал Go vet/race tests, 26 frontend tests, frontend build,
13 content unit tests, schemas/contracts, deck/card/release validation,
documentation consistency и secret scan. `make smoke` собрал отдельный binary,
проверил health/build/metrics, создал process-local room, подтвердил потерю этой
сессии после SIGKILL/restart, затем отправил SIGTERM и проверил JSON
start/shutdown/stop events на отсутствие чувствительных полей. Unit test controlled shutdown создал
активный матч, подтвердил `aborted_technical`, отсутствие winner, закрытый
admission и счётчик technical abort.

Локально созданный `build/build-manifest.json` зафиксировал QA status `passed`;
CI metadata честно имеет `prerequisiteStatus: not-run`. В GitHub Actions build job
получит `prerequisiteStatus: passed` только после обязательных prerequisite jobs.

## Незакрытый внешний gate

Длительный прогон не выполнялся: issue #21 открыта, комментариев нет, длительность
soak и test plan владельцем ещё не утверждены. NFR-13 прямо запрещает выбирать
длительность после запуска или выдавать произвольный прогон за acceptance
evidence. После утверждения #21 нужно выполнить immutable manifest artifact и
добавить отдельный soak report; до этого соответствующий checkbox #18 остаётся
незакрытым.
