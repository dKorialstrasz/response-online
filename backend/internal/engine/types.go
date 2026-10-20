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
	MatchID          string              `json:"matchId"`
	Versions         Versions            `json:"versions"`
	Players          [2]PlayerConfig     `json:"players"`
	Seed             uint64              `json:"seed"`
	StartedAt        time.Time           `json:"startedAt"`
	InternalEventCap int                 `json:"internalEventCap,omitempty"`
	Abilities        []AbilityDefinition `json:"-"`
	Cards            []CardDefinition    `json:"-"`
}

// CardDefinition is the immutable, version-pinned gameplay subset consumed by
// the engine. Exact display text remains in the content catalog; the engine
// receives only fields needed to validate and resolve typed effects.
type CardDefinition struct {
	CardID              string     `json:"cardId"`
	Name                string     `json:"name"`
	Types               []CardType `json:"types"`
	Cost                *int       `json:"cost,omitempty"`
	BasePower           *int       `json:"basePower,omitempty"`
	AbilityIDs          []string   `json:"abilityIds,omitempty"`
	NativeKeywords      []Keyword  `json:"nativeKeywords,omitempty"`
	BaseMagicShieldCost int        `json:"baseMagicShieldCost,omitempty"`
}

type CardType string

const (
	CardHero        CardType = "hero"
	CardOrder       CardType = "order"
	CardUnit        CardType = "unit"
	CardSpell       CardType = "spell"
	CardEnhancement CardType = "enhancement"
)

type Keyword string

const (
	KeywordRider        Keyword = "rider"
	KeywordBypass       Keyword = "bypass"
	KeywordPoison       Keyword = "poison"
	KeywordCharge       Keyword = "charge"
	KeywordRangedAttack Keyword = "ranged_attack"
	KeywordResilience   Keyword = "resilience"
)

type ObjectRef struct {
	ZoneObjectID string `json:"zoneObjectId"`
	Generation   uint64 `json:"generation"`
}

type CardObject struct {
	InstanceID              string    `json:"instanceId"`
	DefinitionID            string    `json:"definitionId"`
	OwnerSeat               Seat      `json:"ownerSeat"`
	ControllerSeat          Seat      `json:"controllerSeat"`
	Zone                    Zone      `json:"zone"`
	ZoneObjectID            string    `json:"zoneObjectId"`
	Generation              uint64    `json:"generation"`
	ZoneIndex               int       `json:"zoneIndex"`
	Lane                    int       `json:"lane"`
	Slot                    BoardSlot `json:"slot"`
	BasePower               int       `json:"basePower"`
	Power                   int       `json:"power"`
	BaseDurability          int       `json:"baseDurability"`
	Durability              int       `json:"durability"`
	Tapped                  bool      `json:"tapped"`
	EnteredInCombat         bool      `json:"enteredInCombat"`
	Keywords                []Keyword `json:"keywords,omitempty"`
	BaseMagicShieldCost     int       `json:"baseMagicShieldCost,omitempty"`
	SkipCombat              bool      `json:"skipCombat,omitempty"`
	BaseAbilitiesSuppressed bool      `json:"baseAbilitiesSuppressed,omitempty"`
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
	ID      string           `json:"id"`
	Objects []ObjectRef      `json:"objects,omitempty"`
	Effects []ResolvedEffect `json:"effects,omitempty"`
	Lane    int              `json:"lane,omitempty"`
	LaneSet bool             `json:"laneSet,omitempty"`
	Slot    BoardSlot        `json:"slot,omitempty"`
}

type PendingChoice struct {
	ChoiceID      string              `json:"choiceId"`
	Seat          Seat                `json:"seat"`
	Kind          string              `json:"kind"`
	Mandatory     bool                `json:"mandatory"`
	Visibility    ChoiceVisibility    `json:"visibility"`
	MinSelections int                 `json:"minSelections"`
	MaxSelections int                 `json:"maxSelections"`
	OrderMatters  bool                `json:"orderMatters"`
	QueuePosition QueuePosition       `json:"queuePosition"`
	Options       []ChoiceOption      `json:"options"`
	Revision      uint64              `json:"revision"`
	Continuation  *ChoiceContinuation `json:"continuation,omitempty"`
}

// ChoiceContinuation is authoritative resume state. It is part of the
// deterministic match state and replay hash, but transport projections never
// serialize it. Handler names belong to the closed Base registry.
type ChoiceContinuation struct {
	Handler    string      `json:"handler"`
	Controller Seat        `json:"controller"`
	SourceRef  *ObjectRef  `json:"sourceRef,omitempty"`
	TargetRefs []ObjectRef `json:"targetRefs,omitempty"`
	Step       int         `json:"step,omitempty"`
}

type ChoiceVisibility string

const (
	ChoicePublic    ChoiceVisibility = "public"
	ChoiceOwnerOnly ChoiceVisibility = "owner_only"
)

type QueuePosition string

const (
	QueueBeforeExisting QueuePosition = "before_existing"
	QueueAfterExisting  QueuePosition = "after_existing"
)

type AbilityStatus string

const (
	AbilityReady   AbilityStatus = "ready"
	AbilityBlocked AbilityStatus = "blocked"
)

type TargetController string

const (
	TargetAny      TargetController = "any"
	TargetSelf     TargetController = "self"
	TargetOpponent TargetController = "opponent"
)

type DurationKind string

const (
	DurationPermanent        DurationKind = "permanent"
	DurationUntilEndOfCombat DurationKind = "until_end_of_combat"
	DurationUntilEndOfTurn   DurationKind = "until_end_of_turn"
	DurationWhileSourceValid DurationKind = "while_source_valid"
)

type EffectKind string

const (
	EffectModifyPower        EffectKind = "modify_power"
	EffectMoveZone           EffectKind = "move_zone"
	EffectEnqueue            EffectKind = "enqueue"
	EffectRequestChoice      EffectKind = "request_choice"
	EffectScheduleDelayed    EffectKind = "schedule_delayed"
	EffectInstallReplacement EffectKind = "install_replacement"
)

type PaymentDefinition struct {
	Influence      int    `json:"influence,omitempty"`
	SacrificeCount int    `json:"sacrificeCount,omitempty"`
	SacrificeZones []Zone `json:"sacrificeZones,omitempty"`
}

type TargetDefinition struct {
	Min        int              `json:"min"`
	Max        int              `json:"max"`
	Zones      []Zone           `json:"zones,omitempty"`
	Controller TargetController `json:"controller"`
}

type ModeDefinition struct {
	ID      string              `json:"id"`
	Effects []EffectInstruction `json:"effects"`
}

type AbilityDefinition struct {
	AbilityID              string              `json:"abilityId"`
	Status                 AbilityStatus       `json:"status"`
	BlockingDecisionRef    string              `json:"blockingDecisionRef,omitempty"`
	ConformanceScenarioIDs []string            `json:"conformanceScenarioIds"`
	SourceRequired         bool                `json:"sourceRequired"`
	SourceZones            []Zone              `json:"sourceZones,omitempty"`
	MinModes               int                 `json:"minModes"`
	MaxModes               int                 `json:"maxModes"`
	Modes                  []ModeDefinition    `json:"modes,omitempty"`
	Targets                TargetDefinition    `json:"targets"`
	Payment                PaymentDefinition   `json:"payment"`
	Effects                []EffectInstruction `json:"effects"`
	ConsumesAction         bool                `json:"consumesAction"`
	BuiltinHandler         string              `json:"builtinHandler,omitempty"`
}

type EffectInstruction struct {
	Kind          EffectKind             `json:"kind"`
	TargetIndex   int                    `json:"targetIndex,omitempty"`
	Amount        int                    `json:"amount,omitempty"`
	Layer         int                    `json:"layer,omitempty"`
	Destination   Zone                   `json:"destination,omitempty"`
	Duration      DurationKind           `json:"duration,omitempty"`
	QueuePosition QueuePosition          `json:"queuePosition,omitempty"`
	Effects       []EffectInstruction    `json:"effects,omitempty"`
	Choice        *ChoiceDefinition      `json:"choice,omitempty"`
	Trigger       *DelayedDefinition     `json:"trigger,omitempty"`
	Replacement   *ReplacementDefinition `json:"replacement,omitempty"`
}

type ChoiceDefinition struct {
	Kind          string                   `json:"kind"`
	Mandatory     bool                     `json:"mandatory"`
	Visibility    ChoiceVisibility         `json:"visibility"`
	MinSelections int                      `json:"minSelections"`
	MaxSelections int                      `json:"maxSelections"`
	OrderMatters  bool                     `json:"orderMatters"`
	QueuePosition QueuePosition            `json:"queuePosition"`
	Options       []ChoiceOptionDefinition `json:"options"`
}

type ChoiceOptionDefinition struct {
	ID            string              `json:"id"`
	TargetIndexes []int               `json:"targetIndexes,omitempty"`
	Effects       []EffectInstruction `json:"effects,omitempty"`
}

type DelayedDefinition struct {
	EventKind string              `json:"eventKind"`
	Once      bool                `json:"once"`
	Duration  DurationKind        `json:"duration"`
	Effects   []EffectInstruction `json:"effects"`
}

type ReplacementDefinition struct {
	EffectKind EffectKind          `json:"effectKind"`
	Once       bool                `json:"once"`
	Duration   DurationKind        `json:"duration"`
	Effects    []EffectInstruction `json:"effects"`
}

type ResolvedEffect struct {
	Kind              EffectKind                     `json:"kind"`
	Controller        Seat                           `json:"controller"`
	SourceRef         *ObjectRef                     `json:"sourceRef,omitempty"`
	TargetRef         *ObjectRef                     `json:"targetRef,omitempty"`
	Amount            int                            `json:"amount,omitempty"`
	Layer             int                            `json:"layer,omitempty"`
	Destination       Zone                           `json:"destination,omitempty"`
	Duration          DurationKind                   `json:"duration,omitempty"`
	QueuePosition     QueuePosition                  `json:"queuePosition,omitempty"`
	Effects           []ResolvedEffect               `json:"effects,omitempty"`
	Choice            *ResolvedChoiceDefinition      `json:"choice,omitempty"`
	Trigger           *ResolvedDelayedDefinition     `json:"trigger,omitempty"`
	Replacement       *ResolvedReplacementDefinition `json:"replacement,omitempty"`
	BypassReplacement bool                           `json:"bypassReplacement,omitempty"`
}

type ResolvedChoiceDefinition struct {
	Kind          string           `json:"kind"`
	Mandatory     bool             `json:"mandatory"`
	Visibility    ChoiceVisibility `json:"visibility"`
	MinSelections int              `json:"minSelections"`
	MaxSelections int              `json:"maxSelections"`
	OrderMatters  bool             `json:"orderMatters"`
	QueuePosition QueuePosition    `json:"queuePosition"`
	Options       []ChoiceOption   `json:"options"`
}

type ResolvedDelayedDefinition struct {
	EventKind string           `json:"eventKind"`
	Once      bool             `json:"once"`
	Duration  DurationKind     `json:"duration"`
	Effects   []ResolvedEffect `json:"effects"`
}

type ResolvedReplacementDefinition struct {
	EffectKind EffectKind       `json:"effectKind"`
	Once       bool             `json:"once"`
	Duration   DurationKind     `json:"duration"`
	Effects    []ResolvedEffect `json:"effects"`
}

type EffectQueueEntry struct {
	EntryID         string         `json:"entryId"`
	BatchID         string         `json:"batchId"`
	EnqueueSequence uint64         `json:"enqueueSequence"`
	Effect          ResolvedEffect `json:"effect"`
}

type ModifierState struct {
	ModifierID            string       `json:"modifierId"`
	CreatedSequence       uint64       `json:"createdSequence"`
	SourceRef             *ObjectRef   `json:"sourceRef,omitempty"`
	TargetRef             ObjectRef    `json:"targetRef"`
	Layer                 int          `json:"layer"`
	PowerDelta            int          `json:"powerDelta"`
	BasePowerSet          *int         `json:"basePowerSet,omitempty"`
	Keyword               Keyword      `json:"keyword,omitempty"`
	MagicShieldCost       int          `json:"magicShieldCost,omitempty"`
	DamageReduction       int          `json:"damageReduction,omitempty"`
	SkipCombat            bool         `json:"skipCombat,omitempty"`
	SuppressBaseAbilities bool         `json:"suppressBaseAbilities,omitempty"`
	CannotIncreasePower   bool         `json:"cannotIncreasePower,omitempty"`
	ExtraActionUses       int          `json:"extraActionUses,omitempty"`
	Duration              DurationKind `json:"duration"`
	CreatedTurn           int          `json:"createdTurn"`
}

type DelayedTriggerState struct {
	TriggerID       string           `json:"triggerId"`
	CreatedSequence uint64           `json:"createdSequence"`
	Controller      Seat             `json:"controller"`
	SourceRef       *ObjectRef       `json:"sourceRef,omitempty"`
	EventKind       string           `json:"eventKind"`
	Once            bool             `json:"once"`
	Duration        DurationKind     `json:"duration"`
	CreatedTurn     int              `json:"createdTurn"`
	Effects         []ResolvedEffect `json:"effects"`
}

type ReplacementState struct {
	ReplacementID   string           `json:"replacementId"`
	CreatedSequence uint64           `json:"createdSequence"`
	Controller      Seat             `json:"controller"`
	SourceRef       *ObjectRef       `json:"sourceRef,omitempty"`
	EffectKind      EffectKind       `json:"effectKind"`
	TargetRef       *ObjectRef       `json:"targetRef,omitempty"`
	Once            bool             `json:"once"`
	Duration        DurationKind     `json:"duration"`
	CreatedTurn     int              `json:"createdTurn"`
	Effects         []ResolvedEffect `json:"effects"`
}

type ActionResolutionState struct {
	Seat Seat `json:"seat"`
}

type PendingChoiceProjection struct {
	ChoiceID      string         `json:"choiceId,omitempty"`
	WaitingSeat   Seat           `json:"waitingSeat"`
	Waiting       bool           `json:"waiting"`
	Kind          string         `json:"kind,omitempty"`
	Mandatory     *bool          `json:"mandatory,omitempty"`
	Revision      uint64         `json:"revision"`
	OptionCount   *int           `json:"optionCount,omitempty"`
	MinSelections *int           `json:"minSelections,omitempty"`
	MaxSelections *int           `json:"maxSelections,omitempty"`
	OrderMatters  *bool          `json:"orderMatters,omitempty"`
	Options       []ChoiceOption `json:"options,omitempty"`
	Private       bool           `json:"private"`
	Redacted      bool           `json:"redacted"`
}

type ResultReason string

const (
	ResultRulesLoss         ResultReason = "rules_loss"
	ResultCardEffect        ResultReason = "card_effect"
	ResultSurrender         ResultReason = "surrender"
	ResultDisconnectForfeit ResultReason = "disconnect_forfeit"
	ResultAbortedTechnical  ResultReason = "aborted_technical"
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
	CommandArrangeOrders   CommandKind = "arrange_orders"
	CommandMulligan        CommandKind = "submit_mulligan"
	CommandPlayCard        CommandKind = "play_card"
	CommandCommitAction    CommandKind = "commit_action"
	CommandPass            CommandKind = "pass_action"
	CommandSurrender       CommandKind = "surrender"
	CommandActivateAbility CommandKind = "activate_ability"
	CommandSubmitChoice    CommandKind = "submit_choice"
)

type Command struct {
	ID                 string      `json:"id"`
	Kind               CommandKind `json:"kind"`
	Seat               Seat        `json:"seat"`
	ExpectedRevision   uint64      `json:"expectedRevision"`
	OrderIDs           []string    `json:"orderIds,omitempty"`
	ObjectRefs         []ObjectRef `json:"objectRefs,omitempty"`
	ActionID           string      `json:"actionId,omitempty"`
	AbilityID          string      `json:"abilityId,omitempty"`
	SourceRef          *ObjectRef  `json:"sourceRef,omitempty"`
	ModeIDs            []string    `json:"modeIds,omitempty"`
	TargetRefs         []ObjectRef `json:"targetRefs,omitempty"`
	PaymentRefs        []ObjectRef `json:"paymentRefs,omitempty"`
	ShieldPaymentRefs  []ObjectRef `json:"shieldPaymentRefs,omitempty"`
	ChoiceID           string      `json:"choiceId,omitempty"`
	OptionIDs          []string    `json:"optionIds,omitempty"`
	Value              int         `json:"value,omitempty"`
	DestinationLane    int         `json:"destinationLane,omitempty"`
	DestinationLaneSet bool        `json:"destinationLaneSet,omitempty"`
	DestinationSlot    BoardSlot   `json:"destinationSlot,omitempty"`
	DeclaredType       CardType    `json:"declaredType,omitempty"`
}

// LegalAction is an exact server-authored command template. A projection
// client may only submit combinations present in these allowlists; Handle
// repeats every check transactionally.
type LegalAction struct {
	Name           string           `json:"name"`
	SourceRef      *ObjectRef       `json:"sourceRef,omitempty"`
	DefinitionID   string           `json:"definitionId,omitempty"`
	DeclaredType   CardType         `json:"declaredType,omitempty"`
	AbilityID      string           `json:"abilityId,omitempty"`
	ModeIDs        []string         `json:"modeIds,omitempty"`
	MinModes       int              `json:"minModes"`
	MaxModes       int              `json:"maxModes"`
	TargetRefs     []ObjectRef      `json:"targetRefs,omitempty"`
	MinTargets     int              `json:"minTargets"`
	MaxTargets     int              `json:"maxTargets"`
	PaymentRefs    []ObjectRef      `json:"paymentRefs,omitempty"`
	PaymentCount   int              `json:"paymentCount"`
	InfluenceCosts []int            `json:"influenceCosts,omitempty"`
	Placements     []LegalPlacement `json:"placements,omitempty"`
	ShieldTargets  []ObjectRef      `json:"shieldTargets,omitempty"`
}

type LegalPlacement struct {
	Lane int       `json:"lane"`
	Slot BoardSlot `json:"slot"`
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
	MatchID            string                 `json:"matchId"`
	Versions           Versions               `json:"versions"`
	StartedAt          time.Time              `json:"startedAt"`
	Revision           uint64                 `json:"revision"`
	Status             MatchStatus            `json:"status"`
	Phase              Phase                  `json:"phase"`
	CombatStep         CombatStep             `json:"combatStep"`
	CombatLane         int                    `json:"combatLane"`
	TurnNumber         int                    `json:"turnNumber"`
	InitiativeSeat     Seat                   `json:"initiativeSeat"`
	ActingSeat         Seat                   `json:"actingSeat"`
	ConsecutivePasses  int                    `json:"consecutivePasses"`
	Players            [2]PlayerState         `json:"players"`
	PendingChoices     []PendingChoice        `json:"pendingChoices"`
	EffectQueue        []EffectQueueEntry     `json:"effectQueue,omitempty"`
	Modifiers          []ModifierState        `json:"modifiers,omitempty"`
	DelayedTriggers    []DelayedTriggerState  `json:"delayedTriggers,omitempty"`
	Replacements       []ReplacementState     `json:"replacements,omitempty"`
	ActionResolution   *ActionResolutionState `json:"actionResolution,omitempty"`
	NextEffectSequence uint64                 `json:"nextEffectSequence,omitempty"`
	NextChoiceSequence uint64                 `json:"nextChoiceSequence,omitempty"`
	Result             *MatchResult           `json:"result,omitempty"`
	Random             RandomState            `json:"random"`
	Events             []Event                `json:"events"`
	Commands           []CommandRecord        `json:"commands"`
	DiagnosticTrace    []string               `json:"diagnosticTrace,omitempty"`
}
