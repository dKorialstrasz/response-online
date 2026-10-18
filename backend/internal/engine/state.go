package engine

import (
	"errors"
	"fmt"
	"sort"
)

func (state State) Clone() State {
	clone := state
	for index := range state.Players {
		clone.Players[index] = state.Players[index]
		clone.Players[index].Objects = append([]CardObject(nil), state.Players[index].Objects...)
		for objectIndex := range clone.Players[index].Objects {
			clone.Players[index].Objects[objectIndex].Keywords = append(
				[]Keyword(nil), state.Players[index].Objects[objectIndex].Keywords...,
			)
		}
		clone.Players[index].AvailableOrderIDs = append([]string(nil), state.Players[index].AvailableOrderIDs...)
		clone.Players[index].Orders = append([]OrderState(nil), state.Players[index].Orders...)
	}
	clone.PendingChoices = make([]PendingChoice, len(state.PendingChoices))
	for index, choice := range state.PendingChoices {
		clone.PendingChoices[index] = choice
		clone.PendingChoices[index].Options = make([]ChoiceOption, len(choice.Options))
		for optionIndex, option := range choice.Options {
			clone.PendingChoices[index].Options[optionIndex] = option
			clone.PendingChoices[index].Options[optionIndex].Objects = append([]ObjectRef(nil), option.Objects...)
			clone.PendingChoices[index].Options[optionIndex].Effects = cloneResolvedEffects(option.Effects)
		}
	}
	clone.EffectQueue = make([]EffectQueueEntry, len(state.EffectQueue))
	for index, entry := range state.EffectQueue {
		clone.EffectQueue[index] = entry
		clone.EffectQueue[index].Effect = cloneResolvedEffect(entry.Effect)
	}
	clone.Modifiers = make([]ModifierState, len(state.Modifiers))
	for index, modifier := range state.Modifiers {
		clone.Modifiers[index] = modifier
		clone.Modifiers[index].SourceRef = cloneObjectRef(modifier.SourceRef)
		if modifier.BasePowerSet != nil {
			value := *modifier.BasePowerSet
			clone.Modifiers[index].BasePowerSet = &value
		}
	}
	clone.DelayedTriggers = make([]DelayedTriggerState, len(state.DelayedTriggers))
	for index, trigger := range state.DelayedTriggers {
		clone.DelayedTriggers[index] = trigger
		clone.DelayedTriggers[index].SourceRef = cloneObjectRef(trigger.SourceRef)
		clone.DelayedTriggers[index].Effects = cloneResolvedEffects(trigger.Effects)
	}
	clone.Replacements = make([]ReplacementState, len(state.Replacements))
	for index, replacement := range state.Replacements {
		clone.Replacements[index] = replacement
		clone.Replacements[index].SourceRef = cloneObjectRef(replacement.SourceRef)
		clone.Replacements[index].TargetRef = cloneObjectRef(replacement.TargetRef)
		clone.Replacements[index].Effects = cloneResolvedEffects(replacement.Effects)
	}
	if state.ActionResolution != nil {
		resolution := *state.ActionResolution
		clone.ActionResolution = &resolution
	}
	if state.Result != nil {
		result := *state.Result
		if state.Result.WinnerSeat != nil {
			winner := *state.Result.WinnerSeat
			result.WinnerSeat = &winner
		}
		if state.Result.LoserSeat != nil {
			loser := *state.Result.LoserSeat
			result.LoserSeat = &loser
		}
		clone.Result = &result
	}
	clone.Events = make([]Event, len(state.Events))
	for index, event := range state.Events {
		clone.Events[index] = event
		if event.Seat != nil {
			seat := *event.Seat
			clone.Events[index].Seat = &seat
		}
	}
	clone.Commands = make([]CommandRecord, len(state.Commands))
	for index, record := range state.Commands {
		clone.Commands[index] = record
		clone.Commands[index].Command.OrderIDs = append([]string(nil), record.Command.OrderIDs...)
		clone.Commands[index].Command.ObjectRefs = append([]ObjectRef(nil), record.Command.ObjectRefs...)
		clone.Commands[index].Command.SourceRef = cloneObjectRef(record.Command.SourceRef)
		clone.Commands[index].Command.ModeIDs = append([]string(nil), record.Command.ModeIDs...)
		clone.Commands[index].Command.TargetRefs = append([]ObjectRef(nil), record.Command.TargetRefs...)
		clone.Commands[index].Command.PaymentRefs = append([]ObjectRef(nil), record.Command.PaymentRefs...)
		clone.Commands[index].Command.ShieldPaymentRefs = append([]ObjectRef(nil), record.Command.ShieldPaymentRefs...)
		clone.Commands[index].Command.OptionIDs = append([]string(nil), record.Command.OptionIDs...)
	}
	clone.DiagnosticTrace = append([]string(nil), state.DiagnosticTrace...)
	return clone
}

func (state *State) player(seat Seat) *PlayerState {
	return &state.Players[int(seat)]
}

func (state *State) findObject(ref ObjectRef) (*CardObject, error) {
	for playerIndex := range state.Players {
		for objectIndex := range state.Players[playerIndex].Objects {
			object := &state.Players[playerIndex].Objects[objectIndex]
			if object.ZoneObjectID == ref.ZoneObjectID && object.Generation == ref.Generation {
				return object, nil
			}
		}
	}
	return nil, errors.New("object reference is stale or unknown")
}

func (state *State) objectsInZone(seat Seat, zone Zone) []*CardObject {
	objects := make([]*CardObject, 0)
	for index := range state.player(seat).Objects {
		object := &state.player(seat).Objects[index]
		if object.Zone == zone {
			objects = append(objects, object)
		}
	}
	sort.Slice(objects, func(left, right int) bool {
		if objects[left].ZoneIndex != objects[right].ZoneIndex {
			return objects[left].ZoneIndex < objects[right].ZoneIndex
		}
		return objects[left].InstanceID < objects[right].InstanceID
	})
	return objects
}

func moveObject(object *CardObject, zone Zone, zoneIndex int) {
	object.Generation++
	object.ZoneObjectID = fmt.Sprintf("%s:g%d", object.InstanceID, object.Generation)
	object.Zone = zone
	object.ZoneIndex = zoneIndex
	if zone != ZoneBoard {
		object.Lane = -1
		object.Slot = SlotNone
	}
}

func (state *State) draw(seat Seat) (bool, error) {
	deck := state.objectsInZone(seat, ZoneDeck)
	if len(deck) == 0 {
		return false, nil
	}
	hand := state.objectsInZone(seat, ZoneHand)
	moveObject(deck[0], ZoneHand, len(hand))
	for index, object := range state.objectsInZone(seat, ZoneDeck) {
		object.ZoneIndex = index
	}
	return true, nil
}

func (state *State) shuffleDeck(seat Seat) {
	deck := state.objectsInZone(seat, ZoneDeck)
	for index := len(deck) - 1; index > 0; index-- {
		other := state.Random.intn(index + 1)
		deck[index].ZoneIndex, deck[other].ZoneIndex = deck[other].ZoneIndex, deck[index].ZoneIndex
	}
}

func (state *State) randomizeBottom(objects []*CardObject, seat Seat) {
	for index := len(objects) - 1; index > 0; index-- {
		other := state.Random.intn(index + 1)
		objects[index], objects[other] = objects[other], objects[index]
	}
	deckSize := len(state.objectsInZone(seat, ZoneDeck))
	for index, object := range objects {
		moveObject(object, ZoneDeck, deckSize+index)
	}
}

func (state *State) boardObject(seat Seat, lane int, slot BoardSlot) *CardObject {
	for playerIndex := range state.Players {
		for objectIndex := range state.Players[playerIndex].Objects {
			object := &state.Players[playerIndex].Objects[objectIndex]
			if object.Zone == ZoneBoard && object.ControllerSeat == seat && object.Lane == lane && object.Slot == slot {
				return object
			}
		}
	}
	return nil
}

func (state *State) nextClosedOrder(seat Seat) *OrderState {
	player := state.player(seat)
	for index := range player.Orders {
		if !player.Orders[index].Revealed {
			return &player.Orders[index]
		}
	}
	return nil
}

func (state *State) finish(reason ResultReason, winner *Seat, loser *Seat, detail string) {
	if state.Result != nil {
		return
	}
	state.Result = &MatchResult{Reason: reason, WinnerSeat: winner, LoserSeat: loser, Detail: detail}
	state.Status = StatusFinished
	state.Phase = PhaseFinished
	state.CombatStep = CombatNone
	state.CombatLane = -1
	state.PendingChoices = nil
	state.EffectQueue = nil
	state.DelayedTriggers = nil
	state.Replacements = nil
	state.ActionResolution = nil
}
