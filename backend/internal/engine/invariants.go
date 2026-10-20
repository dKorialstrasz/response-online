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
	if state.Result != nil && (len(state.PendingChoices) > 0 || len(state.EffectQueue) > 0 || state.ActionResolution != nil) {
		return errors.New("finished match cannot retain unresolved effects or choices")
	}
	if state.ActionResolution != nil && (!state.ActionResolution.Seat.Valid() || state.Phase != PhaseAction) {
		return errors.New("action resolution requires a valid seat during the action phase")
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
		if choice.ChoiceID == "" || !choice.Seat.Valid() || choice.Revision > state.Revision+1 ||
			(choice.Visibility != ChoicePublic && choice.Visibility != ChoiceOwnerOnly) ||
			!validQueuePosition(choice.QueuePosition) {
			return errors.New("pending choice identity is invalid")
		}
		if _, exists := choiceIDs[choice.ChoiceID]; exists {
			return fmt.Errorf("pending choice %s is duplicated", choice.ChoiceID)
		}
		choiceIDs[choice.ChoiceID] = struct{}{}
		if len(choice.Options) == 0 {
			return fmt.Errorf("pending choice %s has no legal options", choice.ChoiceID)
		}
		if choice.MinSelections < 0 || choice.MaxSelections < choice.MinSelections || choice.MaxSelections > len(choice.Options) ||
			(choice.Mandatory && choice.MinSelections == 0) || (!choice.Mandatory && choice.MinSelections != 0) {
			return fmt.Errorf("pending choice %s has invalid selection bounds", choice.ChoiceID)
		}
		optionIDs := make(map[string]struct{}, len(choice.Options))
		for _, option := range choice.Options {
			if option.ID == "" {
				return fmt.Errorf("pending choice %s has an empty option id", choice.ChoiceID)
			}
			if _, exists := optionIDs[option.ID]; exists {
				return fmt.Errorf("pending choice %s repeats option %s", choice.ChoiceID, option.ID)
			}
			optionIDs[option.ID] = struct{}{}
		}
		if choice.Continuation != nil {
			if choice.Continuation.Handler == "" || !choice.Continuation.Controller.Valid() {
				return fmt.Errorf("pending choice %s has an invalid continuation", choice.ChoiceID)
			}
			if choice.Continuation.SourceRef != nil {
				if _, err := state.findObject(*choice.Continuation.SourceRef); err != nil {
					return fmt.Errorf("pending choice %s has a stale continuation source", choice.ChoiceID)
				}
			}
		}
	}

	entryIDs := make(map[string]struct{}, len(state.EffectQueue))
	entrySequences := make(map[uint64]struct{}, len(state.EffectQueue))
	for _, entry := range state.EffectQueue {
		if entry.EntryID == "" || entry.BatchID == "" || entry.EnqueueSequence == 0 || !entry.Effect.Controller.Valid() {
			return errors.New("effect queue entry identity is invalid")
		}
		if _, exists := entryIDs[entry.EntryID]; exists {
			return fmt.Errorf("effect queue entry %s is duplicated", entry.EntryID)
		}
		entryIDs[entry.EntryID] = struct{}{}
		if _, exists := entrySequences[entry.EnqueueSequence]; exists {
			return fmt.Errorf("effect queue sequence %d is duplicated", entry.EnqueueSequence)
		}
		entrySequences[entry.EnqueueSequence] = struct{}{}
		if _, exists := effectHandlerRegistry()[entry.Effect.Kind]; !exists {
			return fmt.Errorf("effect queue entry %s has unknown kind", entry.EntryID)
		}
	}

	modifierIDs := make(map[string]struct{}, len(state.Modifiers))
	for _, modifier := range state.Modifiers {
		if modifier.ModifierID == "" || modifier.CreatedSequence == 0 || !validDuration(modifier.Duration) {
			return errors.New("modifier identity or duration is invalid")
		}
		if _, exists := modifierIDs[modifier.ModifierID]; exists {
			return fmt.Errorf("modifier %s is duplicated", modifier.ModifierID)
		}
		modifierIDs[modifier.ModifierID] = struct{}{}
		if _, err := state.findObject(modifier.TargetRef); err != nil {
			return fmt.Errorf("modifier %s has a stale target", modifier.ModifierID)
		}
	}

	triggerIDs := make(map[string]struct{}, len(state.DelayedTriggers))
	for _, trigger := range state.DelayedTriggers {
		if trigger.TriggerID == "" || trigger.EventKind == "" || !trigger.Controller.Valid() ||
			!validDuration(trigger.Duration) || len(trigger.Effects) == 0 {
			return errors.New("delayed trigger identity is invalid")
		}
		if _, exists := triggerIDs[trigger.TriggerID]; exists {
			return fmt.Errorf("delayed trigger %s is duplicated", trigger.TriggerID)
		}
		triggerIDs[trigger.TriggerID] = struct{}{}
	}

	replacementIDs := make(map[string]struct{}, len(state.Replacements))
	for _, replacement := range state.Replacements {
		if replacement.ReplacementID == "" || !replacement.Controller.Valid() || replacement.EffectKind == "" ||
			!validDuration(replacement.Duration) || len(replacement.Effects) == 0 {
			return errors.New("replacement identity is invalid")
		}
		if _, exists := replacementIDs[replacement.ReplacementID]; exists {
			return fmt.Errorf("replacement %s is duplicated", replacement.ReplacementID)
		}
		replacementIDs[replacement.ReplacementID] = struct{}{}
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
