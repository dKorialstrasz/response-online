import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";

import {
  createRoom,
  createSession,
  getRoom,
  inviteCodeFromUrl,
  invitationUrl,
  isValidRoomCode,
  issueTicket,
  joinRoom,
  normalizeRoomCode,
  russianProtocolError,
  russianError,
  setReady,
  setSelection,
  type MatchCommand,
  type Session,
} from "./api";
import { GameplayScreen } from "./GameplayScreen";
import { acceptProjection, type ProjectionClock } from "./gameplay";
import {
  cardImageUrl,
  cardName,
  confirmedCard,
  confirmedCardById,
  decks,
  findDeck,
  type ConfirmedCard,
  type Deck,
} from "./catalog";
import {
  protocolVersion,
  type PlayerView,
  type RoomSnapshot,
  type ServerEvent,
} from "./protocol";

const factionColors: Record<string, string> = {
  Порядок: "#3274c8",
  Хаос: "#c3423b",
  Технологии: "#7a4ab5",
  Смерть: "#25877f",
  Природа: "#3f9556",
  Жизнь: "#c99a5e",
};

function cardFactionStyle(card: ConfirmedCard): CSSProperties {
  const colors = card.factions
    .map((faction) => factionColors[faction])
    .filter(Boolean);
  const first = colors[0] ?? "#66736b";
  const second = colors[1] ?? first;
  return {
    "--card-faction-a": first,
    "--card-faction-b": second,
  } as CSSProperties;
}

function LobbyCardImage({
  definitionId,
  size = "thumb",
}: {
  definitionId: string;
  size?: "thumb" | "medium";
}) {
  const [failed, setFailed] = useState(false);
  const name = cardName(definitionId);
  const source = cardImageUrl(definitionId, size);
  if (!source || failed) {
    return (
      <span
        className="lobby-card-fallback"
        role="img"
        aria-label={`Изображение карты ${name} недоступно`}
      >
        <strong>{name.slice(0, 1)}</strong>
        <span>{name}</span>
      </span>
    );
  }
  return (
    <img
      className="lobby-card-image"
      src={source}
      alt={name}
      loading={size === "medium" ? "eager" : "lazy"}
      onError={() => setFailed(true)}
    />
  );
}

type ConnectionState = "нет" | "подключение" | "подключено" | "восстановление";

const categoryNames: Record<string, string> = {
  hero: "Герой",
  order: "Приказ",
  unit: "Юнит",
  spell: "Заклинание",
  upgrade: "Улучшение",
};

export function moveItem<T>(items: T[], index: number, direction: -1 | 1): T[] {
  const destination = index + direction;
  if (
    index < 0 ||
    index >= items.length ||
    destination < 0 ||
    destination >= items.length
  )
    return items;
  const next = [...items];
  [next[index], next[destination]] = [next[destination], next[index]];
  return next;
}

export function reorderItem<T>(items: T[], from: number, to: number): T[] {
  if (
    from < 0 ||
    from >= items.length ||
    to < 0 ||
    to >= items.length ||
    from === to
  )
    return items;
  const next = [...items];
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

function ownSeat(room: RoomSnapshot) {
  return room.seats.find((seat) => seat.seatId === room.ownSeat);
}

function opponentSeat(room: RoomSnapshot) {
  return room.seats.find((seat) => seat.seatId !== room.ownSeat);
}

function useMatchConnection(
  session: Session | null,
  room: RoomSnapshot | null,
  onRoomSnapshot: (snapshot: RoomSnapshot) => void,
) {
  const [connection, setConnection] = useState<ConnectionState>("нет");
  const [view, setView] = useState<PlayerView | null>(null);
  const [error, setError] = useState("");
  const [mulliganSubmitted, setMulliganSubmitted] = useState(false);
  const [desynced, setDesynced] = useState(false);
  const [lifecycleNotice, setLifecycleNotice] = useState("");
  const socketRef = useRef<WebSocket | null>(null);
  const latestViewRef = useRef<PlayerView | null>(null);
  const projectionClockRef = useRef<ProjectionClock | null>(null);
  const recoveringRef = useRef(false);
  const mulliganCommandRef = useRef("");

  useEffect(() => {
    latestViewRef.current = view;
  }, [view]);

  useEffect(() => {
    if (!session || !room?.match) {
      socketRef.current?.close();
      socketRef.current = null;
      setConnection("нет");
      setView(null);
      projectionClockRef.current = null;
      setMulliganSubmitted(false);
      setDesynced(false);
      setLifecycleNotice("");
      return;
    }

    let disposed = false;
    let retryTimer: number | undefined;
    let heartbeatTimer: number | undefined;
    let firstConnection = true;
    let reconnectAllowed = true;

    const connect = async () => {
      setConnection(firstConnection ? "подключение" : "восстановление");
      if (!firstConnection) {
        recoveringRef.current = true;
        setDesynced(true);
        setLifecycleNotice(
          "Связь с сервером потеряна. Пытаемся восстановить соединение. На восстановление отведено до 20 секунд",
        );
      }
      try {
        let { ticket } = await issueTicket(session.csrfToken, room.roomId);
        if (disposed) return;
        const scheme = window.location.protocol === "https:" ? "wss:" : "ws:";
        const socket = new WebSocket(
          `${scheme}//${window.location.host}/api/v1/ws`,
        );
        socketRef.current = socket;
        socket.addEventListener("open", () => {
          socket.send(
            JSON.stringify({
              protocolVersion,
              type: "authenticate",
              correlationId: `auth-${crypto.randomUUID()}`,
              ticket,
              connectionId: `browser-${crypto.randomUUID()}`,
            }),
          );
          ticket = "";
          heartbeatTimer = window.setInterval(() => {
            if (socket.readyState === WebSocket.OPEN) {
              socket.send(
                JSON.stringify({
                  protocolVersion,
                  type: "heartbeat",
                  correlationId: `heartbeat-${crypto.randomUUID()}`,
                }),
              );
            }
          }, 1000);
        });
        socket.addEventListener("message", (message) => {
          let event: ServerEvent;
          try {
            event = JSON.parse(String(message.data)) as ServerEvent;
          } catch {
            recoveringRef.current = true;
            setDesynced(true);
            setError(
              "Сервер прислал сообщение неизвестного формата. Получаем актуальное состояние.",
            );
            socket.close();
            return;
          }
          if (event.protocolVersion !== protocolVersion) {
            reconnectAllowed = false;
            setDesynced(true);
            setError(
              "Версия клиента устарела. Обновите страницу, чтобы продолжить.",
            );
            socket.close();
            return;
          }
          if (event.type === "authenticated") {
            firstConnection = false;
            setConnection("подключено");
            setError("");
          } else if (event.type === "roomSnapshot") {
            onRoomSnapshot(event.payload);
          } else if (
            event.type === "matchSnapshot" ||
            event.type === "stateUpdate" ||
            event.type === "choiceRequested" ||
            event.type === "matchFinished"
          ) {
            const previous = projectionClockRef.current;
            const decision = acceptProjection(
              previous,
              event.payload,
              event.eventSeq,
            );
            if (!decision.accepted) return;
            projectionClockRef.current = decision.clock;
            setView(event.payload);
            setDesynced(false);
            if (event.type === "matchSnapshot" && recoveringRef.current) {
              setError("");
              setLifecycleNotice(
                "Соединение восстановлено. Состояние игры обновлено",
              );
              recoveringRef.current = false;
            }
          } else if (event.type === "lifecycleNotice") {
            if (event.payload.view) {
              const decision = acceptProjection(
                projectionClockRef.current,
                event.payload.view,
                event.eventSeq,
              );
              if (decision.accepted) {
                projectionClockRef.current = decision.clock;
                setView(event.payload.view);
                setDesynced(false);
              }
            }
            setLifecycleNotice(event.payload.message);
          } else if (event.type === "commandResult") {
            if (event.commandId === mulliganCommandRef.current)
              setMulliganSubmitted(true);
          } else if (event.type === "error") {
            if (event.error.view) {
              const decision = acceptProjection(
                projectionClockRef.current,
                event.error.view,
                event.eventSeq,
              );
              if (decision.accepted) {
                projectionClockRef.current = decision.clock;
                setView(event.error.view);
              }
            }
            setError(russianProtocolError(event.error));
            if (
              event.error.resyncRequired ||
              event.error.code === "CHOICE_EXPIRED"
            ) {
              recoveringRef.current = true;
              setDesynced(true);
              const current = latestViewRef.current;
              if (current && socket.readyState === WebSocket.OPEN) {
                socket.send(
                  JSON.stringify({
                    protocolVersion,
                    type: "command",
                    commandId: `resync-${crypto.randomUUID()}`,
                    matchId: current.matchId,
                    expectedRevision: current.revision,
                    name: "resync",
                    payload: { lastEventSeq: event.eventSeq },
                  }),
                );
              }
            }
          } else if (event.type === "connectionStatus") {
            setLifecycleNotice(
              event.payload.status === "connected"
                ? "Соединение восстановлено. Состояние игры обновлено"
                : event.payload.seatId === latestViewRef.current?.ownSeat
                  ? "Связь с сервером потеряна. Пытаемся восстановить соединение. На восстановление отведено до 20 секунд"
                  : "Связь с соперником потеряна. Ожидаем восстановление до 20 секунд",
            );
          } else if (event.type === "serverDraining") {
            setDesynced(true);
            setError(
              "Сервер временно недоступен. Попробуйте восстановить соединение позже.",
            );
          }
        });
        socket.addEventListener("close", () => {
          if (heartbeatTimer !== undefined)
            window.clearInterval(heartbeatTimer);
          if (!disposed) {
            if (reconnectAllowed) {
              setConnection("восстановление");
              recoveringRef.current = true;
              setDesynced(true);
              setLifecycleNotice(
                "Связь с сервером потеряна. Пытаемся восстановить соединение. На восстановление отведено до 20 секунд",
              );
              retryTimer = window.setTimeout(() => void connect(), 1200);
            } else {
              setConnection("нет");
            }
          }
        });
        socket.addEventListener("error", () => {
          setError(
            "Связь с сервером потеряна. Пытаемся восстановить соединение. На восстановление отведено до 20 секунд",
          );
        });
      } catch (caught) {
        if (!disposed) {
          setError(russianError(caught));
          retryTimer = window.setTimeout(() => void connect(), 1800);
        }
      }
    };

    void connect();
    return () => {
      disposed = true;
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
      if (heartbeatTimer !== undefined) window.clearInterval(heartbeatTimer);
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [onRoomSnapshot, room?.match?.matchId, room?.roomId, session]);

  useEffect(() => {
    const resyncOnReturn = () => {
      if (document.visibilityState !== "visible") return;
      const socket = socketRef.current;
      const current = latestViewRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN || !current) return;
      recoveringRef.current = true;
      setDesynced(true);
      socket.send(
        JSON.stringify({
          protocolVersion,
          type: "command",
          commandId: `resync-${crypto.randomUUID()}`,
          matchId: current.matchId,
          expectedRevision: current.revision,
          name: "resync",
          payload: {},
        }),
      );
    };
    document.addEventListener("visibilitychange", resyncOnReturn);
    return () =>
      document.removeEventListener("visibilitychange", resyncOnReturn);
  }, []);

  const send = useCallback(
    (command: MatchCommand) => {
      const socket = socketRef.current;
      const current = latestViewRef.current;
      if (
        !socket ||
        socket.readyState !== WebSocket.OPEN ||
        !current ||
        desynced ||
        (current.status === "finished" && command.name !== "requestRematch")
      )
        return false;
      const commandId = `${command.name}-${crypto.randomUUID()}`;
      if (command.name === "submitMulligan")
        mulliganCommandRef.current = commandId;
      socket.send(
        JSON.stringify({
          protocolVersion,
          type: "command",
          commandId,
          matchId: current.matchId,
          expectedRevision: current.revision,
          name: command.name,
          payload: command.payload,
        }),
      );
      return true;
    },
    [desynced],
  );

  return {
    connection,
    desynced,
    error,
    lifecycleNotice,
    mulliganSubmitted,
    send,
    setError,
    view,
  };
}

export function EntryScreen({
  busy,
  code,
  error,
  onCodeChange,
  onCreate,
  onJoin,
}: {
  busy: boolean;
  code: string;
  error: string;
  onCodeChange: (code: string) => void;
  onCreate: () => void;
  onJoin: () => void;
}) {
  return (
    <main className="entry-shell">
      <section className="entry-card" aria-labelledby="page-title">
        <div className="brand-mark" aria-hidden="true">
          Р
        </div>
        <p className="eyebrow">Карточная дуэль для двоих</p>
        <h1 id="page-title">Респонс Онлайн</h1>
        <p className="lead">
          Создайте приватную комнату или введите код приглашения. Регистрация не
          нужна.
        </p>
        {error && <Notice kind="error">{error}</Notice>}
        <button className="primary wide" disabled={busy} onClick={onCreate}>
          {busy ? "Подготавливаем сессию…" : "Создать приватную комнату"}
        </button>
        <div className="divider">
          <span>или</span>
        </div>
        <form
          className="join-form"
          onSubmit={(event) => {
            event.preventDefault();
            onJoin();
          }}
        >
          <label htmlFor="room-code">Код комнаты</label>
          <div className="join-row">
            <input
              id="room-code"
              autoComplete="off"
              autoCapitalize="characters"
              inputMode="text"
              maxLength={32}
              value={code}
              onChange={(event) =>
                onCodeChange(event.target.value.toUpperCase())
              }
              placeholder="Например, AB34CD56EFG23"
              aria-describedby="code-help"
            />
            <button
              className="secondary"
              disabled={busy || !isValidRoomCode(code)}
              type="submit"
            >
              Войти
            </button>
          </div>
          <p className="help" id="code-help">
            Код можно ввести вручную. Буфер обмена не требуется.
          </p>
        </form>
        <p className="privacy-note">
          Ссылка содержит только код комнаты. Данные сессии остаются в
          защищённом хранилище браузера.
        </p>
      </section>
    </main>
  );
}

function Notice({
  children,
  kind = "info",
}: {
  children: ReactNode;
  kind?: "info" | "error" | "success";
}) {
  return (
    <div
      className={`notice ${kind}`}
      role={kind === "error" ? "alert" : "status"}
    >
      {children}
    </div>
  );
}

export function DeckPicker({
  currentDeckId,
  disabled,
  onChoose,
  onInspect,
  inspectedDeck,
}: {
  currentDeckId: string;
  disabled: boolean;
  onChoose: (deck: Deck) => void;
  onInspect: (deck: Deck) => void;
  inspectedDeck: Deck;
}) {
  return (
    <section className="lobby-panel deck-panel" aria-labelledby="deck-title">
      <div className="section-heading">
        <div>
          <p className="step">Шаг 1</p>
          <h2 id="deck-title">Выберите колоду</h2>
        </div>
        <span className="count-badge">6 колод</span>
      </div>
      <div className="deck-layout">
        <div className="deck-grid" aria-label="Доступные колоды">
          {decks.map((deck) => (
            <button
              className={`deck-tile faction-${deck.deckId.split("-")[0]} ${inspectedDeck.deckId === deck.deckId ? "inspected" : ""}`}
              key={deck.deckId}
              onClick={() => onInspect(deck)}
              aria-pressed={inspectedDeck.deckId === deck.deckId}
            >
              <span className="deck-hero-art">
                <LobbyCardImage definitionId={deck.hero.definitionId} />
              </span>
              <span className="deck-name">{deck.displayName}</span>
              <span className="deck-faction">{deck.factionName}</span>
              {currentDeckId === deck.deckId && (
                <span className="selected-label">Выбрана</span>
              )}
            </button>
          ))}
        </div>
        <DeckPreview
          deck={inspectedDeck}
          selected={currentDeckId === inspectedDeck.deckId}
          disabled={disabled}
          onChoose={() => onChoose(inspectedDeck)}
        />
      </div>
    </section>
  );
}

function DeckPreview({
  deck,
  disabled,
  onChoose,
  selected,
}: {
  deck: Deck;
  disabled: boolean;
  onChoose: () => void;
  selected: boolean;
}) {
  const [openCard, setOpenCard] = useState<ConfirmedCard | null>(null);
  const cards = [deck.hero, ...deck.orders, ...deck.main];
  const hero = confirmedCard(deck.hero);
  return (
    <article
      className="deck-preview"
      aria-label={`Состав колоды ${deck.displayName}`}
    >
      <header>
        <button
          className="deck-preview-hero"
          onClick={() => setOpenCard(hero)}
          aria-label={`Открыть карту героя ${hero.name}`}
        >
          <LobbyCardImage definitionId={deck.hero.definitionId} />
        </button>
        <div>
          <p>{deck.factionName}</p>
          <h3>{deck.displayName}</h3>
          <span>30 карт · 5 Приказов · 1 герой</span>
        </div>
      </header>
      <div className="card-list" aria-label="Подтверждённый состав">
        {cards.map((card) => {
          const details = confirmedCard(card);
          return (
            <button
              className="card-row"
              style={cardFactionStyle(details)}
              key={card.definitionId}
              onClick={() => setOpenCard(details)}
              aria-label={`Открыть карту ${details.name}, ${card.count} копий, стоимость ${details.cost ?? "не указана"}, ${categoryNames[card.category] ?? card.category}`}
            >
              <span className="card-count">{card.count}×</span>
              <span className="card-row-name">{details.name}</span>
              <span className="card-cost" aria-label="Стоимость">
                {details.cost ?? "—"}
              </span>
              <span className="card-kind">
                {categoryNames[card.category] ?? card.category}
              </span>
            </button>
          );
        })}
      </div>
      <button
        className="primary wide"
        disabled={disabled || selected}
        onClick={onChoose}
      >
        {selected ? "Эта колода выбрана" : "Выбрать эту колоду"}
      </button>
      <p className="verified-note">
        Показаны только подтверждённые данные утверждённого состава.
      </p>
      {openCard && (
        <CardDialog card={openCard} onClose={() => setOpenCard(null)} />
      )}
    </article>
  );
}

function CardDialog({
  card,
  onClose,
}: {
  card: ConfirmedCard;
  onClose: () => void;
}) {
  const closeButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    closeButton.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        className="card-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="card-dialog-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button
          ref={closeButton}
          className="icon-button dialog-close"
          onClick={onClose}
          aria-label="Закрыть просмотр карты"
        >
          ×
        </button>
        <p className="step">Подтверждённые данные</p>
        <h2 id="card-dialog-title">{card.name}</h2>
        <div className="card-dialog-layout">
          <div className="card-dialog-art" style={cardFactionStyle(card)}>
            <LobbyCardImage definitionId={card.definitionId} size="medium" />
          </div>
          <div className="card-dialog-copy">
            <p>{[...card.types, ...card.subtypes].join(" · ")}</p>
            <dl className="card-facts">
              {card.faction && (
                <>
                  <dt>Фракция</dt>
                  <dd>{card.faction}</dd>
                </>
              )}
              {card.rarity && (
                <>
                  <dt>Редкость</dt>
                  <dd>{card.rarity}</dd>
                </>
              )}
              {card.cost !== undefined && (
                <>
                  <dt>Стоимость</dt>
                  <dd>{card.cost}</dd>
                </>
              )}
              {card.influence !== undefined && (
                <>
                  <dt>Влияние</dt>
                  <dd>{card.influence}</dd>
                </>
              )}
              {card.strength !== undefined && (
                <>
                  <dt>Сила</dt>
                  <dd>{card.strength}</dd>
                </>
              )}
              {card.durability !== undefined && (
                <>
                  <dt>Прочность</dt>
                  <dd>{card.durability}</dd>
                </>
              )}
            </dl>
            {card.abilityText.map((text) => (
              <p className="ability-text" key={text}>
                {text}
              </p>
            ))}
          </div>
        </div>
        <button className="secondary wide" onClick={onClose}>
          Закрыть
        </button>
      </section>
    </div>
  );
}

export function OrderEditor({
  busy,
  onReorder,
  orderIds,
}: {
  busy: boolean;
  onReorder: (next: string[]) => void;
  orderIds: string[];
}) {
  const [announcement, setAnnouncement] = useState("");
  const [dragging, setDragging] = useState<number | null>(null);
  const [openCard, setOpenCard] = useState<ConfirmedCard | null>(null);
  const move = (index: number, direction: -1 | 1) => {
    const next = moveItem(orderIds, index, direction);
    if (next === orderIds) return;
    const destination = index + direction;
    setAnnouncement(
      `${cardName(orderIds[index])}: позиция ${destination + 1} из 5`,
    );
    onReorder(next);
  };
  const drop = (destination: number) => {
    if (dragging === null) return;
    const next = reorderItem(orderIds, dragging, destination);
    const moved = orderIds[dragging];
    setDragging(null);
    if (next === orderIds) return;
    setAnnouncement(`${cardName(moved)}: позиция ${destination + 1} из 5`);
    onReorder(next);
  };
  return (
    <section className="lobby-panel order-panel" aria-labelledby="order-title">
      <div className="section-heading">
        <div>
          <p className="step">Шаг 2</p>
          <h2 id="order-title">Порядок Приказов</h2>
        </div>
      </div>
      <p className="section-copy">
        Перетаскивайте весь блок Приказа или его изображение: первый Приказ
        находится сверху. Для клавиатуры используйте Alt со стрелками.
      </p>
      <ol className="order-list">
        {orderIds.map((orderId, index) => (
          <li
            key={orderId}
            tabIndex={0}
            draggable={!busy}
            className={dragging === index ? "dragging" : ""}
            style={cardFactionStyle(confirmedCardById(orderId))}
            onDragStart={(event) => {
              setDragging(index);
              event.dataTransfer.effectAllowed = "move";
              event.dataTransfer.setData("text/plain", orderId);
            }}
            onDragEnd={() => setDragging(null)}
            onDragOver={(event) => {
              if (!busy) event.preventDefault();
            }}
            onDrop={(event) => {
              event.preventDefault();
              drop(index);
            }}
            onKeyDown={(event) => {
              if (!event.altKey) return;
              if (event.key === "ArrowUp") {
                event.preventDefault();
                move(index, -1);
              }
              if (event.key === "ArrowDown") {
                event.preventDefault();
                move(index, 1);
              }
            }}
          >
            <span className="order-number">{index + 1}</span>
            <button
              className="order-card-preview"
              draggable={!busy}
              onDragStart={(event) => {
                setDragging(index);
                event.dataTransfer.effectAllowed = "move";
                event.dataTransfer.setData("text/plain", orderId);
              }}
              onDragEnd={() => setDragging(null)}
              onClick={() => setOpenCard(confirmedCardById(orderId))}
              aria-label={`Открыть Приказ ${cardName(orderId)}`}
            >
              <LobbyCardImage definitionId={orderId} />
            </button>
            <span className="order-name">{cardName(orderId)}</span>
            <span className="order-drag-handle" aria-hidden="true">
              ⋮⋮
            </span>
          </li>
        ))}
      </ol>
      <p className="visually-hidden" aria-live="polite">
        {announcement}
      </p>
      {openCard && (
        <CardDialog card={openCard} onClose={() => setOpenCard(null)} />
      )}
    </section>
  );
}

export function LobbyScreen({
  busy,
  error,
  onChooseDeck,
  onReady,
  onReorder,
  room,
}: {
  busy: boolean;
  error: string;
  onChooseDeck: (deck: Deck) => void;
  onReady: (ready: boolean) => void;
  onReorder: (orderIds: string[]) => void;
  room: RoomSnapshot;
}) {
  const selectedDeck = findDeck(room.ownSelection.deckId);
  const [inspectedDeck, setInspectedDeck] = useState(selectedDeck ?? decks[0]);
  const [copied, setCopied] = useState(false);
  const [clipboardAvailable, setClipboardAvailable] = useState(false);
  const opponent = opponentSeat(room);
  const me = ownSeat(room);
  const invite = invitationUrl(room.code, window.location.href);

  useEffect(
    () => setClipboardAvailable(Boolean(navigator.clipboard?.writeText)),
    [],
  );
  const copyInvite = async () => {
    try {
      await navigator.clipboard.writeText(invite);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <main className="app-shell">
      <header className="topbar">
        <a className="brand" href="/" aria-label="Респонс Онлайн, на главную">
          <span aria-hidden="true">Р</span> Респонс Онлайн
        </a>
        <div className="room-chip">
          <span>Комната</span>
          <strong>{room.code}</strong>
        </div>
      </header>
      <div className="lobby-wrap">
        <section className="invite-bar" aria-labelledby="invite-title">
          <div>
            <p className="step">Приватная комната</p>
            <h1 id="invite-title">Пригласите соперника</h1>
          </div>
          <div className="invite-fields">
            <label>
              <span>Код</span>
              <input
                readOnly
                value={room.code}
                onFocus={(event) => event.currentTarget.select()}
              />
            </label>
            <label className="invite-link">
              <span>Ссылка</span>
              <input
                readOnly
                value={invite}
                onFocus={(event) => event.currentTarget.select()}
              />
            </label>
            {clipboardAvailable && (
              <button className="secondary" onClick={() => void copyInvite()}>
                {copied ? "Скопировано" : "Копировать ссылку"}
              </button>
            )}
          </div>
          <div
            className={`opponent-state ${opponent?.occupied ? "online" : "waiting"}`}
            role="status"
          >
            <span aria-hidden="true">{opponent?.occupied ? "✓" : "…"}</span>
            {opponent?.occupied
              ? "Соперник подключился"
              : "Ожидаем второго игрока"}
          </div>
        </section>
        {error && <Notice kind="error">{error}</Notice>}
        <div className="lobby-columns">
          <DeckPicker
            currentDeckId={room.ownSelection.deckId}
            disabled={busy}
            inspectedDeck={inspectedDeck}
            onInspect={setInspectedDeck}
            onChoose={onChooseDeck}
          />
          <aside className="setup-column">
            {selectedDeck ? (
              <OrderEditor
                busy={busy}
                orderIds={
                  room.ownSelection.orderIds ??
                  selectedDeck.orders.map((order) => order.definitionId)
                }
                onReorder={onReorder}
              />
            ) : (
              <section
                className="lobby-panel empty-panel"
                aria-labelledby="order-empty-title"
              >
                <p className="step">Шаг 2</p>
                <h2 id="order-empty-title">Порядок Приказов</h2>
                <p>Сначала выберите колоду — здесь появятся пять Приказов.</p>
              </section>
            )}
            <section
              className="lobby-panel ready-panel"
              aria-labelledby="ready-title"
            >
              <p className="step">Шаг 3</p>
              <h2 id="ready-title">Готовность</h2>
              <ul className="player-statuses">
                <li>
                  <span>Вы</span>
                  <strong>{me?.ready ? "Готовы" : "Настраиваете"}</strong>
                </li>
                <li>
                  <span>Соперник</span>
                  <strong>
                    {!opponent?.occupied
                      ? "Не подключён"
                      : opponent.ready
                        ? "Готов"
                        : "Настраивает"}
                  </strong>
                </li>
              </ul>
              <button
                className={me?.ready ? "secondary wide" : "primary wide"}
                disabled={busy || !selectedDeck || !opponent?.occupied}
                onClick={() => onReady(!me?.ready)}
              >
                {me?.ready ? "Изменить выбор" : "Я готов"}
              </button>
              {!opponent?.occupied && (
                <p className="help">
                  Готовность станет доступна после входа соперника.
                </p>
              )}
              {me?.ready && (
                <p className="help">
                  Изменение колоды или порядка автоматически снимет готовность.
                </p>
              )}
            </section>
          </aside>
        </div>
      </div>
    </main>
  );
}

export function MatchScreen({
  connection,
  desynced = false,
  error,
  lifecycleNotice = "",
  mulliganSubmitted,
  onMulligan,
  onCommand,
  room,
  view,
}: {
  connection: ConnectionState;
  desynced?: boolean;
  error: string;
  lifecycleNotice?: string;
  mulliganSubmitted: boolean;
  onMulligan: (
    refs: NonNullable<PlayerView["players"][number]["hand"]>,
  ) => void;
  onCommand: (command: MatchCommand) => void;
  room: RoomSnapshot;
  view: PlayerView | null;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const own = view?.players.find((player) => player.seat === view.ownSeat);
  const opponent = view?.players.find((player) => player.seat !== view.ownSeat);
  const ownInitiative = view?.initiativeSeat === view?.ownSeat;

  if (!view) {
    return (
      <main className="match-shell center-state">
        <div className="spinner" aria-hidden="true" />
        <h1>Подготавливаем матч</h1>
        <p role="status">
          {connection === "восстановление"
            ? "Восстанавливаем соединение…"
            : "Получаем безопасное состояние игры…"}
        </p>
        {error && <Notice kind="error">{error}</Notice>}
      </main>
    );
  }

  if (view.status === "active" || view.status === "finished") {
    return (
      <GameplayScreen
        actionsBlocked={connection !== "подключено" || desynced}
        connectionLabel={
          connection === "подключено"
            ? "Связь установлена"
            : connection === "подключение"
              ? "Подключаемся"
              : connection === "восстановление"
                ? "Восстанавливаем связь"
                : "Связь недоступна"
        }
        desynced={desynced}
        error={error}
        lifecycleNotice={lifecycleNotice}
        onCommand={onCommand}
        room={room}
        view={view}
      />
    );
  }

  return (
    <main className="match-shell">
      <header className="match-header">
        <div>
          <p className="eyebrow">Подготовка матча</p>
          <h1>Респонс Онлайн</h1>
        </div>
        <div
          className={`connection ${connection === "подключено" ? "online" : "waiting"}`}
          role="status"
        >
          <span aria-hidden="true" />
          {connection === "подключено"
            ? "Связь установлена"
            : "Восстанавливаем связь"}
        </div>
      </header>
      {error && <Notice kind="error">{error}</Notice>}
      <section
        className={`initiative ${ownInitiative ? "own" : "opponent"}`}
        aria-labelledby="initiative-title"
      >
        <div className="initiative-icon" aria-hidden="true">
          ✦
        </div>
        <div>
          <p>Инициатива</p>
          <h2 id="initiative-title">
            {ownInitiative ? "Первый ход за вами" : "Первый ход за соперником"}
          </h2>
        </div>
      </section>
      {view.phase === "order_selection" && (
        <section className="preparation-card" aria-live="polite">
          <div className="spinner small" aria-hidden="true" />
          <div>
            <h2>Фиксируем порядок Приказов</h2>
            <p>Ожидаем подтверждение обоих игроков.</p>
          </div>
        </section>
      )}
      {view.phase === "mulligan" && own && (
        <section className="mulligan-panel" aria-labelledby="mulligan-title">
          <div className="section-heading">
            <div>
              <p className="step">Начальная рука</p>
              <h2 id="mulligan-title">Выберите карты для замены</h2>
            </div>
            <span className="count-badge">
              {mulliganSubmitted
                ? "Выбор отправлен"
                : `${selected.length} выбрано`}
            </span>
          </div>
          <p className="section-copy">
            {mulliganSubmitted
              ? "Показана ваша итоговая рука после замены."
              : "Можно заменить любое число карт или оставить руку без изменений."}
          </p>
          <div className="hand" aria-label="Ваша начальная рука">
            {(own.hand ?? []).map((card) => {
              const checked = selected.includes(card.objectRef.zoneObjectId);
              return (
                <label
                  className={`hand-card ${checked && !mulliganSubmitted ? "selected" : ""}`}
                  key={card.objectRef.zoneObjectId}
                >
                  <input
                    type="checkbox"
                    disabled={mulliganSubmitted}
                    checked={!mulliganSubmitted && checked}
                    onChange={() =>
                      setSelected((current) =>
                        checked
                          ? current.filter(
                              (id) => id !== card.objectRef.zoneObjectId,
                            )
                          : [...current, card.objectRef.zoneObjectId],
                      )
                    }
                  />
                  <span className="hand-art">
                    <LobbyCardImage definitionId={card.definitionId} />
                  </span>
                  <strong>{cardName(card.definitionId)}</strong>
                  <span>
                    {mulliganSubmitted
                      ? "Итоговая рука"
                      : checked
                        ? "Будет заменена"
                        : "Оставить"}
                  </span>
                </label>
              );
            })}
          </div>
          {mulliganSubmitted ? (
            <Notice kind="success">
              Выбор принят. Ожидаем решение соперника — его рука и выбор
              остаются скрыты.
            </Notice>
          ) : (
            <button
              className="primary mulligan-submit"
              onClick={() =>
                onMulligan(
                  (own.hand ?? []).filter((card) =>
                    selected.includes(card.objectRef.zoneObjectId),
                  ),
                )
              }
            >
              {selected.length === 0
                ? "Оставить эту руку"
                : `Заменить карты: ${selected.length}`}
            </button>
          )}
          <aside className="opponent-private">
            <span className="private-icon" aria-hidden="true">
              ●
            </span>
            <div>
              <strong>Рука соперника скрыта</strong>
              <p>
                Карт в руке: {opponent?.handCount ?? 0}. Решение о замене не
                показывается.
              </p>
            </div>
          </aside>
        </section>
      )}
      <footer className="match-footer">
        Комната {room.code} · Скрытые данные соперника не передаются интерфейсу
      </footer>
    </main>
  );
}

export function App() {
  const [session, setSession] = useState<Session | null>(null);
  const [room, setRoom] = useState<RoomSnapshot | null>(null);
  const [code, setCode] = useState(() =>
    inviteCodeFromUrl(window.location.href),
  );
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const match = useMatchConnection(session, room, setRoom);

  useEffect(() => {
    const controller = new AbortController();
    void createSession(controller.signal)
      .then(setSession)
      .catch((caught: unknown) => {
        if (!(caught instanceof DOMException && caught.name === "AbortError")) {
          setError(russianError(caught));
        }
      })
      .finally(() => setBusy(false));
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!room || room.match) return;
    const timer = window.setInterval(() => {
      void getRoom(room.roomId)
        .then((snapshot) => {
          setRoom(snapshot);
          setError("");
        })
        .catch((caught) => setError(russianError(caught)));
    }, 1500);
    return () => window.clearInterval(timer);
  }, [room?.match, room?.roomId]);

  const run = async (action: () => Promise<RoomSnapshot>) => {
    setBusy(true);
    setError("");
    try {
      setRoom(await action());
    } catch (caught) {
      setError(russianError(caught));
    } finally {
      setBusy(false);
    }
  };

  const selectedDeck = useMemo(
    () => findDeck(room?.ownSelection.deckId ?? ""),
    [room?.ownSelection.deckId],
  );
  const autoOrderRevision = useRef<number | null>(null);
  const { send, view } = match;
  useEffect(() => {
    if (!view || view.phase !== "order_selection" || !room || !selectedDeck)
      return;
    const player = view.players.find((item) => item.seat === view.ownSeat);
    if (player?.orders.length || autoOrderRevision.current === view.revision)
      return;
    autoOrderRevision.current = view.revision;
    const orderIds =
      room.ownSelection.orderIds ??
      selectedDeck.orders.map((item) => item.definitionId);
    if (!send({ name: "setOrderArrangement", payload: { orderIds } }))
      autoOrderRevision.current = null;
  }, [room, selectedDeck, send, view]);

  if (!room) {
    return (
      <EntryScreen
        busy={busy}
        code={code}
        error={error}
        onCodeChange={(value) => setCode(normalizeRoomCode(value))}
        onCreate={() =>
          session && void run(() => createRoom(session.csrfToken))
        }
        onJoin={() =>
          session && void run(() => joinRoom(session.csrfToken, code))
        }
      />
    );
  }
  if (room.match) {
    return (
      <MatchScreen
        connection={match.connection}
        desynced={match.desynced}
        error={match.error}
        lifecycleNotice={match.lifecycleNotice}
        mulliganSubmitted={match.mulliganSubmitted}
        room={room}
        view={match.view}
        onMulligan={(cards) => {
          match.setError("");
          match.send({
            name: "submitMulligan",
            payload: { objectRefs: cards.map((card) => card.objectRef) },
          });
        }}
        onCommand={(command) => {
          match.setError("");
          match.send(command);
        }}
      />
    );
  }
  return (
    <LobbyScreen
      busy={busy}
      error={error}
      room={room}
      onChooseDeck={(deck) => {
        if (session)
          void run(() =>
            setSelection(
              session.csrfToken,
              room.roomId,
              deck.deckId,
              deck.orders.map((order) => order.definitionId),
            ),
          );
      }}
      onReorder={(orderIds) => {
        if (session && selectedDeck)
          void run(() =>
            setSelection(
              session.csrfToken,
              room.roomId,
              selectedDeck.deckId,
              orderIds,
            ),
          );
      }}
      onReady={(ready) => {
        if (session)
          void run(() => setReady(session.csrfToken, room.roomId, ready));
      }}
    />
  );
}
