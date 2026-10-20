package engine

import "fmt"

const (
	choiceHandlerBaz036          = "baz-036.front.ability:discard-or-sacrifice"
	choiceHandlerBaz121          = "baz-121.front.ability:topdeck"
	choiceHandlerOwnDiscard      = "base:own-discard"
	choiceHandlerBaz044Discard   = "baz-044.front.ability:discard"
	choiceHandlerBaz044Sacrifice = "baz-044.front.ability:sacrifice"
	choiceHandlerBaz091Void      = "baz-091.front.ability:void"
	choiceHandlerBaz105Void      = "baz-105.front.ability:void"
	choiceHandlerBaz020Pick      = "baz-020.front.ability:pick"
	choiceHandlerBaz020Order     = "baz-020.front.ability:order"
	choiceHandlerBaz038Pick      = "baz-038.front.ability:pick"
	choiceHandlerBaz102Pick      = "baz-102.front.ability:pick"
	choiceHandlerBaz009Move      = "baz-009.front.ability:move"
	choiceHandlerBaz029Discard   = "baz-029.front.ability:discard"
	choiceHandlerHighestCost     = "base:highest-cost-sacrifice"
	choiceHandlerBaz112Topdeck   = "baz-112.front.ability:topdeck"
	choiceHandlerBaz127Move      = "baz-127.front.ability:move"
	choiceHandlerBaz015Move      = "baz-015.front.ability:move"
	choiceHandlerBaz021Targets   = "baz-021.front.ability:targets"
	choiceHandlerBaz025Return    = "baz-025.front.ability:return"
	choiceHandlerBaz047Shuffle   = "baz-047.front.ability:shuffle"
	choiceHandlerBaz059Exchange  = "baz-059.front.ability:exchange"
	choiceHandlerBaz065Shuffle   = "baz-065.front.ability:shuffle"
	choiceHandlerBaz066Placement = "baz-066.front.ability:placement"
	choiceHandlerBaz072Draw      = "baz-072.front.ability:draw"
	choiceHandlerBaz088Target    = "baz-088.front.ability:target"
	choiceHandlerBaz107Target    = "baz-107.front.ability:target"
	choiceHandlerPotionDeath     = "base:potion-death"
)

func optionalSingleOption() []ChoiceOption { return []ChoiceOption{{ID: "activate"}} }

func choiceOptions(objects []*CardObject) []ChoiceOption {
	options := make([]ChoiceOption, 0, len(objects))
	for _, object := range objects {
		options = append(options, ChoiceOption{ID: fmt.Sprintf("option-%d", len(options)+1), Objects: []ObjectRef{object.Ref()}})
	}
	return options
}

func (context *builtinAbilityContext) requestBaz009Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-009 optional move is a pending choice", ErrInvalidCommand)
	}
	source, err := context.sourceObject()
	if err != nil {
		return err
	}
	options := make([]ChoiceOption, 0)
	for playerIndex := range context.state.Players {
		for objectIndex := range context.state.Players[playerIndex].Objects {
			object := &context.state.Players[playerIndex].Objects[objectIndex]
			if object.Zone != ZoneBoard || (object.Slot != SlotUnit && object.Slot != SlotSupport) || object.InstanceID == source.InstanceID {
				continue
			}
			for lane := 0; lane < LaneCount; lane++ {
				if lane != object.Lane && context.state.boardObject(object.ControllerSeat, lane, object.Slot) == nil {
					options = append(options, ChoiceOption{ID: fmt.Sprintf("option-%d", len(options)+1), Objects: []ObjectRef{object.Ref()}, Lane: lane, LaneSet: true, Slot: object.Slot})
				}
			}
		}
	}
	if len(options) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_move_other_card", false, ChoicePublic, 0, 1, false, options,
		ChoiceContinuation{Handler: choiceHandlerBaz009Move, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestOwnDiscard(handler string) error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: discard is a pending choice", ErrInvalidCommand)
	}
	options := choiceOptions(context.state.objectsInZone(context.command.Seat, ZoneHand))
	if len(options) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "discard_from_hand", true, ChoiceOwnerOnly, 1, 1, false, options,
		ChoiceContinuation{Handler: handler, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestHighestCostSacrifice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: highest-cost tie is chosen by the opponent", ErrInvalidCommand)
	}
	opponent := context.command.Seat.Other()
	board := context.state.objectsInZone(opponent, ZoneBoard)
	maximum := -1
	candidates := make([]*CardObject, 0)
	for _, object := range board {
		cost := 0
		if definition, ok := context.budget.cards[object.DefinitionID]; ok && definition.Cost != nil {
			cost = *definition.Cost
		}
		if cost > maximum {
			maximum, candidates = cost, []*CardObject{object}
		} else if cost == maximum {
			candidates = append(candidates, object)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		moveToGraveyard(context.state, candidates[0])
		pruneStaleEffectState(context.state)
		return nil
	}
	return requestBuiltinChoice(context.state, opponent, "choose_highest_cost_sacrifice", true, ChoicePublic, 1, 1, false, choiceOptions(candidates),
		ChoiceContinuation{Handler: choiceHandlerHighestCost, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz112Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-112 summon decision is optional pending choice", ErrInvalidCommand)
	}
	objects := make([]*CardObject, 0)
	objects = append(objects, context.state.objectsInZone(context.command.Seat, ZoneGraveyard)...)
	objects = append(objects, context.state.objectsInZone(context.command.Seat, ZoneVoid)...)
	if len(objects) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_topdeck_from_grave_or_void", false, ChoiceOwnerOnly, 0, 1, false, choiceOptions(objects),
		ChoiceContinuation{Handler: choiceHandlerBaz112Topdeck, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz127Move() error {
	if context.command.DestinationLaneSet {
		return fmt.Errorf("%w: baz-127 movement is chosen after the power effect", ErrInvalidCommand)
	}
	if err := context.modifyTargets(2, DurationUntilEndOfTurn); err != nil {
		return err
	}
	target, err := context.target(0)
	if err != nil || target.Zone != ZoneBoard || target.Slot != SlotUnit || target.ControllerSeat != context.command.Seat {
		return fmt.Errorf("%w: baz-127 target must be your board unit", ErrInvalidCommand)
	}
	options := make([]ChoiceOption, 0)
	for lane := 0; lane < LaneCount; lane++ {
		if lane != target.Lane && context.state.boardObject(context.command.Seat, lane, SlotUnit) == nil {
			options = append(options, ChoiceOption{ID: fmt.Sprintf("option-%d", len(options)+1), Objects: []ObjectRef{target.Ref()}, Lane: lane, LaneSet: true, Slot: SlotUnit})
		}
	}
	if len(options) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_move_boosted_unit", false, ChoicePublic, 0, 1, false, options,
		ChoiceContinuation{Handler: choiceHandlerBaz127Move, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz015Move() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-015 is an optional pending choice", ErrInvalidCommand)
	}
	source, err := context.sourceObject()
	if err != nil || source.Zone != ZoneBoard || source.Slot != SlotSupport {
		return fmt.Errorf("%w: baz-015 source must be a board enhancement", ErrInvalidCommand)
	}
	if context.state.boardObject(context.command.Seat, source.Lane, SlotUnit) != nil {
		return nil
	}
	objects := make([]*CardObject, 0)
	for _, object := range context.state.objectsInZone(context.command.Seat, ZoneBoard) {
		if object.Slot == SlotUnit {
			objects = append(objects, object)
		}
	}
	if len(objects) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_move_unit_before_source", false, ChoicePublic, 0, 1, false, choiceOptions(objects),
		ChoiceContinuation{Handler: choiceHandlerBaz015Move, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz021Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-021 voluntary targets use pending choice", ErrInvalidCommand)
	}
	objects := make([]*CardObject, 0)
	for _, object := range context.state.objectsInZone(context.command.Seat.Other(), ZoneBoard) {
		if object.Slot == SlotUnit {
			objects = append(objects, object)
		}
	}
	if len(objects) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "choose_up_to_two_opponent_units", false, ChoicePublic, 0, minInt(2, len(objects)), false, choiceOptions(objects),
		ChoiceContinuation{Handler: choiceHandlerBaz021Targets, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz025Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-025 is an optional pending choice", ErrInvalidCommand)
	}
	if context.source == nil {
		return fmt.Errorf("%w: baz-025 source is required", ErrInvalidCommand)
	}
	object, err := context.state.findObject(*context.source)
	if err != nil || object.OwnerSeat != context.command.Seat || object.Zone != ZoneGraveyard {
		return fmt.Errorf("%w: baz-025 source must be in its controller's graveyard", ErrInvalidCommand)
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_return_source_from_grave", false, ChoicePublic, 0, 1, false,
		[]ChoiceOption{{ID: "return", Objects: []ObjectRef{object.Ref()}}}, ChoiceContinuation{Handler: choiceHandlerBaz025Return, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz047Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-047 is an optional pending choice", ErrInvalidCommand)
	}
	objects := append([]*CardObject(nil), context.state.objectsInZone(context.command.Seat, ZoneGraveyard)...)
	objects = append(objects, context.state.objectsInZone(context.command.Seat, ZoneVoid)...)
	if len(objects) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_shuffle_grave_or_void_card", false, ChoiceOwnerOnly, 0, 1, false, choiceOptions(objects),
		ChoiceContinuation{Handler: choiceHandlerBaz047Shuffle, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz059Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-059 exchange target is chosen after the boost", ErrInvalidCommand)
	}
	if err := context.modifyEnhancedUnit(2, DurationUntilEndOfTurn); err != nil {
		return err
	}
	enhanced, err := context.enhancedUnit()
	if err != nil {
		return err
	}
	objects := make([]*CardObject, 0)
	for _, object := range context.state.objectsInZone(context.command.Seat, ZoneBoard) {
		if object.Slot == SlotUnit && object.InstanceID != enhanced.InstanceID {
			objects = append(objects, object)
		}
	}
	if len(objects) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_exchange_enhanced_unit", false, ChoicePublic, 0, 1, false, choiceOptions(objects),
		ChoiceContinuation{Handler: choiceHandlerBaz059Exchange, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source), TargetRefs: []ObjectRef{enhanced.Ref()}}, context.budget)
}

func (context *builtinAbilityContext) requestBaz065Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-065 voluntary targets use pending choice", ErrInvalidCommand)
	}
	objects := append([]*CardObject(nil), context.state.objectsInZone(context.command.Seat, ZoneGraveyard)...)
	objects = append(objects, context.state.objectsInZone(context.command.Seat, ZoneVoid)...)
	if len(objects) == 0 {
		return context.draw(context.command.Seat, 2)
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "choose_up_to_two_to_shuffle", false, ChoiceOwnerOnly, 0, minInt(2, len(objects)), false, choiceOptions(objects),
		ChoiceContinuation{Handler: choiceHandlerBaz065Shuffle, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz072Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-072 is an optional pending choice", ErrInvalidCommand)
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_draw_two", false, ChoicePublic, 0, 1, false, optionalSingleOption(),
		ChoiceContinuation{Handler: choiceHandlerBaz072Draw, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz066Choice() error {
	if context.command.DestinationLaneSet || len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-066 placement is chosen after shuffle and reveal", ErrInvalidCommand)
	}
	context.state.shuffleDeck(context.command.Seat)
	deck := context.state.objectsInZone(context.command.Seat, ZoneDeck)
	if len(deck) == 0 {
		return context.draw(context.command.Seat, 2)
	}
	top := deck[0]
	definition, ok := context.budget.cards[top.DefinitionID]
	if !ok || !cardHasType(definition, CardUnit) || definition.Cost == nil || *definition.Cost > 4 {
		return context.draw(context.command.Seat, 2)
	}
	options := make([]ChoiceOption, 0, LaneCount)
	for lane := 0; lane < LaneCount; lane++ {
		if context.state.boardObject(context.command.Seat, lane, SlotUnit) == nil {
			options = append(options, ChoiceOption{ID: fmt.Sprintf("lane-%d", lane), Objects: []ObjectRef{top.Ref()}, Lane: lane, LaneSet: true, Slot: SlotUnit})
		}
	}
	if len(options) == 0 {
		return context.draw(context.command.Seat, 2)
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "optional_place_revealed_top_unit", false, ChoicePublic, 0, 1, false, options,
		ChoiceContinuation{Handler: choiceHandlerBaz066Placement, Controller: context.command.Seat, TargetRefs: []ObjectRef{top.Ref()}}, context.budget)
}

func (context *builtinAbilityContext) requestBaz088Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-088 voluntary target uses pending choice", ErrInvalidCommand)
	}
	objects := make([]*CardObject, 0)
	for playerIndex := range context.state.Players {
		for _, object := range context.state.objectsInZone(Seat(playerIndex), ZoneBoard) {
			if object.Slot == SlotUnit {
				objects = append(objects, object)
			}
		}
	}
	if len(objects) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "choose_up_to_one_unit_ignore_shield", false, ChoicePublic, 0, 1, false, choiceOptions(objects),
		ChoiceContinuation{Handler: choiceHandlerBaz088Target, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz107Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-107 voluntary target uses pending choice", ErrInvalidCommand)
	}
	objects := make([]*CardObject, 0)
	for playerIndex := range context.state.Players {
		for objectIndex := range context.state.Players[playerIndex].Objects {
			object := &context.state.Players[playerIndex].Objects[objectIndex]
			if object.Zone != ZoneDeck && object.Zone != ZoneHand && object.Zone != ZoneVoid {
				objects = append(objects, object)
			}
		}
	}
	if len(objects) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "choose_up_to_one_card_to_void", false, ChoicePublic, 0, 1, false, choiceOptions(objects),
		ChoiceContinuation{Handler: choiceHandlerBaz107Target, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestPotionDeathChoice(number int) error {
	if len(context.command.TargetRefs) != 0 || context.command.DestinationLaneSet {
		return fmt.Errorf("%w: potion death target is chosen after draw and sacrifice", ErrInvalidCommand)
	}
	if err := context.draw(context.command.Seat, 1); err != nil {
		return err
	}
	if err := context.moveSourceTo(ZoneGraveyard); err != nil {
		return err
	}

	options := make([]ChoiceOption, 0)
	switch number {
	case 16:
		for playerIndex := range context.state.Players {
			for _, object := range context.state.objectsInZone(Seat(playerIndex), ZoneBoard) {
				for lane := 0; lane < LaneCount; lane++ {
					if lane != object.Lane && context.state.boardObject(object.ControllerSeat, lane, object.Slot) == nil {
						options = append(options, ChoiceOption{ID: fmt.Sprintf("option-%d", len(options)+1), Objects: []ObjectRef{object.Ref()}, Lane: lane, LaneSet: true, Slot: object.Slot})
					}
				}
			}
		}
	case 34:
		for _, object := range context.state.objectsInZone(context.command.Seat, ZoneGraveyard) {
			definition, ok := context.budget.cards[object.DefinitionID]
			if !ok || !cardHasType(definition, CardUnit) || definition.Cost == nil || *definition.Cost > 4 {
				continue
			}
			for lane := 0; lane < LaneCount; lane++ {
				if context.state.boardObject(context.command.Seat, lane, SlotUnit) == nil {
					options = append(options, ChoiceOption{ID: fmt.Sprintf("option-%d", len(options)+1), Objects: []ObjectRef{object.Ref()}, Lane: lane, LaneSet: true, Slot: SlotUnit})
				}
			}
		}
	case 60, 78, 99:
		for playerIndex := range context.state.Players {
			for _, object := range context.state.objectsInZone(Seat(playerIndex), ZoneBoard) {
				if object.Slot == SlotUnit {
					options = append(options, ChoiceOption{ID: fmt.Sprintf("option-%d", len(options)+1), Objects: []ObjectRef{object.Ref()}})
				}
			}
		}
	case 126:
		for _, object := range context.state.objectsInZone(context.command.Seat.Other(), ZoneBoard) {
			if object.Slot != SlotUnit || object.Power > 2 {
				continue
			}
			for lane := 0; lane < LaneCount; lane++ {
				if context.state.boardObject(context.command.Seat, lane, SlotUnit) == nil {
					options = append(options, ChoiceOption{ID: fmt.Sprintf("option-%d", len(options)+1), Objects: []ObjectRef{object.Ref()}, Lane: lane, LaneSet: true, Slot: SlotUnit})
				}
			}
		}
	default:
		return fmt.Errorf("%w: unsupported potion handler", ErrInvalidCommand)
	}
	if len(options) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, fmt.Sprintf("baz_%03d_death_target", number), true, ChoicePublic, 1, 1, false, options,
		ChoiceContinuation{Handler: choiceHandlerPotionDeath, Controller: context.command.Seat, Step: number}, context.budget)
}

func (context *builtinAbilityContext) requestBaz036Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-036 target is chosen by the opponent after resolution starts", ErrInvalidCommand)
	}
	opponent := context.command.Seat.Other()
	options := choiceOptions(context.state.objectsInZone(opponent, ZoneHand))
	for _, object := range context.state.objectsInZone(opponent, ZoneBoard) {
		options = append(options, ChoiceOption{ID: fmt.Sprintf("option-%d", len(options)+1), Objects: []ObjectRef{object.Ref()}})
	}
	if len(options) == 0 {
		return nil
	}
	return requestBuiltinChoice(
		context.state,
		opponent,
		"opponent_discard_or_sacrifice",
		true,
		ChoiceOwnerOnly,
		1,
		1,
		false,
		options,
		ChoiceContinuation{Handler: choiceHandlerBaz036, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)},
		context.budget,
	)
}

func (context *builtinAbilityContext) requestBaz121Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-121 choice is created from the current hand after drawing", ErrInvalidCommand)
	}
	if err := context.draw(context.command.Seat, 2); err != nil {
		return err
	}
	options := choiceOptions(context.state.objectsInZone(context.command.Seat, ZoneHand))
	if len(options) == 0 {
		return nil
	}
	return requestBuiltinChoice(
		context.state,
		context.command.Seat,
		"put_hand_card_on_top",
		true,
		ChoiceOwnerOnly,
		1,
		1,
		false,
		options,
		ChoiceContinuation{Handler: choiceHandlerBaz121, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)},
		context.budget,
	)
}

func (context *builtinAbilityContext) drawThenRequestDiscard(count int) error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: discard is chosen after drawing", ErrInvalidCommand)
	}
	if err := context.draw(context.command.Seat, count); err != nil {
		return err
	}
	options := choiceOptions(context.state.objectsInZone(context.command.Seat, ZoneHand))
	if len(options) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "discard_from_current_hand", true, ChoiceOwnerOnly, 1, 1, false, options,
		ChoiceContinuation{Handler: choiceHandlerOwnDiscard, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz044Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-044 decisions belong to the opponent", ErrInvalidCommand)
	}
	opponent := context.command.Seat.Other()
	options := choiceOptions(context.state.objectsInZone(opponent, ZoneHand))
	if len(options) == 0 {
		return context.requestBaz044Sacrifice(opponent)
	}
	return requestBuiltinChoice(context.state, opponent, "discard_from_hand", true, ChoiceOwnerOnly, 1, 1, false, options,
		ChoiceContinuation{Handler: choiceHandlerBaz044Discard, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz044Sacrifice(opponent Seat) error {
	options := choiceOptions(context.state.objectsInZone(opponent, ZoneBoard))
	if len(options) == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, opponent, "sacrifice_own_board_card", true, ChoicePublic, 1, 1, false, options,
		ChoiceContinuation{Handler: choiceHandlerBaz044Sacrifice, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestOpponentHandMove(handler, kind string) error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: opponent hand decision cannot be supplied by the activator", ErrInvalidCommand)
	}
	opponent := context.command.Seat.Other()
	options := choiceOptions(context.state.objectsInZone(opponent, ZoneHand))
	if len(options) == 0 {
		if handler == choiceHandlerBaz105Void {
			return context.draw(context.command.Seat, 1)
		}
		return nil
	}
	return requestBuiltinChoice(context.state, opponent, kind, true, ChoiceOwnerOnly, 1, 1, false, options,
		ChoiceContinuation{Handler: handler, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz020Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-020 looks before choosing", ErrInvalidCommand)
	}
	deck := context.state.objectsInZone(context.command.Seat, ZoneDeck)
	look := minInt(4, len(deck))
	if look == 0 {
		return nil
	}
	pick := minInt(2, look)
	return requestBuiltinChoice(context.state, context.command.Seat, "choose_two_from_top_four", true, ChoiceOwnerOnly, pick, pick, false, choiceOptions(deck[:look]),
		ChoiceContinuation{Handler: choiceHandlerBaz020Pick, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, context.budget)
}

func (context *builtinAbilityContext) requestBaz038Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-038 looks before choosing", ErrInvalidCommand)
	}
	deck := context.state.objectsInZone(context.command.Seat, ZoneDeck)
	look := minInt(2, len(deck))
	if look == 0 {
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "choose_one_of_top_two", true, ChoiceOwnerOnly, 1, 1, false, choiceOptions(deck[:look]),
		ChoiceContinuation{Handler: choiceHandlerBaz038Pick, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source), TargetRefs: objectRefs(deck[:look])}, context.budget)
}

func (context *builtinAbilityContext) requestBaz102Choice() error {
	if len(context.command.TargetRefs) != 0 {
		return fmt.Errorf("%w: baz-102 reveals before choosing", ErrInvalidCommand)
	}
	deck := context.state.objectsInZone(context.command.Seat, ZoneDeck)
	look := minInt(4, len(deck))
	if look == 0 {
		return nil
	}
	eligible := make([]*CardObject, 0, look)
	for _, object := range deck[:look] {
		definition, ok := context.budget.cards[object.DefinitionID]
		if ok && cardHasType(definition, CardUnit) {
			eligible = append(eligible, object)
		}
	}
	if len(eligible) == 0 {
		context.state.randomizeBottom(deck[:look], context.command.Seat)
		return nil
	}
	return requestBuiltinChoice(context.state, context.command.Seat, "choose_revealed_unit", false, ChoicePublic, 0, 1, false, choiceOptions(eligible),
		ChoiceContinuation{Handler: choiceHandlerBaz102Pick, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source), TargetRefs: objectRefs(deck[:look])}, context.budget)
}

func requestBuiltinChoice(
	state *State,
	seat Seat,
	kind string,
	mandatory bool,
	visibility ChoiceVisibility,
	minimum int,
	maximum int,
	orderMatters bool,
	options []ChoiceOption,
	continuation ChoiceContinuation,
	budget *eventBudget,
) error {
	if len(options) == 0 || minimum < 0 || maximum < minimum || maximum > len(options) || (mandatory && minimum == 0) || (!mandatory && minimum != 0) {
		return fmt.Errorf("%w: invalid dynamic choice", ErrInvalidCommand)
	}
	state.NextChoiceSequence++
	choice := PendingChoice{
		ChoiceID: fmt.Sprintf("choice-%d", state.NextChoiceSequence), Seat: seat, Kind: kind,
		Mandatory: mandatory, Visibility: visibility, MinSelections: minimum, MaxSelections: maximum,
		OrderMatters: orderMatters, QueuePosition: QueueBeforeExisting, Revision: state.Revision + 1,
		Options: make([]ChoiceOption, len(options)), Continuation: &continuation,
	}
	for index, option := range options {
		choice.Options[index] = ChoiceOption{ID: option.ID, Objects: append([]ObjectRef(nil), option.Objects...), Lane: option.Lane, LaneSet: option.LaneSet, Slot: option.Slot}
	}
	state.PendingChoices = append(state.PendingChoices, choice)
	seatCopy := seat
	return emitEffectEvent(state, budget, "choice_requested", &seatCopy, kind)
}

func resumeBuiltinChoice(state *State, choice PendingChoice, selected []ChoiceOption, budget *eventBudget) error {
	if choice.Continuation == nil || !choice.Continuation.Controller.Valid() {
		return fmt.Errorf("%w: choice continuation is invalid", ErrInvalidCommand)
	}
	context := &builtinAbilityContext{
		state:   state,
		command: Command{Seat: choice.Continuation.Controller},
		source:  cloneObjectRef(choice.Continuation.SourceRef),
		targets: append([]ObjectRef(nil), choice.Continuation.TargetRefs...),
		budget:  budget,
	}
	switch choice.Continuation.Handler {
	case choiceHandlerBaz036:
		if len(selected) != 1 || len(selected[0].Objects) != 1 {
			return fmt.Errorf("%w: baz-036 requires one current option", ErrInvalidCommand)
		}
		object, err := state.findObject(selected[0].Objects[0])
		if err != nil || object.ControllerSeat != choice.Seat || (object.Zone != ZoneHand && object.Zone != ZoneBoard) {
			return fmt.Errorf("%w: baz-036 option is stale, foreign, or in the wrong zone", ErrInvalidCommand)
		}
		moveToGraveyard(state, object)
		pruneStaleEffectState(state)
		return nil
	case choiceHandlerBaz121:
		if len(selected) != 1 || len(selected[0].Objects) != 1 {
			return fmt.Errorf("%w: baz-121 requires one current hand card", ErrInvalidCommand)
		}
		object, err := state.findObject(selected[0].Objects[0])
		if err != nil || object.OwnerSeat != choice.Seat || object.ControllerSeat != choice.Seat || object.Zone != ZoneHand {
			return fmt.Errorf("%w: baz-121 option is stale, foreign, or outside the current hand", ErrInvalidCommand)
		}
		context.targets = []ObjectRef{object.Ref()}
		return context.moveTargetToDeck(true)
	case choiceHandlerOwnDiscard:
		return moveSelectedOwnedCard(state, choice, selected, ZoneHand, ZoneGraveyard)
	case choiceHandlerBaz044Discard:
		if err := moveSelectedOwnedCard(state, choice, selected, ZoneHand, ZoneGraveyard); err != nil {
			return err
		}
		return context.requestBaz044Sacrifice(choice.Seat)
	case choiceHandlerBaz044Sacrifice:
		return moveSelectedControlledCard(state, choice, selected, ZoneBoard, ZoneGraveyard)
	case choiceHandlerBaz091Void:
		return moveSelectedOwnedCard(state, choice, selected, ZoneHand, ZoneVoid)
	case choiceHandlerBaz105Void:
		if err := moveSelectedOwnedCard(state, choice, selected, ZoneHand, ZoneVoid); err != nil {
			return err
		}
		if len(state.objectsInZone(choice.Seat, ZoneHand)) == 0 {
			return context.draw(context.command.Seat, 1)
		}
		return nil
	case choiceHandlerBaz020Pick:
		selectedRefs, err := selectedObjectRefs(selected)
		if err != nil {
			return err
		}
		selectedSet := make(map[ObjectRef]struct{}, len(selectedRefs))
		for _, ref := range selectedRefs {
			object, err := state.findObject(ref)
			if err != nil || object.OwnerSeat != choice.Seat || object.Zone != ZoneDeck {
				return fmt.Errorf("%w: baz-020 selection is stale", ErrInvalidCommand)
			}
			selectedSet[ref] = struct{}{}
			moveObject(object, ZoneHand, len(state.objectsInZone(choice.Seat, ZoneHand)))
		}
		remaining := make([]*CardObject, 0, 2)
		for _, option := range choice.Options {
			if _, chosen := selectedSet[option.Objects[0]]; chosen {
				continue
			}
			if object, err := state.findObject(option.Objects[0]); err == nil && object.Zone == ZoneDeck {
				remaining = append(remaining, object)
			}
		}
		if len(remaining) == 0 {
			return nil
		}
		return requestBuiltinChoice(state, choice.Seat, "order_remaining_on_bottom", true, ChoiceOwnerOnly, len(remaining), len(remaining), true, choiceOptions(remaining),
			ChoiceContinuation{Handler: choiceHandlerBaz020Order, Controller: context.command.Seat, SourceRef: cloneObjectRef(context.source)}, budget)
	case choiceHandlerBaz020Order:
		refs, err := selectedObjectRefs(selected)
		if err != nil {
			return err
		}
		return hideDeckCardsOnBottom(state, choice.Seat, refs)
	case choiceHandlerBaz038Pick:
		refs, err := selectedObjectRefs(selected)
		if err != nil || len(refs) != 1 {
			return fmt.Errorf("%w: baz-038 requires one card", ErrInvalidCommand)
		}
		chosen := refs[0]
		for _, ref := range choice.Continuation.TargetRefs {
			object, findErr := state.findObject(ref)
			if findErr != nil || object.OwnerSeat != choice.Seat || object.Zone != ZoneDeck {
				return fmt.Errorf("%w: baz-038 prefix changed", ErrInvalidCommand)
			}
			if ref == chosen {
				moveObject(object, ZoneHand, len(state.objectsInZone(choice.Seat, ZoneHand)))
			} else {
				moveToGraveyard(state, object)
			}
		}
		return nil
	case choiceHandlerBaz102Pick:
		var chosen *ObjectRef
		if len(selected) == 1 {
			refs, err := selectedObjectRefs(selected)
			if err != nil {
				return err
			}
			chosen = &refs[0]
		}
		rest := make([]*CardObject, 0, len(choice.Continuation.TargetRefs))
		for _, ref := range choice.Continuation.TargetRefs {
			object, err := state.findObject(ref)
			if err != nil || object.OwnerSeat != choice.Seat || object.Zone != ZoneDeck {
				return fmt.Errorf("%w: baz-102 prefix changed", ErrInvalidCommand)
			}
			if chosen != nil && ref == *chosen {
				moveObject(object, ZoneHand, len(state.objectsInZone(choice.Seat, ZoneHand)))
			} else {
				rest = append(rest, object)
			}
		}
		state.randomizeBottom(rest, choice.Seat)
		return nil
	case choiceHandlerBaz009Move, choiceHandlerBaz127Move:
		if len(selected) == 0 {
			return nil
		}
		if len(selected) != 1 || len(selected[0].Objects) != 1 || !selected[0].LaneSet {
			return fmt.Errorf("%w: move choice is invalid", ErrInvalidCommand)
		}
		object, err := state.findObject(selected[0].Objects[0])
		if err != nil || object.Zone != ZoneBoard || state.boardObject(object.ControllerSeat, selected[0].Lane, object.Slot) != nil {
			return fmt.Errorf("%w: move choice became stale", ErrInvalidCommand)
		}
		object.Lane = selected[0].Lane
		return nil
	case choiceHandlerBaz029Discard:
		return moveSelectedOwnedCard(state, choice, selected, ZoneHand, ZoneGraveyard)
	case choiceHandlerHighestCost:
		return moveSelectedControlledCard(state, choice, selected, ZoneBoard, ZoneGraveyard)
	case choiceHandlerBaz112Topdeck:
		if len(selected) == 0 {
			return nil
		}
		refs, err := selectedObjectRefs(selected)
		if err != nil || len(refs) != 1 {
			return fmt.Errorf("%w: baz-112 selection is invalid", ErrInvalidCommand)
		}
		object, err := state.findObject(refs[0])
		if err != nil || object.OwnerSeat != choice.Seat || (object.Zone != ZoneGraveyard && object.Zone != ZoneVoid) {
			return fmt.Errorf("%w: baz-112 selection became stale", ErrInvalidCommand)
		}
		context.targets = []ObjectRef{object.Ref()}
		return context.moveTargetToDeck(true)
	case choiceHandlerBaz015Move:
		if len(selected) == 0 {
			return nil
		}
		refs, err := selectedObjectRefs(selected)
		if err != nil || len(refs) != 1 {
			return fmt.Errorf("%w: baz-015 selection is invalid", ErrInvalidCommand)
		}
		source, err := context.sourceObject()
		if err != nil {
			return err
		}
		object, err := state.findObject(refs[0])
		if err != nil || object.ControllerSeat != choice.Seat || object.Zone != ZoneBoard || object.Slot != SlotUnit || state.boardObject(choice.Seat, source.Lane, SlotUnit) != nil {
			return fmt.Errorf("%w: baz-015 selection became stale", ErrInvalidCommand)
		}
		object.Lane = source.Lane
		return nil
	case choiceHandlerBaz021Targets:
		refs, err := selectedObjectRefs(selected)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}
		context.targets = refs
		if err := context.suppressTargets(DurationUntilEndOfTurn); err != nil {
			return err
		}
		return context.modifyTargets(-3, DurationUntilEndOfTurn)
	case choiceHandlerBaz025Return:
		if len(selected) == 0 {
			return nil
		}
		refs, err := selectedObjectRefs(selected)
		if err != nil || len(refs) != 1 || context.source == nil || refs[0] != *context.source {
			return fmt.Errorf("%w: baz-025 selection is invalid", ErrInvalidCommand)
		}
		object, err := state.findObject(refs[0])
		if err != nil || object.Zone != ZoneGraveyard || object.OwnerSeat != choice.Seat {
			return fmt.Errorf("%w: baz-025 source became stale", ErrInvalidCommand)
		}
		moveObject(object, ZoneHand, len(state.objectsInZone(choice.Seat, ZoneHand)))
		return nil
	case choiceHandlerBaz047Shuffle:
		if len(selected) == 0 {
			return nil
		}
		refs, err := selectedObjectRefs(selected)
		if err != nil || len(refs) != 1 {
			return fmt.Errorf("%w: baz-047 selection is invalid", ErrInvalidCommand)
		}
		object, err := state.findObject(refs[0])
		if err != nil || object.OwnerSeat != choice.Seat || (object.Zone != ZoneGraveyard && object.Zone != ZoneVoid) {
			return fmt.Errorf("%w: baz-047 selection became stale", ErrInvalidCommand)
		}
		context.targets = []ObjectRef{object.Ref()}
		return context.shuffleTargetIntoDeck(choice.Seat)
	case choiceHandlerBaz059Exchange:
		if len(selected) == 0 {
			return nil
		}
		refs, err := selectedObjectRefs(selected)
		if err != nil || len(refs) != 1 || len(choice.Continuation.TargetRefs) != 1 {
			return fmt.Errorf("%w: baz-059 selection is invalid", ErrInvalidCommand)
		}
		enhanced, err := state.findObject(choice.Continuation.TargetRefs[0])
		if err != nil {
			return fmt.Errorf("%w: baz-059 enhanced unit became stale", ErrInvalidCommand)
		}
		other, err := state.findObject(refs[0])
		if err != nil || other.ControllerSeat != choice.Seat || other.Zone != ZoneBoard || other.Slot != SlotUnit || other.InstanceID == enhanced.InstanceID {
			return fmt.Errorf("%w: baz-059 target became stale", ErrInvalidCommand)
		}
		enhanced.Lane, other.Lane = other.Lane, enhanced.Lane
		return nil
	case choiceHandlerBaz065Shuffle:
		refs, err := selectedObjectRefs(selected)
		if err != nil {
			return err
		}
		context.targets = refs
		if len(refs) > 0 {
			if err := context.shuffleTargetsIntoDeck(choice.Seat); err != nil {
				return err
			}
		}
		return context.draw(choice.Seat, 2)
	case choiceHandlerBaz066Placement:
		if len(selected) == 0 {
			return context.draw(choice.Seat, 2)
		}
		if len(selected) != 1 || len(selected[0].Objects) != 1 || !selected[0].LaneSet || len(choice.Continuation.TargetRefs) != 1 ||
			selected[0].Objects[0] != choice.Continuation.TargetRefs[0] {
			return fmt.Errorf("%w: baz-066 placement is invalid", ErrInvalidCommand)
		}
		object, err := state.findObject(selected[0].Objects[0])
		if err != nil || object.OwnerSeat != choice.Seat || object.Zone != ZoneDeck ||
			state.boardObject(choice.Seat, selected[0].Lane, SlotUnit) != nil {
			return fmt.Errorf("%w: baz-066 revealed card or placement became stale", ErrInvalidCommand)
		}
		deck := state.objectsInZone(choice.Seat, ZoneDeck)
		if len(deck) == 0 || deck[0].Ref() != object.Ref() {
			return fmt.Errorf("%w: baz-066 revealed top card changed", ErrInvalidCommand)
		}
		context.targets = []ObjectRef{object.Ref()}
		context.command.DestinationLane = selected[0].Lane
		context.command.DestinationLaneSet = true
		return context.putTargetOnBoard(choice.Seat)
	case choiceHandlerBaz072Draw:
		if len(selected) == 0 {
			return nil
		}
		return context.draw(choice.Seat, 2)
	case choiceHandlerBaz088Target:
		if len(selected) == 0 {
			return nil
		}
		refs, err := selectedObjectRefs(selected)
		if err != nil {
			return err
		}
		context.targets = refs
		return context.modifyTargets(-10, DurationUntilEndOfTurn)
	case choiceHandlerBaz107Target:
		if len(selected) == 0 {
			return nil
		}
		refs, err := selectedObjectRefs(selected)
		if err != nil || len(refs) != 1 {
			return fmt.Errorf("%w: baz-107 selection is invalid", ErrInvalidCommand)
		}
		object, err := state.findObject(refs[0])
		if err != nil || object.Zone == ZoneDeck || object.Zone == ZoneHand || object.Zone == ZoneVoid {
			return fmt.Errorf("%w: baz-107 selection became stale or hidden", ErrInvalidCommand)
		}
		moveObject(object, ZoneVoid, len(state.objectsInZone(object.OwnerSeat, ZoneVoid)))
		pruneStaleEffectState(state)
		return nil
	case choiceHandlerPotionDeath:
		if len(selected) != 1 || len(selected[0].Objects) != 1 {
			return fmt.Errorf("%w: potion death choice requires one option", ErrInvalidCommand)
		}
		object, err := state.findObject(selected[0].Objects[0])
		if err != nil {
			return fmt.Errorf("%w: potion death target became stale", ErrInvalidCommand)
		}
		context.targets = []ObjectRef{object.Ref()}
		switch choice.Continuation.Step {
		case 16:
			if !selected[0].LaneSet || object.Zone != ZoneBoard || object.Lane == selected[0].Lane ||
				state.boardObject(object.ControllerSeat, selected[0].Lane, object.Slot) != nil {
				return fmt.Errorf("%w: baz-016 destination became unavailable", ErrInvalidCommand)
			}
			object.Lane = selected[0].Lane
			return nil
		case 34:
			definition, ok := budget.cards[object.DefinitionID]
			if !ok || object.OwnerSeat != choice.Seat || object.Zone != ZoneGraveyard || !cardHasType(definition, CardUnit) ||
				definition.Cost == nil || *definition.Cost > 4 || !selected[0].LaneSet ||
				state.boardObject(choice.Seat, selected[0].Lane, SlotUnit) != nil {
				return fmt.Errorf("%w: baz-034 target or destination became illegal", ErrInvalidCommand)
			}
			context.command.DestinationLane = selected[0].Lane
			context.command.DestinationLaneSet = true
			return context.putTargetOnBoard(choice.Seat)
		case 60:
			if object.Zone != ZoneBoard || object.Slot != SlotUnit {
				return fmt.Errorf("%w: baz-060 target became illegal", ErrInvalidCommand)
			}
			if err := context.modifyTargets(1, DurationUntilEndOfTurn); err != nil {
				return err
			}
			return context.grantKeyword(KeywordCharge, DurationUntilEndOfTurn)
		case 78:
			if object.Zone != ZoneBoard || object.Slot != SlotUnit {
				return fmt.Errorf("%w: baz-078 target became illegal", ErrInvalidCommand)
			}
			return context.modifyTargets(4, DurationUntilEndOfTurn)
		case 99:
			if object.Zone != ZoneBoard || object.Slot != SlotUnit {
				return fmt.Errorf("%w: baz-099 target became illegal", ErrInvalidCommand)
			}
			return context.grantKeyword(KeywordResilience, DurationUntilEndOfTurn)
		case 126:
			if object.ControllerSeat != choice.Seat.Other() || object.Zone != ZoneBoard || object.Slot != SlotUnit || object.Power > 2 ||
				!selected[0].LaneSet || state.boardObject(choice.Seat, selected[0].Lane, SlotUnit) != nil {
				return fmt.Errorf("%w: baz-126 target or destination became illegal", ErrInvalidCommand)
			}
			moveObject(object, ZoneBoard, 0)
			object.ControllerSeat = choice.Seat
			object.Lane = selected[0].Lane
			object.Slot = SlotUnit
			return nil
		}
		return fmt.Errorf("%w: unknown potion death step %d", ErrInvalidCommand, choice.Continuation.Step)
	default:
		return fmt.Errorf("%w: unknown closed choice continuation %q", ErrInvalidCommand, choice.Continuation.Handler)
	}
}

func selectedObjectRefs(selected []ChoiceOption) ([]ObjectRef, error) {
	result := make([]ObjectRef, 0, len(selected))
	for _, option := range selected {
		if len(option.Objects) != 1 {
			return nil, fmt.Errorf("%w: choice option has invalid object cardinality", ErrInvalidCommand)
		}
		result = append(result, option.Objects[0])
	}
	return result, nil
}

func objectRefs(objects []*CardObject) []ObjectRef {
	result := make([]ObjectRef, len(objects))
	for index, object := range objects {
		result[index] = object.Ref()
	}
	return result
}

func moveSelectedOwnedCard(state *State, choice PendingChoice, selected []ChoiceOption, source Zone, destination Zone) error {
	refs, err := selectedObjectRefs(selected)
	if err != nil || len(refs) != 1 {
		return fmt.Errorf("%w: choice requires one card", ErrInvalidCommand)
	}
	object, err := state.findObject(refs[0])
	if err != nil || object.OwnerSeat != choice.Seat || object.ControllerSeat != choice.Seat || object.Zone != source {
		return fmt.Errorf("%w: selected card is stale, foreign, or in the wrong zone", ErrInvalidCommand)
	}
	moveObject(object, destination, len(state.objectsInZone(object.OwnerSeat, destination)))
	pruneStaleEffectState(state)
	return nil
}

func moveSelectedControlledCard(state *State, choice PendingChoice, selected []ChoiceOption, source Zone, destination Zone) error {
	refs, err := selectedObjectRefs(selected)
	if err != nil || len(refs) != 1 {
		return fmt.Errorf("%w: choice requires one card", ErrInvalidCommand)
	}
	object, err := state.findObject(refs[0])
	if err != nil || object.ControllerSeat != choice.Seat || object.Zone != source {
		return fmt.Errorf("%w: selected card is stale, foreign, or in the wrong zone", ErrInvalidCommand)
	}
	moveObject(object, destination, len(state.objectsInZone(object.OwnerSeat, destination)))
	pruneStaleEffectState(state)
	return nil
}

func hideDeckCardsOnBottom(state *State, seat Seat, ordered []ObjectRef) error {
	selected := make(map[string]*CardObject, len(ordered))
	for _, ref := range ordered {
		object, err := state.findObject(ref)
		if err != nil || object.OwnerSeat != seat || object.Zone != ZoneDeck {
			return fmt.Errorf("%w: ordered deck card is stale", ErrInvalidCommand)
		}
		selected[object.InstanceID] = object
	}
	others := make([]*CardObject, 0)
	for _, object := range state.objectsInZone(seat, ZoneDeck) {
		if _, ok := selected[object.InstanceID]; !ok {
			others = append(others, object)
		}
	}
	for index, object := range others {
		object.ZoneIndex = index
	}
	for index, ref := range ordered {
		object, _ := state.findObject(ref)
		moveObject(object, ZoneDeck, len(others)+index)
	}
	return nil
}
