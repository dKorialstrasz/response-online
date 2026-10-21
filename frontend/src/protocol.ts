// Schema-aligned parity surface for contracts/ws/envelope.schema.json.
// Contract validation checks the discriminants; TypeScript checks consumers.
export const protocolVersion = "1.2" as const;

export const clientCommandNames = [
  "selectDeck",
  "setOrderArrangement",
  "setReady",
  "submitMulligan",
  "playCard",
  "activateAbility",
  "passAction",
  "submitChoice",
  "surrender",
  "requestRematch",
  "leaveRoom",
  "resync",
] as const;

export const serverEventTypes = [
  "authenticated",
  "roomSnapshot",
  "matchSnapshot",
  "stateUpdate",
  "choiceRequested",
  "commandResult",
  "error",
  "connectionStatus",
  "lifecycleNotice",
  "matchFinished",
  "serverDraining",
  "heartbeat",
] as const;

export type ClientCommandName = (typeof clientCommandNames)[number];
export type ServerEventType = (typeof serverEventTypes)[number];
export type SeatId = "seat-1" | "seat-2";

export interface ObjectRef {
  zoneObjectId: string;
  generation: number;
}

export interface Versions {
  content: string;
  deckManifests: string;
  effectSpec: string;
  rules: string;
  protocol: typeof protocolVersion;
  engine: string;
}

export interface CommandPayloadByName {
  selectDeck: { deckId: string };
  setOrderArrangement: { orderIds: string[] };
  setReady: { ready: boolean };
  submitMulligan: { objectRefs: ObjectRef[] };
  playCard: {
    sourceRef: ObjectRef;
    declaredType: "unit" | "spell" | "enhancement";
    modeIds?: string[];
    targetRefs?: ObjectRef[];
    placement?: {
      lane?: number;
      slot?: "unit" | "support" | "location";
    } | null;
    paymentChoice?: { objectRefs?: ObjectRef[] };
    shieldPaymentRefs?: ObjectRef[];
  };
  activateAbility: {
    abilityId: string;
    sourceRef?: ObjectRef;
    modeIds?: string[];
    targetRefs?: ObjectRef[];
    paymentRefs?: ObjectRef[];
    influencePayment?: number;
    shieldPaymentRefs?: ObjectRef[];
  };
  passAction: Record<string, never>;
  submitChoice: { choiceId: string; optionIds: string[] };
  surrender: Record<string, never>;
  requestRematch: { ready: boolean };
  leaveRoom: Record<string, never>;
  resync: { lastEventSeq?: number };
}

export interface CommandEnvelope<TName extends ClientCommandName> {
  protocolVersion: typeof protocolVersion;
  type: "command";
  commandId: string;
  matchId: string;
  expectedRevision: number;
  name: TName;
  payload: CommandPayloadByName[TName];
}

export type ClientCommand = {
  [TName in ClientCommandName]: CommandEnvelope<TName>;
}[ClientCommandName];

export interface VisibleCard {
  objectRef: ObjectRef;
  definitionId: string;
  ownerSeat?: SeatId;
  controllerSeat?: SeatId;
  lane?: number;
  slot?: "unit" | "support" | "location";
  power?: number;
  durability?: number;
  tapped?: boolean;
}

export interface PlayerViewChoice {
  choiceId?: string;
  waitingSeat: SeatId;
  kind?: string;
  mandatory?: boolean;
  revision: number;
  optionCount?: number;
  minSelections?: number;
  maxSelections?: number;
  orderMatters?: boolean;
  options?: Array<{ id: string; objects?: ObjectRef[] }>;
  private: boolean;
  redacted: boolean;
}

export interface PlayerViewAction {
  name: "playCard" | "activateAbility" | "passAction" | "surrender";
  sourceRef?: ObjectRef;
  definitionId?: string;
  declaredType?: "unit" | "spell" | "enhancement";
  abilityId?: string;
  modeIds: string[];
  minModes: number;
  maxModes: number;
  targetRefs: ObjectRef[];
  minTargets: number;
  maxTargets: number;
  paymentRefs: ObjectRef[];
  paymentCount: number;
  influenceCosts: number[];
  placements: Array<{ lane: number; slot: "unit" | "support" }>;
  shieldTargets: ObjectRef[];
}

export interface PlayerViewQueueItem {
  position: number;
  kind: string;
}

export interface PlayerViewJournalEntry {
  sequence: number;
  seat?: SeatId;
  message: string;
}

export interface PlayerView {
  matchId: string;
  versions: Versions;
  revision: number;
  status: "preparing" | "active" | "finished";
  phase: string;
  combatStep?: string;
  combatLane?: number;
  turnNumber: number;
  ownSeat: SeatId;
  initiativeSeat: SeatId;
  actingSeat: SeatId;
  players: [
    {
      seat: SeatId;
      heroId: string;
      influence: number;
      actionsRemaining: number;
      deckCount: number;
      handCount: number;
      hand?: VisibleCard[];
      graveyard: VisibleCard[];
      void: VisibleCard[];
      orders: Array<{
        position: number;
        revealed: boolean;
        definitionId?: string;
      }>;
    },
    {
      seat: SeatId;
      heroId: string;
      influence: number;
      actionsRemaining: number;
      deckCount: number;
      handCount: number;
      hand?: VisibleCard[];
      graveyard: VisibleCard[];
      void: VisibleCard[];
      orders: Array<{
        position: number;
        revealed: boolean;
        definitionId?: string;
      }>;
    },
  ];
  board: VisibleCard[];
  choices: PlayerViewChoice[];
  legalActions: PlayerViewAction[];
  queue: PlayerViewQueueItem[];
  journal: PlayerViewJournalEntry[];
  combatEvents: Array<{
    sequence: number;
    kind: string;
    seat?: SeatId;
    lane?: number;
    message: string;
  }>;
  result?: {
    reason:
      | "rules_loss"
      | "card_effect"
      | "surrender"
      | "disconnect_forfeit"
      | "aborted_technical";
    winnerSeat?: SeatId;
    loserSeat?: SeatId;
  };
}

export interface RoomSnapshot {
  roomId: string;
  code: string;
  status: "waiting" | "active";
  revision: number;
  ownSeat: SeatId;
  ownSelection: { deckId: string; orderIds: string[] | null };
  seats: [
    {
      seatId: SeatId;
      occupied: boolean;
      ready: boolean;
      rematchReady: boolean;
    },
    {
      seatId: SeatId;
      occupied: boolean;
      ready: boolean;
      rematchReady: boolean;
    },
  ];
  match: {
    matchId: string;
    startedAt: string;
    versions: Versions;
    ownDeckManifestId: string;
    initialRevision: number;
  } | null;
  previousMatches?: Array<{
    matchId: string;
    reason:
      | "rules_loss"
      | "card_effect"
      | "surrender"
      | "disconnect_forfeit"
      | "aborted_technical";
    winnerSeat?: SeatId;
    loserSeat?: SeatId;
  }>;
}

export interface ProtocolError {
  code:
    | "INVALID_PAYLOAD"
    | "UNAUTHENTICATED"
    | "FORBIDDEN"
    | "ROOM_UNAVAILABLE"
    | "ROOM_FULL"
    | "MATCH_FINISHED"
    | "NOT_YOUR_DECISION"
    | "ILLEGAL_ACTION"
    | "INVALID_TARGET"
    | "INSUFFICIENT_COST"
    | "STALE_REVISION"
    | "CHOICE_EXPIRED"
    | "COMMAND_ID_REUSED"
    | "RATE_LIMITED"
    | "VERSION_UNSUPPORTED"
    | "SERVER_UNAVAILABLE";
  retryable: boolean;
  correlationId: string;
  messageKey: string;
  resyncRequired?: boolean;
  view?: PlayerView;
}

export type LifecycleNoticeMessage =
  | "Связь с сервером потеряна. Пытаемся восстановить соединение. На восстановление отведено до 20 секунд"
  | "Связь с соперником потеряна. Ожидаем восстановление до 20 секунд"
  | "Соединение восстановлено. Состояние игры обновлено"
  | "Игрок не восстановил соединение за 20 секунд. Ему засчитано техническое поражение"
  | "Оба игрока не восстановили соединение. Матч завершён технически без победителя"
  | "Нет принятого действия уже 30 секунд. Если сервер не примет действие в течение следующих 60 секунд, матч завершится технически без победителя"
  | "Ожидаем действие соперника. Если бездействие продолжится, матч завершится технически без победителя"
  | "Матч завершён технически из-за бездействия. Победитель не определяется";

interface ServerEventBase<TType extends ServerEventType> {
  protocolVersion: typeof protocolVersion;
  type: TType;
  correlationId?: string;
  commandId?: string;
  matchId?: string;
  revision: number;
  eventSeq: number;
}

export type ServerEvent =
  | (ServerEventBase<"authenticated"> & {
      correlationId: string;
      payload: { seatId: SeatId };
    })
  | (ServerEventBase<"roomSnapshot"> & {
      correlationId: string;
      payload: RoomSnapshot;
    })
  | (ServerEventBase<
      "matchSnapshot" | "stateUpdate" | "choiceRequested" | "matchFinished"
    > & {
      matchId: string;
      payload: PlayerView;
    })
  | (ServerEventBase<"commandResult"> & {
      commandId: string;
      matchId: string;
      accepted: true;
      payload?: RoomSnapshot;
    })
  | (ServerEventBase<"error"> & { accepted: false; error: ProtocolError })
  | (ServerEventBase<"connectionStatus"> & {
      matchId: string;
      payload: {
        seatId: SeatId;
        status: "connected" | "reconnecting" | "disconnected";
        deadlineAt?: string;
      };
    })
  | (ServerEventBase<"lifecycleNotice"> & {
      payload: { message: LifecycleNoticeMessage; view?: PlayerView };
    })
  | (ServerEventBase<"serverDraining"> & { payload: { retryAfterMs: number } })
  | (ServerEventBase<"heartbeat"> & { correlationId: string });
