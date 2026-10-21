import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import type { CSSProperties, DragEvent } from "react";

import type { MatchCommand } from "./api";
import {
  cardImageUrl,
  cardName,
  confirmedCard,
  type CatalogCard,
} from "./catalog";
import {
  composeDraftCommand,
  type ActionDraft,
  draftReducer,
  phaseName,
  resultText,
} from "./gameplay";
import type {
  ObjectRef,
  PlayerView,
  PlayerViewAction,
  PlayerViewChoice,
  RoomSnapshot,
  VisibleCard,
} from "./protocol";

const sameRef = (left: ObjectRef, right: ObjectRef) =>
  left.zoneObjectId === right.zoneObjectId &&
  left.generation === right.generation;

function displayCard(definitionId: string) {
  return confirmedCard({
    definitionId,
    name: cardName(definitionId),
    category: "",
    count: 1,
  } satisfies CatalogCard);
}

function findVisibleCard(view: PlayerView, ref: ObjectRef) {
  for (const player of view.players) {
    const card = [...(player.hand ?? []), ...player.graveyard].find((item) =>
      sameRef(item.objectRef, ref),
    );
    if (card) return card;
  }
  return view.board.find((item) => sameRef(item.objectRef, ref));
}

function seatText(view: PlayerView, seat: string) {
  return seat === view.ownSeat ? "Вы" : "Соперник";
}

function useDialogClose(onClose: () => void) {
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    closeRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);
  return closeRef;
}

function CardButton({
  card,
  compact = false,
  onOpen,
  onPlay,
  selected = false,
  draggable = false,
  onDragAction,
  onDragActionEnd,
}: {
  card: VisibleCard;
  compact?: boolean;
  onOpen?: (card: VisibleCard) => void;
  onPlay?: (card: VisibleCard) => void;
  selected?: boolean;
  draggable?: boolean;
  onDragAction?: (card: VisibleCard) => void;
  onDragActionEnd?: () => void;
}) {
  return (
    <button
      className={`game-card ${compact ? "compact" : ""} ${card.tapped ? "tapped" : ""} ${selected ? "selected" : ""}`}
      onClick={() => (onPlay ? onPlay(card) : onOpen?.(card))}
      onDoubleClick={() => onOpen?.(card)}
      draggable={draggable}
      onDragStart={(event) => {
        event.dataTransfer.setData(
          "application/x-response-card",
          card.objectRef.zoneObjectId,
        );
        event.dataTransfer.effectAllowed = "move";
        onDragAction?.(card);
      }}
      onDragEnd={onDragActionEnd}
      aria-label={`${onPlay ? "Выбрать для розыгрыша" : "Открыть"} карту ${cardName(card.definitionId)}`}
      aria-pressed={onPlay ? selected : undefined}
    >
      <CardArtwork definitionId={card.definitionId} />
      {(card.power !== undefined || card.durability !== undefined) && (
        <span className="game-card-stats">
          <b>{card.power ?? "—"}</b>
          <span aria-hidden="true"> / </span>
          <b>{card.durability ?? "—"}</b>
        </span>
      )}
    </button>
  );
}

function CardArtwork({
  definitionId,
  large = false,
}: {
  definitionId: string;
  large?: boolean;
}) {
  const [failed, setFailed] = useState(false);
  const source = cardImageUrl(definitionId, large ? "medium" : "thumb");
  const name = cardName(definitionId);
  if (!source || failed) {
    return (
      <span
        className="card-image-fallback"
        role="img"
        aria-label={`Изображение карты ${name} недоступно`}
      >
        <span>{name.slice(0, 1)}</span>
        <strong>{name}</strong>
      </span>
    );
  }
  return (
    <span className="card-artwork">
      <span className="card-image-loading" aria-hidden="true">
        Загрузка…
      </span>
      <img
        src={source}
        alt={name}
        loading={large ? "eager" : "lazy"}
        onError={() => setFailed(true)}
      />
    </span>
  );
}

function EmptySlot({
  enabled = false,
  label,
  onActivate,
}: {
  enabled?: boolean;
  label: string;
  onActivate?: (event?: DragEvent<HTMLButtonElement>) => void;
}) {
  if (!enabled || !onActivate)
    return <span className="empty-slot">{label}</span>;
  return (
    <button
      className="empty-slot drop-enabled"
      type="button"
      onClick={() => onActivate()}
      onDragOver={(event) => {
        event.preventDefault();
        event.dataTransfer.dropEffect = "move";
      }}
      onDrop={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onActivate(event);
      }}
      aria-label={`${label}: разыграть сюда выбранную карту`}
    >
      {label}
    </button>
  );
}

function OrderCard({
  definitionId,
  hidden,
  onOpen,
}: {
  definitionId?: string;
  hidden?: boolean;
  onOpen?: (definitionId: string) => void;
}) {
  return hidden || !definitionId ? (
    <span className="order-card card-back" aria-label="Закрытый приказ">
      <span>R</span>
    </span>
  ) : (
    <button
      className="order-card"
      aria-label={`Приказ ${cardName(definitionId)}`}
      onClick={() => onOpen?.(definitionId)}
    >
      <CardArtwork definitionId={definitionId} />
    </button>
  );
}

function HiddenHand({ count }: { count: number }) {
  return (
    <div className="hidden-hand" aria-label={`В руке соперника ${count} карт`}>
      {Array.from({ length: Math.min(count, 10) }, (_, index) => (
        <span
          className="card-back"
          key={index}
          style={{ "--fan": index } as CSSProperties}
        >
          <span>R</span>
        </span>
      ))}
      <span className="count-badge">{count}</span>
    </div>
  );
}

function ZoneButton({
  cards,
  label,
  onOpen,
}: {
  cards: VisibleCard[];
  label: string;
  onOpen: () => void;
}) {
  return (
    <button
      className="zone-pile"
      onClick={onOpen}
      aria-label={`${label}: ${cards.length} карт. Открыть галерею`}
    >
      {cards[0] ? (
        <CardArtwork definitionId={cards[cards.length - 1].definitionId} />
      ) : (
        <span className="zone-empty" />
      )}
      <strong>{label}</strong>
      <span className="count-badge">{cards.length}</span>
    </button>
  );
}

function ZoneGallery({
  title,
  cards,
  onClose,
  onCard,
}: {
  title: string;
  cards: VisibleCard[];
  onClose: () => void;
  onCard: (card: VisibleCard) => void;
}) {
  const closeRef = useDialogClose(onClose);
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        className="zone-gallery"
        role="dialog"
        aria-modal="true"
        aria-labelledby="gallery-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button
          ref={closeRef}
          className="icon-button dialog-close"
          onClick={onClose}
          aria-label="Закрыть галерею"
        >
          ×
        </button>
        <p className="step">Открытая игровая зона</p>
        <h2 id="gallery-title">
          {title} · {cards.length}
        </h2>
        <div className="gallery-grid">
          {cards.map((card) => (
            <CardButton
              card={card}
              key={card.objectRef.zoneObjectId}
              onOpen={onCard}
            />
          ))}
        </div>
        {cards.length === 0 && (
          <p className="help">В этой зоне пока нет карт.</p>
        )}
      </section>
    </div>
  );
}

function CardZoom({
  card,
  onClose,
}: {
  card: Pick<VisibleCard, "definitionId" | "power" | "durability">;
  onClose: () => void;
}) {
  const details = displayCard(card.definitionId);
  const closeRef = useDialogClose(onClose);
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        className="card-dialog gameplay-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="zoom-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button
          ref={closeRef}
          className="icon-button dialog-close"
          onClick={onClose}
          aria-label="Закрыть просмотр карты"
        >
          ×
        </button>
        <p className="step">Увеличенная карта</p>
        <h2 id="zoom-title">{details.name}</h2>
        <div className="zoom-layout">
          <CardArtwork definitionId={card.definitionId} large />
          <div className="zoom-copy">
            <p>{[...details.types, ...details.subtypes].join(" · ")}</p>
            <dl className="card-facts">
              {details.cost !== undefined && (
                <>
                  <dt>Стоимость</dt>
                  <dd>{details.cost}</dd>
                </>
              )}
              {details.strength !== undefined && (
                <>
                  <dt>Сила</dt>
                  <dd>{card.power ?? details.strength}</dd>
                </>
              )}
              {details.durability !== undefined && (
                <>
                  <dt>Прочность</dt>
                  <dd>{card.durability ?? details.durability}</dd>
                </>
              )}
            </dl>
            {details.abilityText.map((text) => (
              <p className="ability-text" key={text}>
                {text}
              </p>
            ))}
          </div>
        </div>
        <p className="verified-note">
          Показаны подтверждённые русские поля закреплённой версии контента.
        </p>
        <button className="secondary wide" onClick={onClose}>
          Закрыть
        </button>
      </section>
    </div>
  );
}

function HelpDialog({ onClose }: { onClose: () => void }) {
  const closeRef = useDialogClose(onClose);
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        className="help-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="help-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button
          ref={closeRef}
          className="icon-button dialog-close"
          onClick={onClose}
          aria-label="Закрыть справку"
        >
          ×
        </button>
        <h2 id="help-title">Справка по игре</h2>
        <h3>Управление</h3>
        <p>
          Карту можно перетащить из руки в центр стола или выбрать кликом, а
          затем нажать на область розыгрыша. Escape отменяет незавершённый ввод.
        </p>
        <h3>Ход и решения</h3>
        <p>
          Сервер показывает доступные действия. До отправки можно изменить выбор
          или нажать «Отменить ввод». После подтверждения команда окончательна.
        </p>
        <h3>Связь</h3>
        <p>
          При рассинхронизации действия блокируются до свежего снимка. После
          возврата во вкладку клиент заново запрашивает персональное состояние.
        </p>
        <h3>Результат</h3>
        <p>
          Игровая победа и техническое завершение различаются. Техническое
          завершение может не иметь победителя.
        </p>
        <h3>Ключевые слова</h3>
        <dl className="keyword-list">
          <dt>Призыв</dt>
          <dd>Эффект при появлении карты согласно её тексту.</dd>
          <dt>Обход</dt>
          <dd>Особое взаимодействие в бою; итог определяет сервер.</dd>
          <dt>Стойкость</dt>
          <dd>Защитное свойство, применяемое сервером при разрешении боя.</dd>
          <dt>Магический щит</dt>
          <dd>Защита с серверно предложенной оплатой.</dd>
          <dt>Натиск</dt>
          <dd>Свойство вступления в бой в текущем ходу.</dd>
          <dt>Яд</dt>
          <dd>Боевой эффект, полностью рассчитываемый сервером.</dd>
          <dt>Дальняя атака</dt>
          <dd>Особый порядок взаимодействия в линии боя.</dd>
          <dt>Активация</dt>
          <dd>Отдельное действие карты, когда сервер его предлагает.</dd>
        </dl>
        <button className="secondary wide" onClick={onClose}>
          Понятно
        </button>
      </section>
    </div>
  );
}

function SurrenderDialog({
  onCancel,
  onConfirm,
}: {
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const cancelRef = useDialogClose(onCancel);
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onCancel}>
      <section
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="surrender-title"
        onMouseDown={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if (event.key !== "Tab") return;
          const buttons = Array.from(
            event.currentTarget.querySelectorAll<HTMLButtonElement>("button"),
          );
          const first = buttons[0];
          const last = buttons.at(-1);
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last?.focus();
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first?.focus();
          }
        }}
      >
        <h2 id="surrender-title">Подтвердить сдачу?</h2>
        <p>
          Сервер немедленно зафиксирует поражение. После отправки отменить сдачу
          нельзя.
        </p>
        <div className="dialog-actions">
          <button ref={cancelRef} className="secondary" onClick={onCancel}>
            Продолжить матч
          </button>
          <button className="danger-button" onClick={onConfirm}>
            Сдаться
          </button>
        </div>
      </section>
    </div>
  );
}

function ChoicePanel({
  choice,
  disabled,
  onSubmit,
  view,
}: {
  choice: PlayerViewChoice;
  disabled: boolean;
  onSubmit: (ids: string[]) => void;
  view: PlayerView;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const minimum = choice.minSelections ?? (choice.mandatory ? 1 : 0);
  const maximum = choice.maxSelections ?? choice.optionCount ?? 0;
  if (choice.redacted || !choice.choiceId) {
    return (
      <section className="choice-panel waiting-choice" aria-live="assertive">
        <p className="step">Решение имеет приоритет</p>
        <h2>Ожидаем решение соперника</h2>
        <p>Закрытые варианты и причина ожидания не раскрываются.</p>
      </section>
    );
  }
  if (choice.waitingSeat !== view.ownSeat) {
    return (
      <section className="choice-panel waiting-choice" aria-live="assertive">
        <p className="step">Решение имеет приоритет</p>
        <h2>Ожидаем решение соперника</h2>
        <p>Партия продолжится после подтверждения на стороне соперника.</p>
      </section>
    );
  }
  const ready = selected.length >= minimum && selected.length <= maximum;
  return (
    <section
      className="choice-panel"
      aria-labelledby="choice-title"
      aria-live="assertive"
    >
      <p className="step">Решение имеет приоритет</p>
      <h2 id="choice-title">
        {choice.mandatory
          ? "Выберите обязательный вариант"
          : "Выберите вариант"}
      </h2>
      <p>
        Нужно выбрать от {minimum} до {maximum}.{" "}
        {choice.orderMatters
          ? "Порядок выбора учитывается."
          : "Порядок выбора не учитывается."}
      </p>
      <div className="choice-options">
        {(choice.options ?? []).map((option, index) => {
          const active = selected.includes(option.id);
          const cards = (option.objects ?? [])
            .map((ref) => findVisibleCard(view, ref))
            .filter((card): card is VisibleCard => Boolean(card));
          const names = cards.map((card) => cardName(card.definitionId));
          return (
            <button
              className={active ? "choice-option active" : "choice-option"}
              aria-pressed={active}
              key={option.id}
              onClick={() =>
                setSelected((items) =>
                  active
                    ? items.filter((id) => id !== option.id)
                    : items.length < maximum
                      ? [...items, option.id]
                      : items,
                )
              }
            >
              {cards.map((card) => (
                <CardArtwork
                  definitionId={card.definitionId}
                  key={card.objectRef.zoneObjectId}
                />
              ))}
              {names.length > 0 ? names.join(" · ") : `Вариант ${index + 1}`}
            </button>
          );
        })}
      </div>
      <div className="dialog-actions">
        {selected.length > 0 && (
          <button
            className="secondary"
            disabled={disabled}
            onClick={() => setSelected([])}
          >
            Сбросить выбор
          </button>
        )}
        {!choice.mandatory && (
          <button
            className="secondary"
            disabled={disabled}
            onClick={() => onSubmit([])}
          >
            Не активировать
          </button>
        )}
        <button
          className="primary"
          disabled={disabled || !ready}
          onClick={() => onSubmit(selected)}
        >
          Подтвердить решение
        </button>
      </div>
    </section>
  );
}

function ActionComposer({
  action,
  view,
  onCancel,
  onConfirm,
}: {
  action: PlayerViewAction;
  view: PlayerView;
  onCancel: () => void;
  onConfirm: (command: MatchCommand) => void;
}) {
  const initialDraft: ActionDraft = {
    action,
    modeIds: [],
    targetRefs: [],
    paymentRefs: [],
    shieldPaymentRefs: [],
    influencePayment:
      action.name === "activateAbility" && action.influenceCosts.length === 1
        ? action.influenceCosts[0]
        : undefined,
    placement:
      action.placements.length === 1 ? action.placements[0] : undefined,
  };
  const [draft, dispatch] = useReducer(draftReducer, initialDraft);
  const submitted = useRef(false);
  if (!draft) return null;
  const command = composeDraftCommand(draft);
  useEffect(() => {
    if (command && !submitted.current) {
      submitted.current = true;
      onConfirm(command);
    }
  }, [command, onConfirm]);
  return (
    <section className="action-composer" aria-labelledby="composer-title">
      <p className="step">Многошаговый ввод</p>
      <h2 id="composer-title">
        {action.name === "playCard"
          ? `Разыграть «${cardName(action.definitionId ?? "")}»`
          : `Активировать «${cardName(action.definitionId ?? "")}»`}
      </h2>
      {action.definitionId && (
        <div className="staged-card">
          <CardArtwork definitionId={action.definitionId} large />
          <span>Карта остаётся в области розыгрыша до завершения выбора.</span>
        </div>
      )}
      {action.modeIds.length > 0 && (
        <fieldset>
          <legend>
            Режимы: от {action.minModes} до {action.maxModes}
          </legend>
          {action.modeIds.map((id, index) => (
            <label key={id}>
              <input
                type="checkbox"
                checked={draft.modeIds.includes(id)}
                onChange={() => dispatch({ type: "режим", id })}
              />{" "}
              Режим {index + 1}
            </label>
          ))}
        </fieldset>
      )}
      {action.placements.length > 0 && (
        <fieldset>
          <legend>Линия и клетка</legend>
          <div className="selection-grid">
            {action.placements.map((placement) => (
              <button
                className={
                  draft.placement?.lane === placement.lane &&
                  draft.placement.slot === placement.slot
                    ? "selection active"
                    : "selection"
                }
                key={`${placement.lane}-${placement.slot}`}
                onClick={() => dispatch({ type: "позиция", ...placement })}
              >
                Линия {placement.lane + 1} ·{" "}
                {placement.slot === "unit"
                  ? "юнит"
                  : placement.slot === "support"
                    ? "поддержка"
                    : "локация"}
              </button>
            ))}
          </div>
        </fieldset>
      )}
      {action.targetRefs.length > 0 && (
        <fieldset>
          <legend>
            Цели: от {action.minTargets} до {action.maxTargets}
          </legend>
          <div className="selection-grid">
            {action.targetRefs.map((ref, index) => {
              const card = findVisibleCard(view, ref);
              const active = draft.targetRefs.some((item) =>
                sameRef(item, ref),
              );
              return (
                <button
                  className={active ? "selection active" : "selection"}
                  key={ref.zoneObjectId}
                  onClick={() => dispatch({ type: "цель", ref })}
                >
                  {card && <CardArtwork definitionId={card.definitionId} />}
                  {card ? cardName(card.definitionId) : `Цель ${index + 1}`}
                </button>
              );
            })}
          </div>
        </fieldset>
      )}
      {action.paymentCount > 0 && (
        <fieldset>
          <legend>Оплата картами: {action.paymentCount}</legend>
          <div className="selection-grid">
            {action.paymentRefs.map((ref, index) => {
              const card = findVisibleCard(view, ref);
              const active = draft.paymentRefs.some((item) =>
                sameRef(item, ref),
              );
              return (
                <button
                  className={active ? "selection active" : "selection"}
                  key={ref.zoneObjectId}
                  onClick={() => dispatch({ type: "оплата", ref })}
                >
                  {card && <CardArtwork definitionId={card.definitionId} />}
                  {card ? cardName(card.definitionId) : `Оплата ${index + 1}`}
                </button>
              );
            })}
          </div>
        </fieldset>
      )}
      {action.influenceCosts.length > 0 &&
        action.name === "activateAbility" && (
          <fieldset>
            <legend>Оплата влиянием</legend>
            <div className="selection-grid">
              {action.influenceCosts.map((value) => (
                <button
                  className={
                    draft.influencePayment === value
                      ? "selection active"
                      : "selection"
                  }
                  key={value}
                  onClick={() => dispatch({ type: "влияние", value })}
                >
                  Заплатить влияния: {value}
                </button>
              ))}
            </div>
          </fieldset>
        )}
      {action.shieldTargets.length > 0 && (
        <fieldset>
          <legend>Магический щит</legend>
          <p className="help">
            Отметьте цели, для которых нужно оплатить Магический щит. Итог
            проверит сервер.
          </p>
          <div className="selection-grid">
            {action.shieldTargets.map((ref, index) => {
              const card = findVisibleCard(view, ref);
              const active = draft.shieldPaymentRefs.some((item) =>
                sameRef(item, ref),
              );
              return (
                <button
                  className={active ? "selection active" : "selection"}
                  key={ref.zoneObjectId}
                  onClick={() => dispatch({ type: "щит", ref })}
                >
                  {card && <CardArtwork definitionId={card.definitionId} />}
                  {card
                    ? `Оплатить щит: ${cardName(card.definitionId)}`
                    : `Оплатить щит цели ${index + 1}`}
                </button>
              );
            })}
          </div>
        </fieldset>
      )}
      <div className="command-review">
        <strong>Проверка команды</strong>
        <span>
          Стоимость влияния:{" "}
          {draft.influencePayment ?? action.influenceCosts[0] ?? 0}
        </span>
        <span>Режимов: {draft.modeIds.length}</span>
        <span>Целей: {draft.targetRefs.length}</span>
        <span>Карт в оплате: {draft.paymentRefs.length}</span>
        <span>Оплат Магического щита: {draft.shieldPaymentRefs.length}</span>
      </div>
      <div className="dialog-actions">
        <button className="secondary" onClick={onCancel}>
          Отменить ввод
        </button>
        <span className="help">
          Команда отправится автоматически после последнего обязательного
          выбора.
        </span>
      </div>
    </section>
  );
}

function useCombatPlayback(events: PlayerView["combatEvents"]) {
  const seen = useRef<number | null>(null);
  const [queue, setQueue] = useState<PlayerView["combatEvents"]>([]);
  const [active, setActive] = useState<
    PlayerView["combatEvents"][number] | null
  >(null);
  useEffect(() => {
    const latest = events.at(-1)?.sequence ?? 0;
    if (seen.current === null) {
      seen.current = latest; // snapshot/resync establishes state without replaying old effects
      return;
    }
    const additions = events.filter(
      (event) => event.sequence > (seen.current ?? 0),
    );
    if (additions.length) {
      seen.current = latest;
      setQueue((current) => [...current, ...additions]);
    }
  }, [events]);
  useEffect(() => {
    if (active || queue.length === 0) return;
    setActive(queue[0]);
    setQueue((current) => current.slice(1));
  }, [active, queue]);
  useEffect(() => {
    if (!active) return;
    const timer = window.setTimeout(() => setActive(null), 720);
    return () => window.clearTimeout(timer);
  }, [active]);
  return { active, replaying: Boolean(active) || queue.length > 0 };
}

export function GameplayScreen({
  actionsBlocked,
  connectionLabel,
  desynced,
  error,
  lifecycleNotice,
  onCommand,
  room,
  view,
}: {
  actionsBlocked: boolean;
  connectionLabel: string;
  desynced: boolean;
  error: string;
  lifecycleNotice: string;
  onCommand: (command: MatchCommand) => void;
  room: RoomSnapshot;
  view: PlayerView;
}) {
  const [zoom, setZoom] = useState<Pick<
    VisibleCard,
    "definitionId" | "power" | "durability"
  > | null>(null);
  const [help, setHelp] = useState(false);
  const [draftAction, setDraftAction] = useState<PlayerViewAction | null>(null);
  const [selectedAction, setSelectedAction] = useState<PlayerViewAction | null>(
    null,
  );
  const [draggingAction, setDraggingAction] = useState<PlayerViewAction | null>(
    null,
  );
  const [gallery, setGallery] = useState<{
    title: string;
    cards: VisibleCard[];
  } | null>(null);
  const [confirmSurrender, setConfirmSurrender] = useState(false);
  const playback = useCombatPlayback(view.combatEvents ?? []);
  const own = view.players.find((player) => player.seat === view.ownSeat)!;
  const opponent = view.players.find((player) => player.seat !== view.ownSeat)!;
  const pendingChoice = view.choices[0];
  const actionSuggestions = view.legalActions.filter(
    (action) => action.name === "playCard" || action.name === "activateAbility",
  );
  const interactionBlocked = actionsBlocked || playback.replaying;
  const actionForCard = (card: VisibleCard) =>
    actionSuggestions.find(
      (action) => action.sourceRef && sameRef(action.sourceRef, card.objectRef),
    );
  const stageAction = (action: PlayerViewAction | null) => {
    if (!action || interactionBlocked || pendingChoice || result) return;
    setSelectedAction(null);
    setDraftAction(action);
  };
  const actionFromDrop = (event: DragEvent<HTMLElement>) => {
    const id = event.dataTransfer.getData("application/x-response-card");
    return actionSuggestions.find(
      (candidate) => candidate.sourceRef?.zoneObjectId === id,
    );
  };
  const canPlace = (
    action: PlayerViewAction | null,
    lane: number,
    slot: "unit" | "support",
  ) =>
    Boolean(
      action?.placements.some(
        (placement) => placement.lane === lane && placement.slot === slot,
      ),
    );
  const stagePlacement = (
    action: PlayerViewAction | null | undefined,
    lane: number,
    slot: "unit" | "support",
  ) => {
    if (!action || !canPlace(action, lane, slot)) return;
    stageAction({ ...action, placements: [{ lane, slot }] });
    setDraggingAction(null);
  };
  const passAvailable = view.legalActions.some(
    (action) => action.name === "passAction",
  );
  const cardByCell = useMemo(
    () =>
      new Map(
        view.board.map((card) => [
          `${card.controllerSeat}-${card.lane}-${card.slot}`,
          card,
        ]),
      ),
    [view.board],
  );
  const ownLocation = view.board.find(
    (card) => card.controllerSeat === own.seat && card.slot === "location",
  );
  const opponentLocation = view.board.find(
    (card) => card.controllerSeat === opponent.seat && card.slot === "location",
  );
  const result = view.result;
  const ownRematchReady = room.seats.find(
    (seat) => seat.seatId === view.ownSeat,
  )?.rematchReady;
  const opponentRematchReady = room.seats.find(
    (seat) => seat.seatId !== view.ownSeat,
  )?.rematchReady;

  useEffect(() => {
    setSelectedAction(null);
    setDraftAction(null);
  }, [view.matchId, view.revision]);
  useEffect(() => {
    const cancel = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setSelectedAction(null);
        setDraftAction(null);
      }
    };
    window.addEventListener("keydown", cancel);
    return () => window.removeEventListener("keydown", cancel);
  }, []);

  return (
    <main
      className={`game-shell ${playback.replaying ? "combat-replaying" : ""}`}
    >
      <header className="game-topbar">
        <div>
          <p className="eyebrow">Комната {room.code}</p>
          <h1>Респонс Онлайн</h1>
        </div>
        <div className="game-top-actions">
          <button className="secondary" onClick={() => setHelp(true)}>
            Справка
          </button>
          <div
            className={`connection ${connectionLabel === "Связь установлена" ? "online" : "waiting"}`}
            role="status"
          >
            <span aria-hidden="true" />
            {connectionLabel}
          </div>
        </div>
      </header>
      {desynced && (
        <div className="sync-banner danger" role="alert">
          Состояние синхронизируется. Действия временно заблокированы.
          {lifecycleNotice && (
            <span className="sync-detail">{lifecycleNotice}</span>
          )}
        </div>
      )}
      {error && (
        <div className="sync-banner danger" role="alert">
          {error}
        </div>
      )}
      <section className="match-facts" aria-label="Состояние хода">
        <div>
          <span>Фаза</span>
          <strong>{phaseName(view.phase)}</strong>
        </div>
        <div>
          <span>Ход</span>
          <strong>{view.turnNumber}</strong>
        </div>
        <div>
          <span>Инициатива</span>
          <strong>{seatText(view, view.initiativeSeat)}</strong>
        </div>
        <div>
          <span>Действует</span>
          <strong>{seatText(view, view.actingSeat)}</strong>
        </div>
        <div>
          <span>Ваши действия</span>
          <strong>{own.actionsRemaining}</strong>
        </div>
      </section>

      {pendingChoice && (
        <ChoicePanel
          key={`${pendingChoice.choiceId ?? "redacted"}-${pendingChoice.revision}`}
          choice={pendingChoice}
          disabled={interactionBlocked}
          view={view}
          onSubmit={(optionIds) =>
            onCommand({
              name: "submitChoice",
              payload: { choiceId: pendingChoice.choiceId!, optionIds },
            })
          }
        />
      )}

      <div className="game-layout">
        <section className="battle-area" aria-label="Поле боя">
          <div className="player-ribbon opponent">
            <div>
              <strong>Соперник</strong>
              <span>Влияние: {opponent.influence}</span>
            </div>
            <HiddenHand count={opponent.handCount} />
            <div className="player-location">
              <button
                className="hero-card"
                onClick={() => setZoom({ definitionId: opponent.heroId })}
                aria-label={`Открыть героя соперника ${cardName(opponent.heroId)}`}
              >
                <CardArtwork definitionId={opponent.heroId} />
              </button>
              {opponentLocation ? (
                <CardButton compact card={opponentLocation} onOpen={setZoom} />
              ) : (
                <EmptySlot label="Локация соперника" />
              )}
            </div>
            <div className="player-zones">
              <span
                className="deck-pile card-back"
                aria-label={`Колода соперника: ${opponent.deckCount}`}
              >
                <span>R</span>
                <b>{opponent.deckCount}</b>
              </span>
              <ZoneButton
                cards={opponent.graveyard}
                label="Кладбище"
                onOpen={() =>
                  setGallery({
                    title: "Кладбище соперника",
                    cards: opponent.graveyard,
                  })
                }
              />
              <ZoneButton
                cards={opponent.void ?? []}
                label="Космос"
                onOpen={() =>
                  setGallery({
                    title: "Космос соперника",
                    cards: opponent.void ?? [],
                  })
                }
              />
            </div>
          </div>
          <div className="lanes">
            {Array.from({ length: 5 }, (_, lane) => (
              <article
                className={`${
                  (playback.active?.lane ?? view.combatLane) === lane
                    ? "lane active-combat"
                    : "lane"
                } ${(draggingAction ?? selectedAction)?.placements.length === 0 ? "spell-drop-target" : ""}`}
                key={lane}
                aria-label={`Линия ${lane + 1}`}
                onClick={(event) => {
                  if (
                    selectedAction?.placements.length === 0 &&
                    !(event.target as Element).closest("button")
                  )
                    stageAction(selectedAction);
                }}
                onDragOver={(event) => {
                  if (
                    !interactionBlocked &&
                    draggingAction?.placements.length === 0
                  )
                    event.preventDefault();
                }}
                onDrop={(event) => {
                  const action = actionFromDrop(event);
                  if (!action || action.placements.length > 0) return;
                  event.preventDefault();
                  stageAction(action);
                  setDraggingAction(null);
                }}
              >
                <h2>Линия {lane + 1}</h2>
                <div className="lane-side opponent-side">
                  <OrderCard
                    definitionId={opponent.orders[lane]?.definitionId}
                    hidden={!opponent.orders[lane]?.revealed}
                    onOpen={(definitionId) => setZoom({ definitionId })}
                  />
                  {(["support", "unit"] as const).map((slot) => {
                    const card = cardByCell.get(
                      `${opponent.seat}-${lane}-${slot}`,
                    );
                    return card ? (
                      <CardButton
                        compact
                        card={card}
                        onOpen={setZoom}
                        key={slot}
                      />
                    ) : (
                      <EmptySlot
                        key={slot}
                        label={slot === "unit" ? "Юнит" : "Усиление"}
                      />
                    );
                  })}
                </div>
                <div className="clash-mark" aria-hidden="true">
                  ◆
                </div>
                <div className="lane-side own-side">
                  {(["unit", "support"] as const).map((slot) => {
                    const card = cardByCell.get(`${own.seat}-${lane}-${slot}`);
                    return card ? (
                      <CardButton
                        compact
                        card={card}
                        onOpen={setZoom}
                        key={slot}
                      />
                    ) : (
                      <EmptySlot
                        key={slot}
                        label={slot === "unit" ? "Юнит" : "Усиление"}
                        enabled={canPlace(
                          draggingAction ?? selectedAction,
                          lane,
                          slot,
                        )}
                        onActivate={(event) =>
                          stagePlacement(
                            event ? actionFromDrop(event) : selectedAction,
                            lane,
                            slot,
                          )
                        }
                      />
                    );
                  })}
                  <OrderCard
                    definitionId={own.orders[lane]?.definitionId}
                    hidden={
                      !own.orders[lane]?.revealed &&
                      !own.orders[lane]?.definitionId
                    }
                    onOpen={(definitionId) => setZoom({ definitionId })}
                  />
                </div>
              </article>
            ))}
          </div>
          {draftAction && (
            <div className="action-draft-backdrop">
              <section
                className="action-draft-panel"
                role="dialog"
                aria-modal="true"
                aria-label="Продолжение розыгрыша карты"
              >
                <ActionComposer
                  key={`${draftAction.name}-${draftAction.sourceRef?.zoneObjectId ?? ""}`}
                  action={draftAction}
                  view={view}
                  onCancel={() => setDraftAction(null)}
                  onConfirm={(command) => {
                    onCommand(command);
                    setDraftAction(null);
                  }}
                />
              </section>
            </div>
          )}
          {playback.active && (
            <div className="combat-cue" role="status">
              <strong>
                {playback.active.lane !== undefined
                  ? `Линия ${playback.active.lane + 1}`
                  : "Бой"}
              </strong>
              <span>{playback.active.message}</span>
            </div>
          )}
          <div className="own-dashboard">
            <div className="player-ribbon own">
              <div>
                <strong>Вы · {cardName(own.heroId)}</strong>
                <span>Влияние: {own.influence}</span>
              </div>
              <div className="player-location">
                <button
                  className="hero-card"
                  onClick={() => setZoom({ definitionId: own.heroId })}
                  aria-label={`Открыть вашего героя ${cardName(own.heroId)}`}
                >
                  <CardArtwork definitionId={own.heroId} />
                </button>
                {ownLocation ? (
                  <CardButton compact card={ownLocation} onOpen={setZoom} />
                ) : (
                  <EmptySlot label="Ваша локация" />
                )}
              </div>
              <div className="player-zones">
                <span
                  className="deck-pile card-back"
                  aria-label={`Ваша колода: ${own.deckCount}`}
                >
                  <span>R</span>
                  <b>{own.deckCount}</b>
                </span>
                <ZoneButton
                  cards={own.graveyard}
                  label="Кладбище"
                  onOpen={() =>
                    setGallery({ title: "Ваше кладбище", cards: own.graveyard })
                  }
                />
                <ZoneButton
                  cards={own.void ?? []}
                  label="Космос"
                  onOpen={() =>
                    setGallery({ title: "Ваш космос", cards: own.void ?? [] })
                  }
                />
              </div>
            </div>
            <section className="own-hand" aria-labelledby="hand-title">
              <div className="section-heading">
                <h2 id="hand-title">Ваша рука</h2>
                <span className="count-badge">{own.handCount}</span>
              </div>
              <div className="hand gameplay-hand">
                {(own.hand ?? []).map((card) => (
                  <CardButton
                    card={card}
                    onOpen={setZoom}
                    onPlay={
                      actionForCard(card)
                        ? () => setSelectedAction(actionForCard(card) ?? null)
                        : undefined
                    }
                    selected={Boolean(
                      selectedAction?.sourceRef &&
                      sameRef(selectedAction.sourceRef, card.objectRef),
                    )}
                    draggable={
                      Boolean(actionForCard(card)) && !interactionBlocked
                    }
                    onDragAction={(draggedCard) =>
                      setDraggingAction(actionForCard(draggedCard) ?? null)
                    }
                    onDragActionEnd={() => setDraggingAction(null)}
                    key={card.objectRef.zoneObjectId}
                  />
                ))}
              </div>
            </section>
          </div>
        </section>

        <aside className="game-sidebar">
          <section
            className="sidebar-card actions-panel"
            aria-labelledby="actions-title"
          >
            <h2 id="actions-title">Системные действия</h2>
            <p className="help">
              Карты разыгрываются через руку и поле. Здесь остаются только
              действия матча.
            </p>
            <button
              className="secondary wide"
              disabled={
                interactionBlocked ||
                !passAvailable ||
                Boolean(pendingChoice) ||
                Boolean(result)
              }
              onClick={() => onCommand({ name: "passAction", payload: {} })}
            >
              Спасовать
            </button>
            <button
              className="danger-button wide"
              disabled={interactionBlocked || Boolean(result)}
              onClick={() => setConfirmSurrender(true)}
            >
              Сдаться
            </button>
            {actionSuggestions.length === 0 && !passAvailable && !result && (
              <p className="help">
                Сервер не предлагает действий: ожидаем ход или решение.
              </p>
            )}
          </section>
          <section
            className="sidebar-card queue-panel"
            aria-labelledby="queue-title"
          >
            <div className="section-heading">
              <h2 id="queue-title">Очередь эффектов</h2>
              <span className="count-badge">{view.queue.length}</span>
            </div>
            {view.queue.length ? (
              <ol>
                {view.queue.map((item) => (
                  <li key={`${item.position}-${item.kind}`}>
                    <span>{item.position}</span>
                    {item.kind}
                  </li>
                ))}
              </ol>
            ) : (
              <p className="help">Публичная очередь пуста.</p>
            )}
            <p className="help">
              Бой воспроизводится для обоих игроков с одной фиксированной
              скоростью.
            </p>
          </section>
          <section
            className="sidebar-card journal-panel"
            aria-labelledby="journal-title"
          >
            <h2 id="journal-title">Публичный журнал</h2>
            <ol>
              {view.journal.slice(-30).map((entry) => (
                <li key={entry.sequence}>
                  <span>#{entry.sequence}</span>
                  <strong>
                    {entry.seat ? `${seatText(view, entry.seat)}: ` : ""}
                    {entry.message}
                  </strong>
                </li>
              ))}
            </ol>
          </section>
        </aside>
      </div>

      {result && (
        <section
          className="result-screen"
          role="dialog"
          aria-modal="true"
          aria-labelledby="result-title"
        >
          <p className="step">Единственный итог матча</p>
          <h2 id="result-title">
            {result.winnerSeat
              ? result.winnerSeat === view.ownSeat
                ? "Вы победили"
                : "Победил соперник"
              : "Победитель не определяется"}
          </h2>
          <p>{resultText(result.reason)}</p>
          <p className="result-code">
            Причина результата подтверждена сервером.
          </p>
          <button
            className={ownRematchReady ? "secondary" : "primary"}
            disabled={actionsBlocked}
            onClick={() =>
              onCommand({
                name: "requestRematch",
                payload: { ready: !ownRematchReady },
              })
            }
          >
            {ownRematchReady ? "Отменить предложение" : "Предложить реванш"}
          </button>
          <p className="help">
            {ownRematchReady
              ? opponentRematchReady
                ? "Оба игрока согласились. Начинается новая партия."
                : "Ожидаем согласия соперника. Предложение можно отменить."
              : "Новая партия начнётся только после согласия обоих игроков и получит новый идентификатор."}
          </p>
        </section>
      )}
      {zoom && <CardZoom card={zoom} onClose={() => setZoom(null)} />}
      {gallery && (
        <ZoneGallery
          title={gallery.title}
          cards={gallery.cards}
          onClose={() => setGallery(null)}
          onCard={setZoom}
        />
      )}
      {help && <HelpDialog onClose={() => setHelp(false)} />}
      {confirmSurrender && (
        <SurrenderDialog
          onCancel={() => setConfirmSurrender(false)}
          onConfirm={() => {
            onCommand({ name: "surrender", payload: {} });
            setConfirmSurrender(false);
          }}
        />
      )}
    </main>
  );
}
