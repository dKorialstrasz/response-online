import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createRoom,
  invitationUrl,
  inviteCodeFromUrl,
  isValidRoomCode,
  joinRoom,
  normalizeRoomCode,
  setReady,
  setSelection,
} from "./api";
import type { RoomSnapshot } from "./protocol";

const snapshot = {
  room: {
    roomId: "room-1",
    code: "ABCDEFGH23456",
    status: "waiting",
    revision: 1,
    ownSeat: "seat-1",
    ownSelection: { deckId: "", orderIds: [] },
    seats: [],
    match: null,
  } as unknown as RoomSnapshot,
};

afterEach(() => vi.unstubAllGlobals());

function ok(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

describe("код комнаты и приглашение", () => {
  it("нормализует регистр и пробелы и отвергает чужой алфавит", () => {
    expect(normalizeRoomCode("  abcdefgh23456  ")).toBe("ABCDEFGH23456");
    expect(isValidRoomCode(" abcdefgh23456 ")).toBe(true);
    expect(isValidRoomCode("ABC-DEF-2345")).toBe(false);
    expect(isValidRoomCode("ABCDEFGH0189")).toBe(false);
  });

  it("оставляет в ссылке только код комнаты", () => {
    const url = invitationUrl(
      "abcdefgh23456",
      "https://game.example/play?secret=hidden&token=csrf#private",
    );
    expect(url).toBe("https://game.example/play?room=ABCDEFGH23456");
    expect(url).not.toMatch(/secret|token|csrf|private/);
    expect(inviteCodeFromUrl(url)).toBe("ABCDEFGH23456");
  });
});

describe("интеграция HTTP-контракта", () => {
  it("создаёт и присоединяет комнату с cookie, CSRF и idempotency", async () => {
    const fetchMock = vi.fn().mockImplementation(() => ok(snapshot));
    vi.stubGlobal("fetch", fetchMock);
    await createRoom("csrf-value");
    await joinRoom("csrf-value", " abcdefgh23456 ");
    const create = fetchMock.mock.calls[0] as [string, RequestInit];
    const join = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(create[0]).toBe("/api/v1/rooms");
    expect(create[1].credentials).toBe("same-origin");
    expect(create[1].headers).toMatchObject({ "X-CSRF-Token": "csrf-value" });
    expect(
      (create[1].headers as Record<string, string>)["Idempotency-Key"],
    ).toMatch(/^create-room-/);
    expect(join[0]).toBe("/api/v1/rooms/join");
    expect(JSON.parse(String(join[1].body))).toEqual({ code: "ABCDEFGH23456" });
  });

  it("отправляет только deckId и допустимый порядок, а изменение следует за ready", async () => {
    const fetchMock = vi.fn().mockImplementation(() => ok(snapshot));
    vi.stubGlobal("fetch", fetchMock);
    await setReady("csrf-value", "room-1", true);
    await setSelection("csrf-value", "room-1", "technology-dari", [
      "baz-018",
      "baz-019",
      "baz-020",
      "baz-021",
      "baz-022",
    ]);
    const selection = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(selection[0]).toBe("/api/v1/rooms/room-1/selection");
    expect(JSON.parse(String(selection[1].body))).toEqual({
      deckId: "technology-dari",
      orderIds: ["baz-018", "baz-019", "baz-020", "baz-021", "baz-022"],
    });
    expect(String(selection[1].body)).not.toContain("cards");
  });
});
