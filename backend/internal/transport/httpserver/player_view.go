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
	MatchID        string              `json:"matchId"`
	Versions       engine.Versions     `json:"versions"`
	Revision       uint64              `json:"revision"`
	Status         engine.MatchStatus  `json:"status"`
	Phase          engine.Phase        `json:"phase"`
	CombatStep     engine.CombatStep   `json:"combatStep,omitempty"`
	CombatLane     int                 `json:"combatLane"`
	TurnNumber     int                 `json:"turnNumber"`
	OwnSeat        string              `json:"ownSeat"`
	InitiativeSeat string              `json:"initiativeSeat"`
	ActingSeat     string              `json:"actingSeat"`
	Players        [2]PlayerViewPlayer `json:"players"`
	Board          []PlayerViewCard    `json:"board"`
	Choices        []PlayerViewChoice  `json:"choices"`
	Result         *PlayerViewResult   `json:"result,omitempty"`
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
	ChoiceID    string                   `json:"choiceId,omitempty"`
	WaitingSeat string                   `json:"waitingSeat"`
	Kind        string                   `json:"kind,omitempty"`
	Mandatory   bool                     `json:"mandatory,omitempty"`
	Revision    uint64                   `json:"revision"`
	OptionCount int                      `json:"optionCount,omitempty"`
	Options     []PlayerViewChoiceOption `json:"options,omitempty"`
	Private     bool                     `json:"private"`
	Redacted    bool                     `json:"redacted"`
}

type PlayerViewChoiceOption struct {
	ID      string             `json:"id"`
	Objects []objectRefPayload `json:"objects,omitempty"`
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

func projectPlayerView(state engine.State, viewer engine.Seat, codec referenceCodec) PlayerView {
	view := PlayerView{
		MatchID: state.MatchID, Versions: state.Versions, Revision: state.Revision,
		Status: state.Status, Phase: state.Phase, CombatStep: state.CombatStep,
		CombatLane: state.CombatLane, TurnNumber: state.TurnNumber,
		OwnSeat: seatName(viewer), InitiativeSeat: seatName(state.InitiativeSeat),
		ActingSeat: seatName(state.ActingSeat), Board: make([]PlayerViewCard, 0),
		Choices: make([]PlayerViewChoice, 0),
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
