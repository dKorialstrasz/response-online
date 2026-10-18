# M1 transport/security self-check

- Roadmap issue: [#14](https://github.com/dKorialstrasz/response-online/issues/14)
- Requirements: NET-01–11, SEC-01–07, SEC-09–10, AC-06–07
- Protocol: `1.0`
- Scope: локальный standalone M1; публичный deployment отсутствует

## Network audit

| Проверка | Результат |
| --- | --- |
| Session secret | Только host-only HttpOnly cookie; отсутствует в JSON room/WS payload |
| WS ticket | Только first-message auth, query string запрещён, digest удаляется при redeem |
| Origin/CORS/CSRF | Exact allowlist; credentialed CORS; mutating HTTP требует CSRF |
| Hidden engine state | `PlayerView` не имеет полей Random, Commands, audit/effect bodies/trace |
| Object identity | HMAC ref не содержит и не принимает engine instance/zone object ID |
| Opponent hand/deck | Только counts; definition/ref/order отсутствуют |
| Error/log surface | Стабильные категории и message keys; transport не логирует request body/cookie/ticket |
| Replay/race | commandId fingerprint cache + serialized MatchSession; invalid не меняет revision |
| Resource limits | Body/frame/depth/array/string/rate bounds применяются до engine |
| Two-seat delivery | Принятая команда рассылает разные `PlayerView` текущим соединениям обоих seats |
| Slow client | Синхронная запись имеет deadline; reconnect semantics переданы #15 |

## Проверки от 18 октября 2026 года

| Проверка | Результат |
| --- | --- |
| Go format/vet/race suite | `make check-go` — passed на Go 1.26.9 |
| Frontend | Prettier, ESLint, TypeScript, Vitest и production Vite build — passed |
| Contracts/content/docs/secrets | `make check-data` — passed |
| Adversarial fuzz | 228 184 inputs за 6 секунд, panic/failure отсутствуют |
| Dependency security | govulncheck v1.9.0: no vulnerabilities; pnpm audit: no known vulnerabilities |
| Dependency license | Gorilla WebSocket v1.5.3 / BSD-3-Clause registry — passed |
| Black-box runtime | `bash tests/smoke/local_backend.sh` — passed |
| Immutable artifacts | Go binary, frontend archive и build manifest созданы успешно |

Это self-check исполнителя, а не независимый security review. Issue закрывается
только после зелёного CI и явной пользовательской приёмки владельцем.
