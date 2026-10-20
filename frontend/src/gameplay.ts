import type { MatchCommand } from "./api";
import type { ObjectRef, PlayerView, PlayerViewAction } from "./protocol";

export type ProjectionClock = {
  matchId: string;
  revision: number;
  eventSeq: number;
};

export function acceptProjection(
  current: ProjectionClock | null,
  view: PlayerView,
  eventSeq: number,
): { accepted: boolean; clock: ProjectionClock } {
  const next = { matchId: view.matchId, revision: view.revision, eventSeq };
  if (!current || current.matchId !== view.matchId)
    return { accepted: true, clock: next };
  if (
    view.revision < current.revision ||
    (view.revision === current.revision && eventSeq < current.eventSeq)
  )
    return { accepted: false, clock: current };
  return { accepted: true, clock: next };
}

export type ActionDraft = {
  action: PlayerViewAction;
  modeIds: string[];
  targetRefs: ObjectRef[];
  paymentRefs: ObjectRef[];
  shieldPaymentRefs: ObjectRef[];
  influencePayment?: number;
  placement?: PlayerViewAction["placements"][number];
};

export type DraftEvent =
  | { type: "начать"; action: PlayerViewAction }
  | { type: "режим"; id: string }
  | { type: "цель"; ref: ObjectRef }
  | { type: "оплата"; ref: ObjectRef }
  | { type: "щит"; ref: ObjectRef }
  | { type: "влияние"; value: number }
  | { type: "позиция"; lane: number; slot: "unit" | "support" }
  | { type: "отменить" };

const sameRef = (left: ObjectRef, right: ObjectRef) =>
  left.zoneObjectId === right.zoneObjectId &&
  left.generation === right.generation;

function toggleRef(current: ObjectRef[], ref: ObjectRef, maximum: number) {
  if (current.some((item) => sameRef(item, ref)))
    return current.filter((item) => !sameRef(item, ref));
  return current.length < maximum ? [...current, ref] : current;
}

export function draftReducer(
  state: ActionDraft | null,
  event: DraftEvent,
): ActionDraft | null {
  if (event.type === "отменить") return null;
  if (event.type === "начать")
    return {
      action: event.action,
      modeIds: [],
      targetRefs: [],
      paymentRefs: [],
      shieldPaymentRefs: [],
      influencePayment:
        event.action.name === "activateAbility" &&
        event.action.influenceCosts.length === 1
          ? event.action.influenceCosts[0]
          : undefined,
      placement:
        event.action.placements.length === 1
          ? event.action.placements[0]
          : undefined,
    };
  if (!state) return null;
  switch (event.type) {
    case "режим":
      return {
        ...state,
        modeIds: state.modeIds.includes(event.id)
          ? state.modeIds.filter((id) => id !== event.id)
          : state.modeIds.length < state.action.maxModes
            ? [...state.modeIds, event.id]
            : state.modeIds,
      };
    case "цель":
      return {
        ...state,
        targetRefs: toggleRef(
          state.targetRefs,
          event.ref,
          state.action.maxTargets,
        ),
      };
    case "оплата":
      return {
        ...state,
        paymentRefs: toggleRef(
          state.paymentRefs,
          event.ref,
          state.action.paymentCount,
        ),
      };
    case "щит":
      return {
        ...state,
        shieldPaymentRefs: toggleRef(
          state.shieldPaymentRefs,
          event.ref,
          state.action.shieldTargets.length,
        ),
      };
    case "влияние":
      return state.action.influenceCosts.includes(event.value)
        ? { ...state, influencePayment: event.value }
        : state;
    case "позиция":
      return { ...state, placement: { lane: event.lane, slot: event.slot } };
  }
}

export function draftReady(draft: ActionDraft): boolean {
  const { action } = draft;
  return (
    draft.modeIds.length >= action.minModes &&
    draft.modeIds.length <= action.maxModes &&
    draft.targetRefs.length >= action.minTargets &&
    draft.targetRefs.length <= action.maxTargets &&
    draft.paymentRefs.length === action.paymentCount &&
    (action.name !== "activateAbility" ||
      action.influenceCosts.length === 0 ||
      draft.influencePayment !== undefined) &&
    (action.placements.length === 0 || Boolean(draft.placement))
  );
}

export function composeDraftCommand(draft: ActionDraft): MatchCommand | null {
  if (!draftReady(draft)) return null;
  const { action } = draft;
  if (action.name === "playCard" && action.sourceRef && action.declaredType) {
    return {
      name: "playCard",
      payload: {
        sourceRef: action.sourceRef,
        declaredType: action.declaredType,
        modeIds: draft.modeIds,
        targetRefs: draft.targetRefs,
        placement: draft.placement ?? null,
        paymentChoice: { objectRefs: draft.paymentRefs },
        shieldPaymentRefs: draft.shieldPaymentRefs,
      },
    };
  }
  if (action.name === "activateAbility" && action.abilityId) {
    return {
      name: "activateAbility",
      payload: {
        abilityId: action.abilityId,
        sourceRef: action.sourceRef,
        modeIds: draft.modeIds,
        targetRefs: draft.targetRefs,
        paymentRefs: draft.paymentRefs,
        influencePayment: draft.influencePayment,
        shieldPaymentRefs: draft.shieldPaymentRefs,
      },
    };
  }
  return null;
}

export function phaseName(phase: string): string {
  const names: Record<string, string> = {
    setup: "Подготовка",
    order_selection: "Приказы",
    mulligan: "Начальная рука",
    start: "Начало хода",
    draw: "Добор",
    action: "Действия",
    combat: "Бой",
    end: "Завершение хода",
    recovery: "Восстановление",
    finished: "Матч завершён",
  };
  return names[phase] ?? "Игровая фаза";
}

export function resultText(
  reason:
    | "rules_loss"
    | "card_effect"
    | "surrender"
    | "disconnect_forfeit"
    | "aborted_technical",
): string {
  switch (reason) {
    case "rules_loss":
      return "Поражение по правилам партии";
    case "card_effect":
      return "Матч завершён эффектом карты";
    case "surrender":
      return "Один из игроков сдался";
    case "disconnect_forfeit":
      return "Техническое поражение из-за потери связи";
    case "aborted_technical":
      return "Матч завершён технически без игрового результата";
  }
}
