import type { ObjectRef, RoomSnapshot } from "./protocol";

export type Session = {
  sessionId: string;
  expiresAt: string;
  csrfToken: string;
};

export type ApiProblem = {
  code?: string;
  retryable?: boolean;
  correlationId?: string;
  messageKey?: string;
};

export class ApiError extends Error {
  readonly status: number;
  readonly problem: ApiProblem;

  constructor(status: number, problem: ApiProblem) {
    super(problem.messageKey ?? `HTTP ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
  }
}

function requestId(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

async function jsonRequest<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      ...init.headers,
    },
  });
  if (!response.ok) {
    let problem: ApiProblem = {};
    try {
      problem = (await response.json()) as ApiProblem;
    } catch {
      // The localized UI handles non-JSON failures as a generic server error.
    }
    throw new ApiError(response.status, problem);
  }
  return (await response.json()) as T;
}

export function normalizeRoomCode(value: string): string {
  return value.trim().toUpperCase();
}

export function isValidRoomCode(value: string): boolean {
  return /^[A-Z2-7]{10,32}$/.test(normalizeRoomCode(value));
}

export function invitationUrl(code: string, currentUrl: string): string {
  const current = new URL(currentUrl);
  const invitation = new URL(current.pathname || "/", current.origin);
  invitation.searchParams.set("room", normalizeRoomCode(code));
  return invitation.toString();
}

export function inviteCodeFromUrl(currentUrl: string): string {
  const value = new URL(currentUrl).searchParams.get("room") ?? "";
  const normalized = normalizeRoomCode(value);
  return isValidRoomCode(normalized) ? normalized : "";
}

export async function createSession(signal?: AbortSignal): Promise<Session> {
  return jsonRequest<Session>("/api/v1/sessions", {
    method: "POST",
    body: "{}",
    signal,
  });
}

function secureHeaders(csrfToken: string): HeadersInit {
  return { "X-CSRF-Token": csrfToken };
}

export async function createRoom(csrfToken: string): Promise<RoomSnapshot> {
  const response = await jsonRequest<{ room: RoomSnapshot }>("/api/v1/rooms", {
    method: "POST",
    headers: {
      ...secureHeaders(csrfToken),
      "Idempotency-Key": requestId("create-room"),
    },
    body: "{}",
  });
  return response.room;
}

export async function joinRoom(
  csrfToken: string,
  code: string,
): Promise<RoomSnapshot> {
  const response = await jsonRequest<{ room: RoomSnapshot }>(
    "/api/v1/rooms/join",
    {
      method: "POST",
      headers: {
        ...secureHeaders(csrfToken),
        "Idempotency-Key": requestId("join-room"),
      },
      body: JSON.stringify({ code: normalizeRoomCode(code) }),
    },
  );
  return response.room;
}

export async function getRoom(roomId: string): Promise<RoomSnapshot> {
  const response = await jsonRequest<{ room: RoomSnapshot }>(
    `/api/v1/rooms/${encodeURIComponent(roomId)}`,
  );
  return response.room;
}

export async function setSelection(
  csrfToken: string,
  roomId: string,
  deckId: string,
  orderIds: string[],
): Promise<RoomSnapshot> {
  const response = await jsonRequest<{ room: RoomSnapshot }>(
    `/api/v1/rooms/${encodeURIComponent(roomId)}/selection`,
    {
      method: "POST",
      headers: secureHeaders(csrfToken),
      body: JSON.stringify({ deckId, orderIds }),
    },
  );
  return response.room;
}

export async function setReady(
  csrfToken: string,
  roomId: string,
  ready: boolean,
): Promise<RoomSnapshot> {
  const response = await jsonRequest<{ room: RoomSnapshot }>(
    `/api/v1/rooms/${encodeURIComponent(roomId)}/ready`,
    {
      method: "POST",
      headers: secureHeaders(csrfToken),
      body: JSON.stringify({ ready }),
    },
  );
  return response.room;
}

export async function issueTicket(
  csrfToken: string,
  roomId: string,
): Promise<{ ticket: string; expiresAt: string }> {
  return jsonRequest<{ ticket: string; expiresAt: string }>(
    "/api/v1/ws-tickets",
    {
      method: "POST",
      headers: secureHeaders(csrfToken),
      body: JSON.stringify({ roomId }),
    },
  );
}

export type MatchCommand =
  | { name: "setOrderArrangement"; payload: { orderIds: string[] } }
  | { name: "submitMulligan"; payload: { objectRefs: ObjectRef[] } };

export function russianError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.problem.code) {
      case "ROOM_FULL":
        return "В комнате уже два игрока. Попросите создать новое приглашение.";
      case "ROOM_UNAVAILABLE":
        return "Комната не найдена или срок приглашения истёк.";
      case "UNAUTHENTICATED":
        return "Анонимная сессия недоступна. Обновите страницу и попробуйте снова.";
      case "FORBIDDEN":
        return "Сервер отклонил запрос безопасности. Обновите страницу.";
      case "RATE_LIMITED":
        return "Слишком много запросов. Подождите немного и повторите действие.";
      case "INVALID_PAYLOAD":
        return "Сервер отклонил данные. Проверьте выбор и повторите действие.";
      default:
        return "Сервер временно недоступен. Повторите действие позже.";
    }
  }
  return "Не удалось связаться с сервером. Проверьте соединение и повторите действие.";
}
