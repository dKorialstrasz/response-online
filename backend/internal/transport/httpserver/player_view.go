package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

// PlayerView is a transport-owned allowlist. It intentionally cannot contain
// engine.Random, commands, audit events, effect bodies, diagnostic traces, or
// CardObject.InstanceID.
type PlayerView struct {
	MatchID        string                   `json:"matchId"`
	Versions       engine.Versions          `json:"versions"`
	Revision       uint64                   `json:"revision"`
	Status         engine.MatchStatus       `json:"status"`
	Phase          engine.Phase             `json:"phase"`
	CombatStep     engine.CombatStep        `json:"combatStep,omitempty"`
	CombatLane     int                      `json:"combatLane"`
	TurnNumber     int                      `json:"turnNumber"`
	OwnSeat        string                   `json:"ownSeat"`
	InitiativeSeat string                   `json:"initiativeSeat"`
	ActingSeat     string                   `json:"actingSeat"`
	Players        [2]PlayerViewPlayer      `json:"players"`
	Board          []PlayerViewCard         `json:"board"`
	Choices        []PlayerViewChoice       `json:"choices"`
	LegalActions   []PlayerViewAction       `json:"legalActions"`
	Queue          []PlayerViewQueueItem    `json:"queue"`
	Journal        []PlayerViewJournalEntry `json:"journal"`
	Result         *PlayerViewResult        `json:"result,omitempty"`
}

type PlayerViewPlayer struct {
	Seat             string            `json:"seat"`
	HeroID           string            `json:"heroId"`
	Influence        int               `json:"influence"`
	ActionsRemaining int               `json:"actionsRemaining"`
	DeckCount        int               `json:"deckCount"`
	HandCount        int               `json:"handCount"`
	Hand             []PlayerViewCard  `json:"hand,omitempty"`
	Graveyard        []PlayerViewCard  `json:"graveyard"`
	Orders           []PlayerViewOrder `json:"orders"`
}

type PlayerViewCard struct {
	ObjectRef      objectRefPayload `json:"objectRef"`
	DefinitionID   string           `json:"definitionId"`
	OwnerSeat      string           `json:"ownerSeat,omitempty"`
	ControllerSeat string           `json:"controllerSeat,omitempty"`
	Lane           int              `json:"lane,omitempty"`
	Slot           engine.BoardSlot `json:"slot,omitempty"`
	Power          int              `json:"power,omitempty"`
	Durability     int              `json:"durability,omitempty"`
	Tapped         bool             `json:"tapped,omitempty"`
}

type PlayerViewOrder struct {
	Position     int    `json:"position"`
	Revealed     bool   `json:"revealed"`
	DefinitionID string `json:"definitionId,omitempty"`
}

type PlayerViewChoice struct {
	ChoiceID      string                   `json:"choiceId,omitempty"`
	WaitingSeat   string                   `json:"waitingSeat"`
	Kind          string                   `json:"kind,omitempty"`
	Mandatory     *bool                    `json:"mandatory,omitempty"`
	Revision      uint64                   `json:"revision"`
	OptionCount   *int                     `json:"optionCount,omitempty"`
	MinSelections *int                     `json:"minSelections,omitempty"`
	MaxSelections *int                     `json:"maxSelections,omitempty"`
	OrderMatters  *bool                    `json:"orderMatters,omitempty"`
	Options       []PlayerViewChoiceOption `json:"options,omitempty"`
	Private       bool                     `json:"private"`
	Redacted      bool                     `json:"redacted"`
}

type PlayerViewAction struct {
	Name           string                `json:"name"`
	SourceRef      *objectRefPayload     `json:"sourceRef,omitempty"`
	DefinitionID   string                `json:"definitionId,omitempty"`
	DeclaredType   engine.CardType       `json:"declaredType,omitempty"`
	AbilityID      string                `json:"abilityId,omitempty"`
	ModeIDs        []string              `json:"modeIds"`
	MinModes       int                   `json:"minModes"`
	MaxModes       int                   `json:"maxModes"`
	TargetRefs     []objectRefPayload    `json:"targetRefs"`
	MinTargets     int                   `json:"minTargets"`
	MaxTargets     int                   `json:"maxTargets"`
	PaymentRefs    []objectRefPayload    `json:"paymentRefs"`
	PaymentCount   int                   `json:"paymentCount"`
	InfluenceCosts []int                 `json:"influenceCosts"`
	Placements     []PlayerViewPlacement `json:"placements"`
	ShieldTargets  []objectRefPayload    `json:"shieldTargets"`
}

type PlayerViewPlacement struct {
	Lane int              `json:"lane"`
	Slot engine.BoardSlot `json:"slot"`
}

type PlayerViewChoiceOption struct {
	ID      string             `json:"id"`
	Objects []objectRefPayload `json:"objects,omitempty"`
}

type PlayerViewQueueItem struct {
	Position int    `json:"position"`
	Kind     string `json:"kind"`
}

type PlayerViewJournalEntry struct {
	Sequence uint64 `json:"sequence"`
	Seat     string `json:"seat,omitempty"`
	Message  string `json:"message"`
}

type PlayerViewResult struct {
	Reason     engine.ResultReason `json:"reason"`
	WinnerSeat string              `json:"winnerSeat,omitempty"`
	LoserSeat  string              `json:"loserSeat,omitempty"`
}

type referenceCodec struct {
	key []byte
}

func newReferenceCodec(key []byte) referenceCodec {
	return referenceCodec{key: append([]byte(nil), key...)}
}

func (codec referenceCodec) encode(ref engine.ObjectRef) objectRefPayload {
	mac := hmac.New(sha256.New, codec.key)
	_, _ = mac.Write([]byte(ref.ZoneObjectID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strconv.FormatUint(ref.Generation, 10)))
	return objectRefPayload{
		ZoneObjectID: "obj_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18]),
		Generation:   ref.Generation,
	}
}

func (codec referenceCodec) decode(state engine.State, value objectRefPayload) (engine.ObjectRef, error) {
	if value.ZoneObjectID == "" || len(value.ZoneObjectID) > 128 {
		return engine.ObjectRef{}, fmt.Errorf("invalid object reference")
	}
	for _, player := range state.Players {
		for _, object := range player.Objects {
			ref := object.Ref()
			encoded := codec.encode(ref)
			if hmac.Equal([]byte(encoded.ZoneObjectID), []byte(value.ZoneObjectID)) &&
				encoded.Generation == value.Generation {
				return ref, nil
			}
		}
	}
	return engine.ObjectRef{}, fmt.Errorf("unknown or stale object reference")
}

func projectPlayerView(state engine.State, viewer engine.Seat, codec referenceCodec, legal ...[]engine.LegalAction) PlayerView {
	view := PlayerView{
		MatchID: state.MatchID, Versions: state.Versions, Revision: state.Revision,
		Status: state.Status, Phase: state.Phase, CombatStep: state.CombatStep,
		CombatLane: state.CombatLane, TurnNumber: state.TurnNumber,
		OwnSeat: seatName(viewer), InitiativeSeat: seatName(state.InitiativeSeat),
		ActingSeat: seatName(state.ActingSeat), Board: make([]PlayerViewCard, 0),
		Choices: make([]PlayerViewChoice, 0), LegalActions: make([]PlayerViewAction, 0),
		Queue: make([]PlayerViewQueueItem, 0), Journal: make([]PlayerViewJournalEntry, 0),
	}
	for playerIndex, player := range state.Players {
		projected := PlayerViewPlayer{
			Seat: seatName(player.Seat), HeroID: player.HeroID, Influence: player.Influence,
			ActionsRemaining: player.ActionsRemaining, Hand: make([]PlayerViewCard, 0),
			Graveyard: make([]PlayerViewCard, 0), Orders: make([]PlayerViewOrder, 0, len(player.Orders)),
		}
		for _, object := range player.Objects {
			switch object.Zone {
			case engine.ZoneDeck:
				projected.DeckCount++
			case engine.ZoneHand:
				projected.HandCount++
				if player.Seat == viewer {
					projected.Hand = append(projected.Hand, projectKnownCard(object, codec, false))
				}
			case engine.ZoneBoard:
				view.Board = append(view.Board, projectKnownCard(object, codec, true))
			case engine.ZoneGraveyard:
				projected.Graveyard = append(projected.Graveyard, projectKnownCard(object, codec, true))
			}
		}
		for _, order := range player.Orders {
			item := PlayerViewOrder{Position: order.Position, Revealed: order.Revealed}
			if player.Seat == viewer || order.Revealed {
				item.DefinitionID = order.DefinitionID
			}
			projected.Orders = append(projected.Orders, item)
		}
		view.Players[playerIndex] = projected
	}
	for _, choice := range state.PendingChoiceProjections(viewer) {
		projected := PlayerViewChoice{
			ChoiceID: choice.ChoiceID, WaitingSeat: seatName(choice.WaitingSeat), Kind: choice.Kind,
			Mandatory: choice.Mandatory, Revision: choice.Revision, OptionCount: choice.OptionCount,
			MinSelections: choice.MinSelections, MaxSelections: choice.MaxSelections, OrderMatters: choice.OrderMatters,
			Private: choice.Private, Redacted: choice.Redacted,
		}
		for _, option := range choice.Options {
			item := PlayerViewChoiceOption{ID: option.ID, Objects: make([]objectRefPayload, 0, len(option.Objects))}
			for _, ref := range option.Objects {
				item.Objects = append(item.Objects, codec.encode(ref))
			}
			projected.Options = append(projected.Options, item)
		}
		view.Choices = append(view.Choices, projected)
	}
	if len(legal) > 0 {
		for _, action := range legal[0] {
			projected := PlayerViewAction{
				Name: action.Name, DefinitionID: action.DefinitionID, DeclaredType: action.DeclaredType,
				AbilityID: action.AbilityID, ModeIDs: append(make([]string, 0, len(action.ModeIDs)), action.ModeIDs...),
				MinModes: action.MinModes, MaxModes: action.MaxModes, MinTargets: action.MinTargets,
				MaxTargets: action.MaxTargets, PaymentCount: action.PaymentCount,
				InfluenceCosts: append(make([]int, 0, len(action.InfluenceCosts)), action.InfluenceCosts...),
				TargetRefs:     make([]objectRefPayload, 0, len(action.TargetRefs)),
				PaymentRefs:    make([]objectRefPayload, 0, len(action.PaymentRefs)),
				Placements:     make([]PlayerViewPlacement, 0, len(action.Placements)),
				ShieldTargets:  make([]objectRefPayload, 0, len(action.ShieldTargets)),
			}
			if action.SourceRef != nil {
				encoded := codec.encode(*action.SourceRef)
				projected.SourceRef = &encoded
			}
			for _, ref := range action.TargetRefs {
				projected.TargetRefs = append(projected.TargetRefs, codec.encode(ref))
			}
			for _, ref := range action.PaymentRefs {
				projected.PaymentRefs = append(projected.PaymentRefs, codec.encode(ref))
			}
			for _, ref := range action.ShieldTargets {
				projected.ShieldTargets = append(projected.ShieldTargets, codec.encode(ref))
			}
			for _, placement := range action.Placements {
				projected.Placements = append(projected.Placements, PlayerViewPlacement{Lane: placement.Lane, Slot: placement.Slot})
			}
			view.LegalActions = append(view.LegalActions, projected)
		}
	}
	for _, entry := range state.EffectQueue {
		if entry.Effect.SourceRef == nil || !publicObjectRef(state, *entry.Effect.SourceRef) {
			continue
		}
		if name := publicEffectName(entry.Effect.Kind); name != "" {
			view.Queue = append(view.Queue, PlayerViewQueueItem{Position: len(view.Queue) + 1, Kind: name})
		}
	}
	for _, event := range state.Events {
		message := publicEventMessage(event.Kind)
		if message == "" {
			continue
		}
		item := PlayerViewJournalEntry{Sequence: event.Sequence, Message: message}
		if event.Seat != nil {
			item.Seat = seatName(*event.Seat)
		}
		view.Journal = append(view.Journal, item)
	}
	if state.Result != nil {
		result := &PlayerViewResult{Reason: state.Result.Reason}
		if state.Result.WinnerSeat != nil {
			result.WinnerSeat = seatName(*state.Result.WinnerSeat)
		}
		if state.Result.LoserSeat != nil {
			result.LoserSeat = seatName(*state.Result.LoserSeat)
		}
		view.Result = result
	}
	return view
}

func publicObjectRef(state engine.State, ref engine.ObjectRef) bool {
	for _, player := range state.Players {
		for _, object := range player.Objects {
			if object.Ref() == ref {
				return object.Zone == engine.ZoneBoard || object.Zone == engine.ZoneGraveyard
			}
		}
	}
	return false
}

func publicEffectName(kind engine.EffectKind) string {
	switch kind {
	case engine.EffectModifyPower:
		return "Изменение силы"
	case engine.EffectMoveZone:
		return "Перемещение карты"
	case engine.EffectEnqueue:
		return "Добавление эффекта"
	case engine.EffectRequestChoice:
		return "Ожидание решения"
	case engine.EffectScheduleDelayed:
		return "Отложенный эффект"
	case engine.EffectInstallReplacement:
		return "Замещающий эффект"
	default:
		return ""
	}
}

func publicEventMessage(kind string) string {
	switch kind {
	case "match_created":
		return "Матч создан"
	case "setup_completed":
		return "Подготовка матча началась"
	case "starting_hands_dealt":
		return "Начальные руки розданы"
	case "orders_arranged":
		return "Порядок Приказов подтверждён"
	case "mulligan_submitted":
		return "Начальная рука подтверждена"
	case "phase_started":
		return "Началась новая фаза"
	case "draw_skipped":
		return "Добор пропущен"
	case "card_drawn":
		return "Игрок получил карту"
	case "action_passed":
		return "Игрок спасовал"
	case "action_auto_passed":
		return "Действия игрока закончились"
	case "action_committed":
		return "Игровое действие принято"
	case "card_played":
		return "Карта разыграна"
	case "ability_activated":
		return "Способность активирована"
	case "combat_started":
		return "Начался бой"
	case "combat_lane_started":
		return "Разыграна линия боя"
	case "units_collided":
		return "Юниты столкнулись"
	case "unit_died":
		return "Юнит отправлен на кладбище"
	case "location_hit":
		return "Локация получила урон"
	case "location_destroyed":
		return "Локация уничтожена"
	case "hero_attack_skipped":
		return "Атака героя пропущена"
	case "hero_hit":
		return "Герой получил урон"
	case "combat_ended":
		return "Бой завершён"
	case "recovery_completed":
		return "Восстановление завершено"
	case "order_revealed":
		return "Приказ раскрыт"
	case "match_finished", "match_rules_loss", "match_card_effect", "match_surrender", "match_disconnect_forfeit", "match_aborted_technical":
		return "Матч завершён"
	default:
		return ""
	}
}

func projectKnownCard(object engine.CardObject, codec referenceCodec, public bool) PlayerViewCard {
	card := PlayerViewCard{ObjectRef: codec.encode(object.Ref()), DefinitionID: object.DefinitionID}
	if public {
		card.OwnerSeat = seatName(object.OwnerSeat)
		card.ControllerSeat = seatName(object.ControllerSeat)
		card.Lane = object.Lane
		card.Slot = object.Slot
		card.Power = object.Power
		card.Durability = object.Durability
		card.Tapped = object.Tapped
	}
	return card
}

func seatName(seat engine.Seat) string {
	if seat == engine.SeatB {
		return string(applicationSeatTwo)
	}
	return string(applicationSeatOne)
}

type applicationSeatName string

const (
	applicationSeatOne applicationSeatName = "seat-1"
	applicationSeatTwo applicationSeatName = "seat-2"
)
