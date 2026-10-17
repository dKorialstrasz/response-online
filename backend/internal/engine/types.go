package engine

import "time"

const (
	PlayerCount             = 2
	LaneCount               = 5
	StartingHandSize        = 7
	ActionsPerPlayer        = 5
	OrdersPerPlayer         = 5
	DefaultInternalEventCap = 10_000
)

type Seat uint8

const (
	SeatA Seat = iota
	SeatB
)

func (seat Seat) Valid() bool { return seat == SeatA || seat == SeatB }

func (seat Seat) Other() Seat {
	if seat == SeatA {
		return SeatB
	}
	return SeatA
}

type MatchStatus string

const (
	StatusPreparing MatchStatus = "preparing"
	StatusActive    MatchStatus = "active"
	StatusFinished  MatchStatus = "finished"
)

type Phase string

const (
	PhaseSetup          Phase = "setup"
	PhaseOrderSelection Phase = "order_selection"
	PhaseMulligan       Phase = "mulligan"
	PhaseStart          Phase = "start"
	PhaseDraw           Phase = "draw"
	PhaseAction         Phase = "action"
	PhaseCombat         Phase = "combat"
	PhaseEnd            Phase = "end"
	PhaseRecovery       Phase = "recovery"
	PhaseFinished       Phase = "finished"
)

type CombatStep string

const (
	CombatNone  CombatStep = ""
	CombatStart CombatStep = "start_combat"
	CombatLane  CombatStep = "lane_step"
	CombatEnd   CombatStep = "end_combat"
)

type Zone string

const (
	ZoneDeck      Zone = "deck"
	ZoneHand      Zone = "hand"
	ZoneBoard     Zone = "board"
	ZoneGraveyard Zone = "graveyard"
	ZoneVoid      Zone = "void"
	ZoneQueue     Zone = "queue"
)

type BoardSlot string

const (
	SlotNone     BoardSlot = ""
	SlotUnit     BoardSlot = "unit"
	SlotSupport  BoardSlot = "support"
	SlotLocation BoardSlot = "location"
)

type Versions struct {
	Content       string `json:"content"`
	DeckManifests string `json:"deckManifests"`
	EffectSpec    string `json:"effectSpec"`
	Rules         string `json:"rules"`
	Protocol      string `json:"protocol"`
	Engine        string `json:"engine"`
}

type PlayerConfig struct {
	Seat          Seat     `json:"seat"`
	HeroID        string   `json:"heroId"`
	DeckCardIDs   []string `json:"deckCardIds"`
	AvailableIDs  []string `json:"availableOrderIds"`
	BaseInfluence int      `json:"baseInfluence"`
}

type Config struct {
	MatchID          string          `json:"matchId"`
	Versions         Versions        `json:"versions"`
	Players          [2]PlayerConfig `json:"players"`
	Seed             uint64          `json:"seed"`
	StartedAt        time.Time       `json:"startedAt"`
	InternalEventCap int             `json:"internalEventCap,omitempty"`
}

type ObjectRef struct {
	ZoneObjectID string `json:"zoneObjectId"`
	Generation   uint64 `json:"generation"`
}

type CardObject struct {
	InstanceID      string    `json:"instanceId"`
	DefinitionID    string    `json:"definitionId"`
	OwnerSeat       Seat      `json:"ownerSeat"`
	ControllerSeat  Seat      `json:"controllerSeat"`
	Zone            Zone      `json:"zone"`
	ZoneObjectID    string    `json:"zoneObjectId"`
	Generation      uint64    `json:"generation"`
	ZoneIndex       int       `json:"zoneIndex"`
	Lane            int       `json:"lane"`
	Slot            BoardSlot `json:"slot"`
	BasePower       int       `json:"basePower"`
	Power           int       `json:"power"`
	BaseDurability  int       `json:"baseDurability"`
	Durability      int       `json:"durability"`
	Tapped          bool      `json:"tapped"`
	EnteredInCombat bool      `json:"enteredInCombat"`
}

func (object CardObject) Ref() ObjectRef {
	return ObjectRef{ZoneObjectID: object.ZoneObjectID, Generation: object.Generation}
}

type OrderState struct {
	DefinitionID string `json:"definitionId"`
	Position     int    `json:"position"`
	Revealed     bool   `json:"revealed"`
}

type PlayerState struct {
	Seat              Seat         `json:"seat"`
	HeroID            string       `json:"heroId"`
	BaseInfluence     int          `json:"baseInfluence"`
	Influence         int          `json:"influence"`
	Objects           []CardObject `json:"objects"`
	AvailableOrderIDs []string     `json:"availableOrderIds"`
	Orders            []OrderState `json:"orders"`
	OrdersLocked      bool         `json:"ordersLocked"`
	MulliganSubmitted bool         `json:"mulliganSubmitted"`
	ActionsRemaining  int          `json:"actionsRemaining"`
}

type ChoiceOption struct {
	ID      string      `json:"id"`
	Objects []ObjectRef `json:"objects,omitempty"`
}

type PendingChoice struct {
	ChoiceID string         `json:"choiceId"`
	Seat     Seat           `json:"seat"`
	Kind     string         `json:"kind"`
	Options  []ChoiceOption `json:"options"`
	Revision uint64         `json:"revision"`
}

type ResultReason string

const (
	ResultRulesLoss        ResultReason = "rules_loss"
	ResultCardEffect       ResultReason = "card_effect"
	ResultSurrender        ResultReason = "surrender"
	ResultAbortedTechnical ResultReason = "aborted_technical"
)

type MatchResult struct {
	Reason     ResultReason `json:"reason"`
	WinnerSeat *Seat        `json:"winnerSeat,omitempty"`
	LoserSeat  *Seat        `json:"loserSeat,omitempty"`
	Detail     string       `json:"detail"`
}

type Event struct {
	Sequence uint64 `json:"sequence"`
	Kind     string `json:"kind"`
	Seat     *Seat  `json:"seat,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type CommandKind string

const (
	CommandArrangeOrders CommandKind = "arrange_orders"
	CommandMulligan      CommandKind = "submit_mulligan"
	CommandCommitAction  CommandKind = "commit_action"
	CommandPass          CommandKind = "pass_action"
	CommandSurrender     CommandKind = "surrender"
)

type Command struct {
	ID               string      `json:"id"`
	Kind             CommandKind `json:"kind"`
	Seat             Seat        `json:"seat"`
	ExpectedRevision uint64      `json:"expectedRevision"`
	OrderIDs         []string    `json:"orderIds,omitempty"`
	ObjectRefs       []ObjectRef `json:"objectRefs,omitempty"`
	ActionID         string      `json:"actionId,omitempty"`
}

type CommandRecord struct {
	Sequence uint64  `json:"sequence"`
	Command  Command `json:"command"`
}

type RandomState struct {
	Seed   uint64 `json:"seed"`
	State  uint64 `json:"state"`
	Cursor uint64 `json:"cursor"`
}

type State struct {
	MatchID           string          `json:"matchId"`
	Versions          Versions        `json:"versions"`
	StartedAt         time.Time       `json:"startedAt"`
	Revision          uint64          `json:"revision"`
	Status            MatchStatus     `json:"status"`
	Phase             Phase           `json:"phase"`
	CombatStep        CombatStep      `json:"combatStep"`
	CombatLane        int             `json:"combatLane"`
	TurnNumber        int             `json:"turnNumber"`
	InitiativeSeat    Seat            `json:"initiativeSeat"`
	ActingSeat        Seat            `json:"actingSeat"`
	ConsecutivePasses int             `json:"consecutivePasses"`
	Players           [2]PlayerState  `json:"players"`
	PendingChoices    []PendingChoice `json:"pendingChoices"`
	Result            *MatchResult    `json:"result,omitempty"`
	Random            RandomState     `json:"random"`
	Events            []Event         `json:"events"`
	Commands          []CommandRecord `json:"commands"`
	DiagnosticTrace   []string        `json:"diagnosticTrace,omitempty"`
}
