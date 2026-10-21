import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  DeckPicker,
  EntryScreen,
  MatchScreen,
  OrderEditor,
  moveItem,
  reorderItem,
} from "./App";
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
      protocol: "1.2",
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
      void: [],
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
      void: [],
      orders: [],
    },
  ],
  board: [],
  choices: [],
  legalActions: [],
  queue: [],
  journal: [],
  combatEvents: [],
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
    expect(html.match(/class="deck-hero-art/g)).toHaveLength(6);
    expect(html).toContain("card-cost");
    expect(html).toContain("response-league.ru/api/v1/card-images/");
    expect(html).toContain("Показаны только подтверждённые данные");
  });

  it("меняет порядок без перетаскивания и не выходит за границы", () => {
    expect(moveItem(["а", "б", "в"], 1, -1)).toEqual(["б", "а", "в"]);
    const original = ["а", "б", "в"];
    expect(moveItem(original, 0, -1)).toBe(original);
    expect(moveItem(original, 2, 1)).toBe(original);
  });

  it("переставляет Приказы drag-and-drop и показывает их изображениями без стрелок", () => {
    expect(reorderItem(["а", "б", "в"], 0, 2)).toEqual(["б", "в", "а"]);
    const original = ["а", "б", "в"];
    expect(reorderItem(original, -1, 2)).toBe(original);
    const html = renderToStaticMarkup(
      <OrderEditor
        busy={false}
        orderIds={decks[0].orders.map((order) => order.definitionId)}
        onReorder={vi.fn()}
      />,
    );
    expect(html.match(/draggable="true"/g)).toHaveLength(10);
    expect(html).toContain("--card-faction-a:#7a4ab5");
    expect(html.match(/class="order-card-preview"/g)).toHaveLength(5);
    expect(html).not.toContain("Поднять ");
    expect(html).not.toContain("Опустить ");
  });

  it("использует исправленные цвета Технологий, Смерти и Жизни", () => {
    const palette = new Map([
      ["Технологии", "#7a4ab5"],
      ["Смерть", "#25877f"],
      ["Жизнь", "#c99a5e"],
    ]);
    for (const [faction, color] of palette) {
      const deck = decks.find((candidate) => candidate.factionName === faction);
      expect(deck).toBeDefined();
      const html = renderToStaticMarkup(
        <OrderEditor
          busy={false}
          orderIds={deck!.orders.map((order) => order.definitionId)}
          onReorder={vi.fn()}
        />,
      );
      expect(html).toContain(`--card-faction-a:${color}`);
    }
  });

  it("не выводит руку или решение соперника даже из лишнего поля ответа", () => {
    const html = renderToStaticMarkup(
      <MatchScreen
        connection="подключено"
        error=""
        lifecycleNotice="Соединение восстановлено. Состояние игры обновлено"
        mulliganSubmitted
        room={room}
        view={view}
        onMulligan={vi.fn()}
        onCommand={vi.fn()}
      />,
    );
    expect(html).toContain("Рука соперника скрыта");
    expect(html).toContain("рука и выбор остаются скрыты");
    expect(html.match(/class="hand-art"/g)).toHaveLength(1);
    expect(html).toContain("response-league.ru/api/v1/card-images/");
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
    expect(html).toContain("Выбрать для розыгрыша карту");
    expect(html).not.toContain("Центральная область розыгрыша");
    expect(html).not.toContain(
      "Соединение восстановлено. Состояние игры обновлено",
    );
    expect(html).toContain("Открыть вашего героя");
    expect(html).toContain("Открыть героя соперника");
    expect(html).not.toContain("Доступные действия");
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
    expect(optional).toContain("Не активировать");
  });

  it("не раскрывает закрытое решение соперника", () => {
    const active: PlayerView = {
      ...view,
      status: "active",
      phase: "action",
      choices: [
        {
          waitingSeat: "seat-2",
          revision: 5,
          private: true,
          redacted: true,
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
    expect(html).toContain("Ожидаем решение соперника");
    expect(html).toContain(
      "Закрытые варианты и причина ожидания не раскрываются",
    );
    expect(html).not.toContain("секрет-соперника");
  });

  it("показывает публичную очередь и журнал независимо от анимаций", () => {
    const active: PlayerView = {
      ...view,
      status: "active",
      phase: "combat",
      queue: [{ position: 1, kind: "Изменение силы" }],
      journal: [{ sequence: 6, seat: "seat-1", message: "Карта разыграна" }],
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
    expect(html).toContain("Очередь эффектов");
    expect(html).toContain("Изменение силы");
    expect(html).toContain("Публичный журнал");
    expect(html).toContain("Карта разыграна");
    expect(html).toContain("одной фиксированной скоростью");
    expect(html).not.toContain("Выключены");
  });

  it("показывает протокольную ошибку и блокирует команды при рассинхронизации", () => {
    const active: PlayerView = {
      ...view,
      status: "active",
      phase: "action",
      legalActions: [
        {
          name: "passAction",
          modeIds: [],
          minModes: 0,
          maxModes: 0,
          targetRefs: [],
          minTargets: 0,
          maxTargets: 0,
          paymentRefs: [],
          paymentCount: 0,
          influenceCosts: [],
          placements: [],
          shieldTargets: [],
        },
      ],
    };
    const html = renderToStaticMarkup(
      <MatchScreen
        connection="подключено"
        desynced
        error="Состояние игры изменилось. Действия заблокированы до завершения синхронизации."
        lifecycleNotice=""
        mulliganSubmitted
        room={room}
        view={active}
        onMulligan={vi.fn()}
        onCommand={vi.fn()}
      />,
    );
    expect(html).toContain("Состояние синхронизируется");
    expect(html).toContain("Состояние игры изменилось");
    expect(html).toMatch(/disabled=""[^>]*>Спасовать/);
  });

  it("блокирует игровое поле результатом и отражает состояние реванша", () => {
    const finishedRoom: RoomSnapshot = {
      ...room,
      seats: [
        { ...room.seats[0], rematchReady: true },
        { ...room.seats[1], rematchReady: false },
      ],
    };
    const finished: PlayerView = {
      ...view,
      status: "finished",
      phase: "finished",
      result: {
        reason: "surrender",
        winnerSeat: "seat-1",
        loserSeat: "seat-2",
      },
    };
    const html = renderToStaticMarkup(
      <MatchScreen
        connection="подключено"
        error=""
        mulliganSubmitted
        room={finishedRoom}
        view={finished}
        onMulligan={vi.fn()}
        onCommand={vi.fn()}
      />,
    );
    expect(html).toContain("Вы победили");
    expect(html).toContain("Один из игроков сдался");
    expect(html).toContain("Отменить предложение");
    expect(html).toContain("Ожидаем согласия соперника");
    expect(html).not.toContain("surrender");
  });
});
