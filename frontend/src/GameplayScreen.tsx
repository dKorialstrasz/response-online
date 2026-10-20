import { useEffect, useMemo, useReducer, useRef, useState } from "react";

import type { MatchCommand } from "./api";
import { cardName, confirmedCard, type CatalogCard } from "./catalog";
import {
  composeDraftCommand,
  type ActionDraft,
  draftReady,
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
}: {
  card: VisibleCard;
  compact?: boolean;
  onOpen: (card: VisibleCard) => void;
}) {
  return (
    <button
      className={`game-card ${compact ? "compact" : ""} ${card.tapped ? "tapped" : ""}`}
      onClick={() => onOpen(card)}
      aria-label={`Открыть карту ${cardName(card.definitionId)}`}
    >
      <span className="game-card-mark" aria-hidden="true">
        {cardName(card.definitionId).slice(0, 1)}
      </span>
      <strong>{cardName(card.definitionId)}</strong>
      {(card.power !== undefined || card.durability !== undefined) && (
        <span className="game-card-stats">
          Сила {card.power ?? "—"} · Прочность {card.durability ?? "—"}
        </span>
      )}
    </button>
  );
}

function EmptySlot({ label }: { label: string }) {
  return <span className="empty-slot">{label}</span>;
}

function GraveyardRow({
  cards,
  label,
  onOpen,
}: {
  cards: VisibleCard[];
  label: string;
  onOpen: (card: VisibleCard) => void;
}) {
  return (
    <section className="graveyard-strip" aria-label={label}>
      <strong>{label}</strong>
      <span className="count-badge">{cards.length}</span>
      <div>
        {cards.map((card) => (
          <CardButton
            card={card}
            compact
            key={card.objectRef.zoneObjectId}
            onOpen={onOpen}
          />
        ))}
        {cards.length === 0 && <span className="help">Пусто</span>}
      </div>
    </section>
  );
}

function CardZoom({
  card,
  onClose,
}: {
  card: VisibleCard;
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
          Все карты, варианты и клетки выбираются кнопками. Перетаскивание и
          наведение мыши не требуются. Нажмите Escape, чтобы закрыть окно.
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
          const names = (option.objects ?? [])
            .map((ref) => findVisibleCard(view, ref))
            .filter((card): card is VisibleCard => Boolean(card))
            .map((card) => cardName(card.definitionId));
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
  if (!draft) return null;
  const command = composeDraftCommand(draft);
  return (
    <section className="action-composer" aria-labelledby="composer-title">
      <p className="step">Многошаговый ввод</p>
      <h2 id="composer-title">
        {action.name === "playCard"
          ? `Разыграть «${cardName(action.definitionId ?? "")}»`
          : `Активировать «${cardName(action.definitionId ?? "")}»`}
      </h2>
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
        <button
          className="primary"
          disabled={!draftReady(draft) || !command}
          onClick={() => command && onConfirm(command)}
        >
          Подтвердить и отправить
        </button>
      </div>
    </section>
  );
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
  const [zoom, setZoom] = useState<VisibleCard | null>(null);
  const [help, setHelp] = useState(false);
  const [draftAction, setDraftAction] = useState<PlayerViewAction | null>(null);
  const [confirmSurrender, setConfirmSurrender] = useState(false);
  const [animations, setAnimations] = useState<
    "обычные" | "быстрые" | "выключены"
  >("обычные");
  const own = view.players.find((player) => player.seat === view.ownSeat)!;
  const opponent = view.players.find((player) => player.seat !== view.ownSeat)!;
  const pendingChoice = view.choices[0];
  const actionSuggestions = view.legalActions.filter(
    (action) => action.name === "playCard" || action.name === "activateAbility",
  );
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
  const result = view.result;
  const ownRematchReady = room.seats.find(
    (seat) => seat.seatId === view.ownSeat,
  )?.rematchReady;
  const opponentRematchReady = room.seats.find(
    (seat) => seat.seatId !== view.ownSeat,
  )?.rematchReady;

  return (
    <main className={`game-shell animations-${animations}`}>
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
      {(desynced || lifecycleNotice) && (
        <div
          className={desynced ? "sync-banner danger" : "sync-banner"}
          role="alert"
        >
          {lifecycleNotice ||
            "Состояние синхронизируется. Действия временно заблокированы."}
          {desynced && lifecycleNotice && (
            <span className="sync-detail">
              Состояние синхронизируется. Действия временно заблокированы.
            </span>
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
        <div>
          <span>Связь</span>
          <strong>{connectionLabel}</strong>
        </div>
      </section>

      {pendingChoice && (
        <ChoicePanel
          key={`${pendingChoice.choiceId ?? "redacted"}-${pendingChoice.revision}`}
          choice={pendingChoice}
          disabled={actionsBlocked}
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
              <span>
                Влияние: {opponent.influence} · Рука: {opponent.handCount} ·
                Колода: {opponent.deckCount}
              </span>
            </div>
            <div className="orders" aria-label="Приказы соперника">
              {opponent.orders.map((order) => (
                <span key={order.position}>
                  {order.revealed
                    ? cardName(order.definitionId ?? "")
                    : "Закрыт"}
                </span>
              ))}
            </div>
          </div>
          <GraveyardRow
            cards={opponent.graveyard}
            label="Кладбище соперника"
            onOpen={setZoom}
          />
          <div className="lanes">
            {Array.from({ length: 5 }, (_, lane) => (
              <article
                className={
                  view.combatLane === lane ? "lane active-combat" : "lane"
                }
                key={lane}
                aria-label={`Линия ${lane + 1}`}
              >
                <h2>Линия {lane + 1}</h2>
                <div className="lane-side opponent-side">
                  {(["location", "support", "unit"] as const).map((slot) => {
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
                        label={
                          slot === "unit"
                            ? "Юнит"
                            : slot === "support"
                              ? "Поддержка"
                              : "Локация"
                        }
                      />
                    );
                  })}
                </div>
                <div className="clash-mark" aria-hidden="true">
                  ◆
                </div>
                <div className="lane-side own-side">
                  {(["unit", "support", "location"] as const).map((slot) => {
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
                        label={
                          slot === "unit"
                            ? "Юнит"
                            : slot === "support"
                              ? "Поддержка"
                              : "Локация"
                        }
                      />
                    );
                  })}
                </div>
              </article>
            ))}
          </div>
          <div className="player-ribbon own">
            <div>
              <strong>Вы · {cardName(own.heroId)}</strong>
              <span>
                Влияние: {own.influence} · Колода: {own.deckCount} · Кладбище:{" "}
                {own.graveyard.length}
              </span>
            </div>
            <div className="orders" aria-label="Ваши Приказы">
              {own.orders.map((order) => (
                <span key={order.position}>
                  {cardName(order.definitionId ?? "")}
                </span>
              ))}
            </div>
          </div>
          <GraveyardRow
            cards={own.graveyard}
            label="Ваше кладбище"
            onOpen={setZoom}
          />
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
                  key={card.objectRef.zoneObjectId}
                />
              ))}
            </div>
          </section>
        </section>

        <aside className="game-sidebar">
          <section
            className="sidebar-card actions-panel"
            aria-labelledby="actions-title"
          >
            <h2 id="actions-title">Доступные действия</h2>
            {draftAction ? (
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
            ) : (
              <>
                <div className="action-list">
                  {actionSuggestions.map((action, index) => (
                    <button
                      className="primary"
                      disabled={
                        actionsBlocked ||
                        Boolean(pendingChoice) ||
                        Boolean(result)
                      }
                      key={`${action.name}-${action.sourceRef?.zoneObjectId ?? index}`}
                      onClick={() => setDraftAction(action)}
                    >
                      {action.name === "playCard"
                        ? `Разыграть ${cardName(action.definitionId ?? "")}`
                        : `Активировать ${cardName(action.definitionId ?? "")}`}
                    </button>
                  ))}
                </div>
                <button
                  className="secondary wide"
                  disabled={
                    actionsBlocked ||
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
                  disabled={actionsBlocked || Boolean(result)}
                  onClick={() => setConfirmSurrender(true)}
                >
                  Сдаться
                </button>
                {actionSuggestions.length === 0 &&
                  !passAvailable &&
                  !result && (
                    <p className="help">
                      Сервер не предлагает действий: ожидаем ход или решение.
                    </p>
                  )}
              </>
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
            <label className="animation-setting">
              Анимации
              <select
                value={animations}
                onChange={(event) =>
                  setAnimations(event.target.value as typeof animations)
                }
              >
                <option value="обычные">Обычные</option>
                <option value="быстрые">Быстрые</option>
                <option value="выключены">Выключены</option>
              </select>
            </label>
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
