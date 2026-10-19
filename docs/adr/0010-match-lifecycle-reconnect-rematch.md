# ADR-0010: сериализованный lifecycle матча, reconnect и rematch

- Статус: принято к проверке
- Дата: 19 октября 2026 года
- Связанная задача: [#15](https://github.com/dKorialstrasz/response-online/issues/15)
- Связанные требования: FR-19–28, NET-05–08, UC-08–10, AC-08
- Учитывает: [ADR-0001](0001-in-memory-match-state.md),
  [ADR-0003](0003-serialized-match-execution.md),
  [ADR-0008](0008-http-ws-security-boundary.md)

## Контекст

Команда игрока, reconnect и серверный timeout могут одновременно претендовать
на изменение одного матча. Отдельные mutex/timer callbacks оставили бы порядок
результата зависимым от goroutine scheduling и позволили бы timeout обойти
идемпотентность, revision и единственный terminal result.

## Решение

- Все accepted commands, heartbeat/reconnect transitions, проверки дедлайнов,
  idle warning и terminal outcomes выполняются через mailbox одного
  `MatchSession`. Перед любой командой или reconnect сначала применяются уже
  наступившие дедлайны: событие непосредственно до дедлайна принимается, на
  дедлайне и после него — нет.
- Production scheduler держит только ближайший `time.Timer`; duration строится
  из `time.Now`, поэтому Go использует monotonic component. Инъекция `Clock` и
  явная `CheckDeadlines` образуют virtual-clock seam без sleep-тестов.
- Heartbeat отправляется раз в секунду. Последний принятый heartbeat задаёт
  detection deadline 5 секунд. После detection начинается окно reconnect не
  более 20 секунд и не более оставшегося суммарного budget 60 секунд.
- Idle state существует только для seat, от которого engine реально ждёт
  arrangement приказов, mulligan, action или pending choice. Accepted action сбрасывает ожидание
  действовавшего seat; отклонённая команда, heartbeat и reconnect его не
  сбрасывают. Warning/abort наступают через 30/90 секунд.
- Engine предоставляет узкий terminal API только для
  `disconnect_forfeit`/`aborted_technical`. Уже завершённый матч отвергает
  повторную фиксацию и не может сменить result.
- Lifecycle уведомления используют только утверждённые тексты OQ-03. Resume
  повторно строит персональный allowlist `PlayerView`; raw state и скрытые
  идентификаторы в событие не передаются.
- Rematch хранит immutable summary предыдущего result. Отдельный ready каждого
  seat нужен заново; только два ready заменяют engine новым матчем с новым
  `matchId`. Тот же serialized session и WS subscriptions продолжают работу,
  а reconnect budget/idle state начинаются заново.

## Последствия

- In-memory restart по-прежнему теряет активный матч согласно OQ-04.
- Порядок конкурирующих событий определён временем дедлайна и порядком mailbox;
  terminal result создаётся ровно один раз.
- Полный snapshot после resume дороже delta, но сохраняет security boundary и
  исключает рассинхронизацию пропущенных событий.
- Изменение численных параметров или UX-текстов требует нового продуктового
  решения; они не являются игровыми правилами.

