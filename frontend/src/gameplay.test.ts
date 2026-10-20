import { describe, expect, it } from "vitest";

import { ApiError, russianError, russianProtocolError } from "./api";
import {
  composeDraftCommand,
  acceptProjection,
  draftReady,
  draftReducer,
  phaseName,
  resultText,
  type ActionDraft,
} from "./gameplay";
import type { ObjectRef, PlayerViewAction, ProtocolError } from "./protocol";

const source: ObjectRef = {
  zoneObjectId: "obj_AAAAAAAAAAAAAAAAAAAAAAAA",
  generation: 1,
};
const target: ObjectRef = {
  zoneObjectId: "obj_BBBBBBBBBBBBBBBBBBBBBBBB",
  generation: 2,
};
const payment: ObjectRef = {
  zoneObjectId: "obj_CCCCCCCCCCCCCCCCCCCCCCCC",
  generation: 3,
};

const action: PlayerViewAction = {
  name: "playCard",
  sourceRef: source,
  definitionId: "baz-002",
  declaredType: "unit",
  abilityId: "baz-002.front.ability",
  modeIds: ["режим-а", "режим-б"],
  minModes: 1,
  maxModes: 1,
  targetRefs: [target],
  minTargets: 1,
  maxTargets: 1,
  paymentRefs: [payment],
  paymentCount: 1,
  influenceCosts: [2],
  placements: [{ lane: 2, slot: "unit" }],
  shieldTargets: [],
};

function initialDraft(): ActionDraft {
  return {
    action,
    modeIds: [],
    targetRefs: [],
    paymentRefs: [],
    shieldPaymentRefs: [],
    placement: action.placements[0],
  };
}

describe("составление игровой команды", () => {
  it("собирает playCard только после всех серверно заданных шагов", () => {
    let draft: ActionDraft | null = initialDraft();
    expect(draftReady(draft)).toBe(false);
    draft = draftReducer(draft, { type: "режим", id: "режим-а" });
    draft = draftReducer(draft, { type: "цель", ref: target });
    draft = draftReducer(draft, { type: "оплата", ref: payment });
    expect(draft).not.toBeNull();
    expect(draftReady(draft!)).toBe(true);
    expect(composeDraftCommand(draft!)).toEqual({
      name: "playCard",
      payload: {
        sourceRef: source,
        declaredType: "unit",
        modeIds: ["режим-а"],
        targetRefs: [target],
        placement: { lane: 2, slot: "unit" },
        paymentChoice: { objectRefs: [payment] },
        shieldPaymentRefs: [],
      },
    });
  });

  it("отменяет локальный ввод без создания команды", () => {
    expect(draftReducer(initialDraft(), { type: "отменить" })).toBeNull();
  });

  it("не позволяет выбрать больше серверного максимума", () => {
    const second = { ...target, zoneObjectId: "obj_DDDDDDDDDDDDDDDDDDDDDDDD" };
    const first = draftReducer(initialDraft(), { type: "цель", ref: target });
    const secondAttempt = draftReducer(first, { type: "цель", ref: second });
    expect(secondAttempt?.targetRefs).toEqual([target]);
  });
});

describe("русские состояния", () => {
  it("не использует английский fallback для фаз и результатов", () => {
    expect(phaseName("action")).toBe("Действия");
    expect(resultText("aborted_technical")).toContain("технически");
    expect(phaseName("неизвестно")).toBe("Игровая фаза");
  });

  it("различает stale revision, потерю сессии и версию", () => {
    const error = (code: ProtocolError["code"]): ProtocolError => ({
      code,
      retryable: false,
      correlationId: "проверка",
      messageKey: "error.test",
    });
    expect(russianProtocolError(error("STALE_REVISION"))).toContain(
      "синхронизации",
    );
    expect(russianProtocolError(error("UNAUTHENTICATED"))).toContain(
      "сессия утрачена",
    );
    expect(russianProtocolError(error("VERSION_UNSUPPORTED"))).toContain(
      "устарела",
    );
  });

  it("даёт отдельные русские состояния входа и сервера", () => {
    const message = (code: string) => russianError(new ApiError(400, { code }));
    expect(message("ROOM_UNAVAILABLE")).toContain("не найдена");
    expect(message("ROOM_FULL")).toContain("два игрока");
    expect(message("VERSION_UNSUPPORTED")).toContain("устарела");
    expect(message("SERVER_UNAVAILABLE")).toContain("недоступен");
    expect(message("UNAUTHENTICATED")).toContain("сессия недоступна");
    expect(message("FORBIDDEN")).toContain("безопасности");
  });
});

describe("порядок проекций", () => {
  it("игнорирует устаревшую revision и более старый eventSeq", () => {
    const minimal = {
      matchId: "матч-1",
      revision: 10,
    } as Parameters<typeof acceptProjection>[1];
    const current = { matchId: "матч-1", revision: 10, eventSeq: 30 };
    expect(
      acceptProjection(current, { ...minimal, revision: 9 }, 40).accepted,
    ).toBe(false);
    expect(acceptProjection(current, minimal, 29).accepted).toBe(false);
    expect(
      acceptProjection(current, { ...minimal, revision: 11 }, 31).accepted,
    ).toBe(true);
    expect(
      acceptProjection(
        current,
        { ...minimal, matchId: "матч-2", revision: 0 },
        1,
      ).accepted,
    ).toBe(true);
  });
});

describe("активация способности", () => {
  it("передаёт только выбранные серверные оплату и щит", () => {
    const ability: PlayerViewAction = {
      ...action,
      name: "activateAbility",
      declaredType: undefined,
      influenceCosts: [1, 2],
      placements: [],
      paymentCount: 0,
      shieldTargets: [target],
    };
    let draft = draftReducer(null, { type: "начать", action: ability });
    draft = draftReducer(draft, { type: "режим", id: "режим-а" });
    draft = draftReducer(draft, { type: "цель", ref: target });
    draft = draftReducer(draft, { type: "влияние", value: 2 });
    draft = draftReducer(draft, { type: "щит", ref: target });
    expect(composeDraftCommand(draft!)).toEqual({
      name: "activateAbility",
      payload: {
        abilityId: "baz-002.front.ability",
        sourceRef: source,
        modeIds: ["режим-а"],
        targetRefs: [target],
        paymentRefs: [],
        influencePayment: 2,
        shieldPaymentRefs: [target],
      },
    });
  });
});
