package engine

import (
	"errors"
	"fmt"
	"strings"
)

func ValidateState(state State) error {
	if state.MatchID == "" {
		return errors.New("matchId is required")
	}
	if err := validateVersions(state.Versions); err != nil {
		return err
	}
	if !state.InitiativeSeat.Valid() || !state.ActingSeat.Valid() {
		return errors.New("initiative and acting seats must be valid")
	}
	if state.TurnNumber < 0 {
		return errors.New("turn number cannot be negative")
	}
	if state.ConsecutivePasses < 0 || state.ConsecutivePasses > 2 {
		return errors.New("consecutive passes must be between zero and two")
	}
	if state.Result == nil && state.Status == StatusFinished {
		return errors.New("finished match must have one result")
	}
	if state.Result != nil && (state.Status != StatusFinished || state.Phase != PhaseFinished) {
		return errors.New("result requires finished status and phase")
	}
	if state.Result != nil {
		if state.Result.WinnerSeat != nil && !state.Result.WinnerSeat.Valid() {
			return errors.New("result winner seat is invalid")
		}
		if state.Result.LoserSeat != nil && !state.Result.LoserSeat.Valid() {
			return errors.New("result loser seat is invalid")
		}
		if state.Result.WinnerSeat != nil && state.Result.LoserSeat != nil && *state.Result.WinnerSeat == *state.Result.LoserSeat {
			return errors.New("winner and loser cannot be the same seat")
		}
	}

	instanceIDs := make(map[string]struct{})
	zoneObjectIDs := make(map[string]struct{})
	occupied := make(map[string]struct{})
	for playerIndex := range state.Players {
		player := state.Players[playerIndex]
		if player.Seat != Seat(playerIndex) {
			return fmt.Errorf("player index %d has seat %d", playerIndex, player.Seat)
		}
		if player.HeroID == "" {
			return fmt.Errorf("seat %d has no hero", player.Seat)
		}
		if player.ActionsRemaining < 0 || player.ActionsRemaining > ActionsPerPlayer {
			return fmt.Errorf("seat %d has invalid action budget %d", player.Seat, player.ActionsRemaining)
		}
		if player.OrdersLocked {
			if len(player.Orders) != OrdersPerPlayer {
				return fmt.Errorf("seat %d must have %d arranged orders", player.Seat, OrdersPerPlayer)
			}
			seenOrders := make(map[string]struct{})
			for index, order := range player.Orders {
				if order.Position != index || order.DefinitionID == "" {
					return fmt.Errorf("seat %d has invalid order position %d", player.Seat, index)
				}
				if _, exists := seenOrders[order.DefinitionID]; exists {
					return fmt.Errorf("seat %d has duplicate order %s", player.Seat, order.DefinitionID)
				}
				seenOrders[order.DefinitionID] = struct{}{}
			}
		}
		for _, object := range player.Objects {
			if object.InstanceID == "" || object.ZoneObjectID == "" {
				return errors.New("card object identifiers are required")
			}
			if _, exists := instanceIDs[object.InstanceID]; exists {
				return fmt.Errorf("card instance %s appears in more than one zone", object.InstanceID)
			}
			instanceIDs[object.InstanceID] = struct{}{}
			if _, exists := zoneObjectIDs[object.ZoneObjectID]; exists {
				return fmt.Errorf("zone object reference %s is not unique", object.ZoneObjectID)
			}
			zoneObjectIDs[object.ZoneObjectID] = struct{}{}
			if !strings.HasSuffix(object.ZoneObjectID, fmt.Sprintf(":g%d", object.Generation)) {
				return fmt.Errorf("object %s reference does not match generation %d", object.InstanceID, object.Generation)
			}
			if !object.OwnerSeat.Valid() || !object.ControllerSeat.Valid() {
				return fmt.Errorf("object %s has invalid owner/controller", object.InstanceID)
			}
			if object.Zone == ZoneBoard {
				validLane := object.Lane >= 0 && object.Lane < LaneCount
				if object.Slot == SlotLocation {
					validLane = object.Lane == -1
				}
				if !validLane || object.Slot == SlotNone {
					return fmt.Errorf("board object %s has invalid position", object.InstanceID)
				}
				key := fmt.Sprintf("%d:%d:%s", object.ControllerSeat, object.Lane, object.Slot)
				if _, exists := occupied[key]; exists {
					return fmt.Errorf("board cell %s is occupied twice", key)
				}
				occupied[key] = struct{}{}
				if object.Slot == SlotUnit && object.Power <= 0 {
					return fmt.Errorf("board unit %s has non-positive power", object.InstanceID)
				}
				if object.Slot == SlotLocation && object.Durability <= 0 {
					return fmt.Errorf("board location %s has non-positive durability", object.InstanceID)
				}
			} else if object.Lane != -1 || object.Slot != SlotNone {
				return fmt.Errorf("off-board object %s retains a board position", object.InstanceID)
			}
		}
	}

	choiceIDs := make(map[string]struct{})
	for _, choice := range state.PendingChoices {
		if choice.ChoiceID == "" || !choice.Seat.Valid() || choice.Revision > state.Revision+1 {
			return errors.New("pending choice identity is invalid")
		}
		if _, exists := choiceIDs[choice.ChoiceID]; exists {
			return fmt.Errorf("pending choice %s is duplicated", choice.ChoiceID)
		}
		choiceIDs[choice.ChoiceID] = struct{}{}
		if len(choice.Options) == 0 {
			return fmt.Errorf("pending choice %s has no legal options", choice.ChoiceID)
		}
	}
	return nil
}

func validateVersions(versions Versions) error {
	values := []string{
		versions.Content,
		versions.DeckManifests,
		versions.EffectSpec,
		versions.Rules,
		versions.Protocol,
		versions.Engine,
	}
	for _, value := range values {
		if value == "" {
			return errors.New("all content/rules/protocol/engine versions must be pinned")
		}
	}
	return nil
}
