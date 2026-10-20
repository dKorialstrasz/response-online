# Контракты M1

- `http/openapi.json` — protocol `1.1`: диагностика, анонимная сессия, lobby и
  одноразовый WebSocket ticket.
- `ws/envelope.schema.json` — protocol `1.1`: first-message authentication,
  commands, server events, error codes и allowlisted PlayerView.
- `ws/examples/` — синтетические примеры без реальных секретов/скрытых карт.

`scripts/validate_contracts.py` проверяет обязательные endpoints/types, payload
schemas, примеры, error parity и literal-типы Go/TypeScript. Новые сообщения
добавляются только вместе с roadmap issue и leak/adversarial test.

