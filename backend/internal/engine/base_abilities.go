package engine

import (
	"fmt"
	"strconv"
	"strings"
)

type builtinAbilityContext struct {
	state   *State
	command Command
	source  *ObjectRef
	targets []ObjectRef
	budget  *eventBudget
}

type builtinAbilitySpec struct {
	sourceRequired bool
	sourceZones    []Zone
	consumesAction bool
	payment        PaymentDefinition
}

var noAbilityCardNumbers = map[int]struct{}{
	7: {}, 26: {}, 53: {}, 69: {}, 92: {}, 116: {},
}

func builtinAbility(handler string) (builtinAbilitySpec, bool) {
	number, ok := builtinCardNumber(handler)
	if !ok {
		return builtinAbilitySpec{}, false
	}
	spec := builtinAbilitySpec{}
	if builtinNeedsBoardSource(number) {
		spec.sourceRequired = true
		spec.sourceZones = []Zone{ZoneBoard}
	}
	if builtinConsumesAction(number) {
		spec.consumesAction = true
	}
	switch number {
	case 27:
		spec.payment = PaymentDefinition{SacrificeCount: 1, SacrificeZones: []Zone{ZoneHand}}
	case 45:
		spec.payment = PaymentDefinition{SacrificeCount: 1, SacrificeZones: []Zone{ZoneBoard}}
	case 57:
		spec.payment = PaymentDefinition{SacrificeCount: 1, SacrificeZones: []Zone{ZoneBoard}}
	case 119:
		spec.payment = PaymentDefinition{SacrificeCount: 1, SacrificeZones: []Zone{ZoneBoard}}
	}
	return spec, true
}

func builtinCardNumber(handler string) (int, bool) {
	parts := strings.Split(handler, ".")
	if len(parts) != 3 || parts[2] != "ability" || !strings.HasPrefix(parts[0], "baz-") {
		return 0, false
	}
	if parts[1] != "front" && !strings.HasPrefix(parts[1], "level-") {
		return 0, false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(parts[0], "baz-"))
	if err != nil || number < 1 || number > 132 {
		return 0, false
	}
	if _, missing := noAbilityCardNumbers[number]; missing {
		return 0, false
	}
	return number, true
}

func builtinNeedsBoardSource(number int) bool {
	switch number {
	case 1, 2, 5, 6, 15, 16, 27, 45, 46, 48, 59, 60, 70, 73, 75, 76, 77, 78,
		80, 89, 90, 94, 97, 98, 99, 100, 101, 111, 114, 117, 118, 119, 120,
		123, 124, 125, 126, 129, 130:
		return true
	default:
		return false
	}
}

func builtinConsumesAction(number int) bool {
	// Triggered and continuous abilities are resolved by the event queue. All
	// other handlers represent either playing a card or using an Action ability.
	switch number {
	case 3, 5, 6, 8, 10, 15, 19, 24, 25, 30, 31, 32, 37, 39, 41, 46, 47, 49,
		50, 51, 52, 54, 59, 63, 68, 71, 72, 73, 74, 77, 85, 87, 90, 91, 93, 95,
		96, 97, 98, 100, 101, 106, 108, 112, 113, 114, 115, 117, 118, 120, 123,
		124, 125, 129, 130:
		return false
	default:
		return true
	}
}

func executeBuiltinAbility(
	state *State,
	command Command,
	source *ObjectRef,
	targets []ObjectRef,
	budget *eventBudget,
) error {
	number, ok := builtinCardNumber(command.AbilityID)
	if !ok {
		return fmt.Errorf("unsupported closed handler %q", command.AbilityID)
	}
	context := &builtinAbilityContext{state: state, command: command, source: source, targets: targets, budget: budget}
	if len(command.TargetRefs) > 0 && len(targets) == 0 {
		seat := command.Seat
		return emitEffectEvent(state, budget, "card_ability_fizzled", &seat, "Magic Shield payment declined")
	}
	if err := context.execute(number); err != nil {
		return err
	}
	seat := command.Seat
	return emitEffectEvent(state, budget, "card_ability_resolved", &seat, command.AbilityID)
}

func (context *builtinAbilityContext) execute(number int) error {
	switch number {
	case 1, 9, 11:
		if number == 9 {
			return context.requestBaz009Choice()
		}
		return context.moveBoardTarget(number == 1)
	case 2:
		return context.moveSourceAsRider()
	case 3:
		return context.gainInfluence(1)
	case 4:
		return context.analyzeOpponentTop()
	case 5:
		return context.modifySourcePower(context.sourcePower())
	case 6, 94, 98:
		return context.modifyTargets(1, DurationUntilEndOfTurn)
	case 41:
		return context.grantEnhancedKeyword(KeywordBypass)
	case 8, 52, 95:
		return context.grantKeyword(KeywordBypass, DurationWhileSourceValid)
	case 39, 85:
		return context.grantEnhancedKeyword(KeywordPoison)
	case 10, 24:
		return context.grantKeyword(KeywordPoison, DurationWhileSourceValid)
	case 12, 22:
		if err := context.moveTargetToDeck(false); err != nil {
			return err
		}
		if number == 12 {
			return context.draw(context.command.Seat, 1)
		}
		return nil
	case 13:
		if err := context.moveTargetToDeck(false); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 1)
	case 14:
		if err := context.modifyTargets(2, DurationUntilEndOfTurn); err != nil {
			return err
		}
		return context.grantKeyword(KeywordCharge, DurationUntilEndOfTurn)
	case 15:
		return context.requestBaz015Move()
	case 16:
		return context.requestPotionDeathChoice(16)
	case 17:
		target, err := context.target(0)
		if err != nil {
			return err
		}
		return context.modifyTargets(-len(context.state.objectsInZone(target.ControllerSeat, ZoneHand)), DurationUntilEndOfTurn)
	case 18:
		return context.interceptMemory()
	case 19, 101:
		return context.grantEnhancedKeyword(KeywordRangedAttack)
	case 51:
		return context.grantKeyword(KeywordRangedAttack, DurationWhileSourceValid)
	case 20:
		return context.requestBaz020Choice()
	case 21:
		return context.requestBaz021Choice()
	case 23:
		if len(context.state.objectsInZone(context.command.Seat, ZoneDeck)) == 0 {
			return fmt.Errorf("%w: Cassius cannot act with an empty deck", ErrInvalidCommand)
		}
		return context.drawThenRequestDiscard(1)
	case 25:
		return context.requestBaz025Choice()
	case 27:
		return context.gainInfluence(1)
	case 28:
		return context.playTargetForFree()
	case 29:
		return context.requestOwnDiscard(choiceHandlerBaz029Discard)
	case 63:
		return context.grantEnhancedKeyword(KeywordCharge)
	case 30, 68, 74:
		return context.grantKeyword(KeywordCharge, DurationWhileSourceValid)
	case 31:
		return context.modifyTargets(-1, DurationUntilEndOfTurn)
	case 129:
		return context.grantEnhancedKeyword(KeywordResilience)
	case 32:
		return context.grantKeyword(KeywordResilience, DurationWhileSourceValid)
	case 33:
		return context.modifyTargets(-context.unitCount(context.command.Seat, ZoneGraveyard), DurationUntilEndOfTurn)
	case 34:
		return context.requestPotionDeathChoice(34)
	case 35:
		if err := context.destroyTarget(); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 1)
	case 36:
		return context.requestBaz036Choice()
	case 37:
		if err := context.modifyEnhancedUnit(1, DurationWhileSourceValid); err != nil {
			return err
		}
		return context.addShieldToEnhancedUnit(1)
	case 38:
		return context.requestBaz038Choice()
	case 40:
		return context.destroyTarget()
	case 42:
		return context.putTargetOnBoard(context.command.Seat)
	case 43:
		return context.drawThenRequestDiscard(3)
	case 44:
		return context.requestBaz044Choice()
	case 45:
		return context.modifyTargets(-3, DurationUntilEndOfTurn)
	case 46:
		return context.modifySourcePower(1)
	case 47:
		return context.requestBaz047Choice()
	case 48:
		return context.modifyTargets(-context.sourcePower(), DurationUntilEndOfTurn)
	case 49:
		return context.gainInfluence(2)
	case 50, 55:
		return context.requestHighestCostSacrifice()
	case 54:
		context.state.shuffleDeck(context.command.Seat)
		return context.draw(context.command.Seat, 1)
	case 56:
		if err := context.modifyTargets(-1, DurationUntilEndOfTurn); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 1)
	case 57:
		return context.returnRandomOtherGraveyardCard()
	case 58:
		return context.modifyTargets(3, DurationUntilEndOfTurn)
	case 59:
		return context.requestBaz059Choice()
	case 60:
		return context.requestPotionDeathChoice(60)
	case 61:
		return context.randomHandToDeck(context.command.Seat.Other())
	case 62:
		if err := context.draw(context.command.Seat, 1); err != nil {
			return err
		}
		return context.modifyTargets(-len(context.state.objectsInZone(context.command.Seat, ZoneHand)), DurationUntilEndOfTurn)
	case 64:
		if err := context.randomBoardToDeck(context.command.Seat.Other()); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 1)
	case 65:
		return context.requestBaz065Choice()
	case 66:
		return context.requestBaz066Choice()
	case 67:
		return context.gainInfluence(2)
	case 70:
		return context.modifyTargets(-1, DurationUntilEndOfTurn)
	case 71:
		player := context.state.player(context.command.Seat.Other())
		if player.Influence > 0 {
			player.Influence--
		}
		return nil
	case 72:
		return context.requestBaz072Choice()
	case 73:
		return context.recordContinuous("action-card-discount")
	case 75:
		return context.draw(context.command.Seat, 1)
	case 76:
		return context.payXAndModify()
	case 77:
		return context.addShieldToEnhancedUnit(1)
	case 78:
		return context.requestPotionDeathChoice(78)
	case 79:
		return context.modifyTargets(-context.maxBoardUnitPower(context.command.Seat), DurationUntilEndOfTurn)
	case 80:
		return context.gainInfluence(1 + context.revealedOrders(context.command.Seat))
	case 81:
		if err := context.addTraitModifier(func(modifier *ModifierState) { modifier.ExtraActionUses = 1 }, DurationUntilEndOfTurn); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 1)
	case 82:
		if err := context.moveTargetTo(ZoneHand); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 1)
	case 83:
		return context.addTraitModifier(func(modifier *ModifierState) { modifier.SkipCombat = true }, DurationUntilEndOfTurn)
	case 84:
		if err := context.moveTargetTo(ZoneHand); err != nil {
			return err
		}
		return context.recordContinuous("next-turn-name-lock")
	case 86:
		if err := context.draw(context.command.Seat, 1); err != nil {
			return err
		}
		return context.playTargetForFree()
	case 87:
		return context.recordContinuous("next-turn-no-hero-attacks")
	case 88:
		return context.requestBaz088Choice()
	case 89:
		return context.grantKeyword(KeywordCharge, DurationUntilEndOfTurn)
	case 90:
		return context.addTraitModifier(func(modifier *ModifierState) { modifier.DamageReduction = 1 }, DurationWhileSourceValid)
	case 91:
		return context.requestOpponentHandMove(choiceHandlerBaz091Void, "opponent_sends_hand_card_to_void")
	case 93:
		target, err := context.target(0)
		if err != nil {
			return err
		}
		power := target.Power
		if err := context.moveTargetTo(ZoneVoid); err != nil {
			return err
		}
		return context.modifySourcePower(power)
	case 96, 97:
		return context.modifyAllBoardUnits(context.command.Seat, 1, DurationUntilEndOfTurn)
	case 99:
		return context.requestPotionDeathChoice(99)
	case 100:
		return context.modifyEnhancedUnit(3, DurationWhileSourceValid)
	case 102:
		return context.requestBaz102Choice()
	case 103:
		if err := context.moveTargetTo(ZoneVoid); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 1)
	case 104:
		value := 1
		return context.addTraitModifier(func(modifier *ModifierState) { modifier.BasePowerSet = &value }, DurationUntilEndOfTurn)
	case 105:
		return context.requestOpponentHandMove(choiceHandlerBaz105Void, "opponent_sends_hand_card_to_void")
	case 106:
		return context.modifyAllBoardUnits(context.command.Seat, 2, DurationUntilEndOfTurn)
	case 107:
		return context.requestBaz107Choice()
	case 108:
		return context.addShieldToEnhancedUnit(3)
	case 109:
		return context.putTargetOnBoard(context.command.Seat)
	case 110:
		return context.burningFire()
	case 111:
		return context.bjornBoost()
	case 112:
		return context.requestBaz112Choice()
	case 113:
		return context.draw(context.command.Seat, 1)
	case 114:
		return nil
	case 115:
		return context.mill(context.command.Seat.Other(), 2)
	case 117:
		return context.grantKeyword(KeywordPoison, DurationUntilEndOfTurn)
	case 118:
		return context.grantKeyword(KeywordCharge, DurationUntilEndOfTurn)
	case 119:
		return context.modifyTargets(-2, DurationUntilEndOfTurn)
	case 120:
		return nil
	case 121:
		return context.requestBaz121Choice()
	case 122:
		if err := context.moveTargetToDeck(true); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 1)
	case 123:
		value := 1
		return context.addEnhancedTraitModifier(func(modifier *ModifierState) {
			modifier.BasePowerSet = &value
			modifier.CannotIncreasePower = true
		}, DurationWhileSourceValid)
	case 124:
		return context.modifyEnhancedUnit(2, DurationWhileSourceValid)
	case 125:
		return context.modifyEnhancedUnit(1, DurationWhileSourceValid)
	case 126:
		return context.requestPotionDeathChoice(126)
	case 127:
		return context.requestBaz127Move()
	case 128:
		return context.modifyAllBoardUnits(context.command.Seat.Other(), -2, DurationUntilEndOfTurn)
	case 130:
		return context.modifyAllEnhancedUnits(1)
	case 131:
		if err := context.mill(context.command.Seat.Other(), 2); err != nil {
			return err
		}
		return context.draw(context.command.Seat, 2)
	case 132:
		return context.moveTargetToDeck(true)
	default:
		return fmt.Errorf("closed handler baz-%03d has no executable semantics", number)
	}
}

func (context *builtinAbilityContext) target(index int) (*CardObject, error) {
	if index < 0 || index >= len(context.targets) {
		return nil, fmt.Errorf("%w: ability requires target %d", ErrInvalidCommand, index)
	}
	object, err := context.state.findObject(context.targets[index])
	if err != nil {
		return nil, fmt.Errorf("%w: ability target is stale", ErrInvalidCommand)
	}
	return object, nil
}

func (context *builtinAbilityContext) sourceObject() (*CardObject, error) {
	if context.source == nil {
		return nil, fmt.Errorf("%w: ability source is required", ErrInvalidCommand)
	}
	object, err := context.state.findObject(*context.source)
	if err != nil {
		return nil, fmt.Errorf("%w: ability source is stale", ErrInvalidCommand)
	}
	return object, nil
}

func (context *builtinAbilityContext) sourcePower() int {
	object, err := context.sourceObject()
	if err != nil {
		return 0
	}
	return object.Power
}

func (context *builtinAbilityContext) card(object *CardObject) (CardDefinition, error) {
	definition, exists := context.budget.cards[object.DefinitionID]
	if !exists {
		return CardDefinition{}, fmt.Errorf("%w: card %q is not pinned", ErrInvalidCommand, object.DefinitionID)
	}
	return definition, nil
}

func (context *builtinAbilityContext) requireTargetCount(minimum, maximum int) error {
	if len(context.targets) < minimum || len(context.targets) > maximum {
		return fmt.Errorf("%w: expected %d..%d targets", ErrInvalidCommand, minimum, maximum)
	}
	return nil
}

func (context *builtinAbilityContext) draw(seat Seat, count int) error {
	for range count {
		drawn, err := context.state.draw(seat)
		if err != nil {
			return err
		}
		if !drawn {
			break
		}
		seatCopy := seat
		if err := emitEffectEvent(context.state, context.budget, "card_drawn", &seatCopy, "ability"); err != nil {
			return err
		}
	}
	return nil
}

func (context *builtinAbilityContext) gainInfluence(amount int) error {
	player := context.state.player(context.command.Seat)
	player.Influence += amount
	return nil
}

func (context *builtinAbilityContext) addModifier(target ObjectRef, duration DurationKind, configure func(*ModifierState)) error {
	context.state.NextEffectSequence++
	modifier := ModifierState{
		ModifierID:      fmt.Sprintf("modifier-%d", context.state.NextEffectSequence),
		CreatedSequence: context.state.NextEffectSequence,
		SourceRef:       cloneObjectRef(context.source),
		TargetRef:       target,
		Layer:           20,
		Duration:        duration,
		CreatedTurn:     context.state.TurnNumber,
	}
	configure(&modifier)
	context.state.Modifiers = append(context.state.Modifiers, modifier)
	recalculatePower(context.state, target)
	return nil
}

func (context *builtinAbilityContext) addTraitModifier(configure func(*ModifierState), duration DurationKind) error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	target, err := context.target(0)
	if err != nil {
		return err
	}
	return context.addModifier(target.Ref(), duration, configure)
}

func (context *builtinAbilityContext) modifyTargets(amount int, duration DurationKind) error {
	if err := context.requireTargetCount(1, 8); err != nil {
		return err
	}
	for _, ref := range context.targets {
		object, err := context.state.findObject(ref)
		if err != nil || object.Zone != ZoneBoard || object.Slot != SlotUnit {
			return fmt.Errorf("%w: power target must be a board unit", ErrInvalidCommand)
		}
		if err := context.addModifier(object.Ref(), duration, func(modifier *ModifierState) { modifier.PowerDelta = amount }); err != nil {
			return err
		}
	}
	return nil
}

func (context *builtinAbilityContext) modifySourcePower(amount int) error {
	object, err := context.sourceObject()
	if err != nil {
		return err
	}
	return context.addModifier(object.Ref(), DurationUntilEndOfTurn, func(modifier *ModifierState) { modifier.PowerDelta = amount })
}

func (context *builtinAbilityContext) grantKeyword(keyword Keyword, duration DurationKind) error {
	if len(context.targets) == 0 {
		object, err := context.sourceObject()
		if err != nil {
			return err
		}
		return context.addModifier(object.Ref(), duration, func(modifier *ModifierState) { modifier.Keyword = keyword })
	}
	return context.addTraitModifier(func(modifier *ModifierState) { modifier.Keyword = keyword }, duration)
}

func (context *builtinAbilityContext) suppressTargets(duration DurationKind) error {
	for _, ref := range context.targets {
		if err := context.addModifier(ref, duration, func(modifier *ModifierState) { modifier.SuppressBaseAbilities = true }); err != nil {
			return err
		}
	}
	return nil
}

func (context *builtinAbilityContext) moveTargetTo(zone Zone) error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	object, err := context.target(0)
	if err != nil {
		return err
	}
	index := len(context.state.objectsInZone(object.OwnerSeat, zone))
	if zone == ZoneHand {
		index = len(context.state.objectsInZone(object.OwnerSeat, ZoneHand))
	}
	moveObject(object, zone, index)
	pruneStaleEffectState(context.state)
	return nil
}

func (context *builtinAbilityContext) moveSourceTo(zone Zone) error {
	object, err := context.sourceObject()
	if err != nil {
		return err
	}
	moveObject(object, zone, len(context.state.objectsInZone(object.OwnerSeat, zone)))
	pruneStaleEffectState(context.state)
	return nil
}

func (context *builtinAbilityContext) destroyTarget() error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	object, err := context.target(0)
	if err != nil || object.Zone != ZoneBoard {
		return fmt.Errorf("%w: destroy target must be on the board", ErrInvalidCommand)
	}
	moveToGraveyard(context.state, object)
	pruneStaleEffectState(context.state)
	return nil
}

func (context *builtinAbilityContext) discardTarget(seat Seat) error {
	object, err := context.target(0)
	if err != nil || object.Zone != ZoneHand || object.OwnerSeat != seat {
		return fmt.Errorf("%w: discard target must be in the required hand", ErrInvalidCommand)
	}
	moveToGraveyard(context.state, object)
	return nil
}

func (context *builtinAbilityContext) unitCount(seat Seat, zone Zone) int {
	count := 0
	for _, object := range context.state.objectsInZone(seat, zone) {
		definition, ok := context.budget.cards[object.DefinitionID]
		if ok && cardHasType(definition, CardUnit) {
			count++
		}
	}
	return count
}
