import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { DeckPicker, EntryScreen, MatchScreen, moveItem } from "./App";
import { decks } from "./catalog";
import type { PlayerView, RoomSnapshot } from "./protocol";

const room: RoomSnapshot = {
  roomId: "room-public-id",
  code: "ABCDEFGH23456",
  status: "active",
  revision: 4,
  ownSeat: "seat-1",
  ownSelection: {
    deckId: "technology-dari",
    orderIds: ["baz-018", "baz-019", "baz-020", "baz-021", "baz-022"],
  },
  seats: [
    { seatId: "seat-1", occupied: true, ready: true, rematchReady: false },
    { seatId: "seat-2", occupied: true, ready: true, rematchReady: false },
  ],
  match: {
    matchId: "match-public-id",
    startedAt: "2026-10-19T00:00:00Z",
    versions: {
      content: "content",
      deckManifests: "decks",
      effectSpec: "effects",
      rules: "rules",
      protocol: "1.1",
      engine: "engine",
    },
    ownDeckManifestId: "own-manifest",
    initialRevision: 0,
  },
};

const view: PlayerView = {
  matchId: "match-public-id",
  versions: room.match!.versions,
  revision: 3,
  status: "preparing",
  phase: "mulligan",
  turnNumber: 0,
  ownSeat: "seat-1",
  initiativeSeat: "seat-2",
  actingSeat: "seat-2",
  players: [
    {
      seat: "seat-1",
      heroId: "baz-001",
      influence: 5,
      actionsRemaining: 0,
      deckCount: 23,
      handCount: 7,
      hand: [
        {
          objectRef: {
            zoneObjectId: "obj_AAAAAAAAAAAAAAAAAAAAAAAA",
            generation: 1,
          },
          definitionId: "baz-002",
        },
      ],
      graveyard: [],
      orders: [],
    },
    {
      seat: "seat-2",
      heroId: "baz-023",
      influence: 5,
      actionsRemaining: 0,
      deckCount: 23,
      handCount: 7,
      hand: [
        {
          objectRef: {
            zoneObjectId: "obj_BBBBBBBBBBBBBBBBBBBBBBBB",
            generation: 1,
          },
          definitionId: "секрет-соперника",
        },
      ],
      graveyard: [],
      orders: [],
    },
  ],
  board: [],
  choices: [],
  legalActions: [],
};

describe("интерфейс комнаты", () => {
  it("показывает русский вход и ручной ввод кода", () => {
    const html = renderToStaticMarkup(
      <EntryScreen
        busy={false}
        code=""
        error=""
        onCodeChange={vi.fn()}
        onCreate={vi.fn()}
        onJoin={vi.fn()}
      />,
    );
    expect(html).toContain("Создать приватную комнату");
    expect(html).toContain("Код комнаты");
    expect(html).toContain("Буфер обмена не требуется");
  });

  it("рендерит ровно шесть утверждённых колод", () => {
    const html = renderToStaticMarkup(
      <DeckPicker
        currentDeckId=""
        disabled={false}
        inspectedDeck={decks[0]}
        onChoose={vi.fn()}
        onInspect={vi.fn()}
      />,
    );
    expect(decks).toHaveLength(6);
    for (const deck of decks) expect(html).toContain(deck.displayName);
    expect(html.match(/class="deck-tile/g)).toHaveLength(6);
    expect(html).toContain("Показаны только подтверждённые данные");
  });

  it("меняет порядок без перетаскивания и не выходит за границы", () => {
    expect(moveItem(["а", "б", "в"], 1, -1)).toEqual(["б", "а", "в"]);
    const original = ["а", "б", "в"];
    expect(moveItem(original, 0, -1)).toBe(original);
    expect(moveItem(original, 2, 1)).toBe(original);
  });

  it("не выводит руку или решение соперника даже из лишнего поля ответа", () => {
    const html = renderToStaticMarkup(
      <MatchScreen
        connection="подключено"
        error=""
        mulliganSubmitted
        room={room}
        view={view}
        onMulligan={vi.fn()}
        onCommand={vi.fn()}
      />,
    );
    expect(html).toContain("Рука соперника скрыта");
    expect(html).toContain("рука и выбор остаются скрыты");
    expect(html).not.toContain("секрет-соперника");
    expect(html).not.toContain("obj_BBBBB");
  });

  it("объявляет ожидание и восстановление соединения", () => {
    const waiting = renderToStaticMarkup(
      <MatchScreen
        connection="восстановление"
        error="Соединение потеряно"
        mulliganSubmitted={false}
        room={room}
        view={null}
        onMulligan={vi.fn()}
        onCommand={vi.fn()}
      />,
    );
    expect(waiting).toContain("Восстанавливаем соединение");
    expect(waiting).toContain("Соединение потеряно");
  });

  it("рендерит playCard только из серверного legalActions", () => {
    const active: PlayerView = {
      ...view,
      status: "active",
      phase: "action",
      actingSeat: "seat-1",
      legalActions: [
        {
          name: "playCard",
          sourceRef: view.players[0].hand![0].objectRef,
          definitionId: "baz-121",
          declaredType: "spell",
          modeIds: [],
          minModes: 0,
          maxModes: 0,
          targetRefs: [],
          minTargets: 0,
          maxTargets: 0,
          paymentRefs: [],
          paymentCount: 0,
          influenceCosts: [1],
          placements: [],
          shieldTargets: [],
        },
      ],
    };
    const html = renderToStaticMarkup(
      <MatchScreen
        connection="подключено"
        error=""
        mulliganSubmitted
        room={room}
        view={active}
        onMulligan={vi.fn()}
        onCommand={vi.fn()}
      />,
    );
    expect(html).toContain("Разыграть Разумный выбор");
  });

  it("показывает decline только для optional PendingChoice", () => {
    const active: PlayerView = {
      ...view,
      status: "active",
      phase: "action",
      actingSeat: "seat-1",
      choices: [
        {
          choiceId: "choice-1",
          waitingSeat: "seat-1",
          kind: "put_hand_card_on_top",
          mandatory: true,
          revision: 4,
          optionCount: 1,
          minSelections: 1,
          maxSelections: 1,
          orderMatters: false,
          options: [
            { id: "option-1", objects: [view.players[0].hand![0].objectRef] },
          ],
          private: true,
          redacted: false,
        },
      ],
    };
    const mandatory = renderToStaticMarkup(
      <MatchScreen
        connection="подключено"
        error=""
        mulliganSubmitted
        room={room}
        view={active}
        onMulligan={vi.fn()}
        onCommand={vi.fn()}
      />,
    );
    expect(mandatory).toContain("Выберите обязательный вариант");
    expect(mandatory).not.toContain("Отказаться");

    active.choices[0] = {
      ...active.choices[0],
      mandatory: false,
      minSelections: 0,
    };
    const optional = renderToStaticMarkup(
      <MatchScreen
        connection="подключено"
        error=""
        mulliganSubmitted
        room={room}
        view={active}
        onMulligan={vi.fn()}
        onCommand={vi.fn()}
      />,
    );
    expect(optional).toContain("Отказаться");
  });
});
