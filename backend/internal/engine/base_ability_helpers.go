package engine

import (
	"fmt"
	"sort"
)

func applyMagicShieldPayments(state *State, command Command, targets []ObjectRef) ([]ObjectRef, error) {
	paid := make(map[ObjectRef]struct{}, len(command.ShieldPaymentRefs))
	for _, ref := range command.ShieldPaymentRefs {
		if _, duplicate := paid[ref]; duplicate {
			return nil, fmt.Errorf("%w: duplicate Magic Shield payment", ErrInvalidCommand)
		}
		paid[ref] = struct{}{}
	}
	result := make([]ObjectRef, 0, len(targets))
	used := make(map[ObjectRef]struct{}, len(paid))
	total := 0
	for _, ref := range targets {
		object, err := state.findObject(ref)
		if err != nil {
			return nil, fmt.Errorf("%w: Magic Shield target is stale", ErrInvalidCommand)
		}
		cost := 0
		if object.Zone == ZoneBoard && object.ControllerSeat != command.Seat {
			cost = object.BaseMagicShieldCost + modifierInt(state, ref, func(modifier ModifierState) int { return modifier.MagicShieldCost })
		}
		if cost == 0 {
			result = append(result, ref)
			continue
		}
		if _, accepted := paid[ref]; !accepted {
			continue
		}
		used[ref] = struct{}{}
		total += cost
		result = append(result, ref)
	}
	if len(used) != len(paid) {
		return nil, fmt.Errorf("%w: Magic Shield payment references an unprotected target", ErrInvalidCommand)
	}
	player := state.player(command.Seat)
	if player.Influence < total {
		return nil, fmt.Errorf("%w: insufficient influence for Magic Shield", ErrInvalidCommand)
	}
	player.Influence -= total
	return result, nil
}

func (context *builtinAbilityContext) moveBoardTarget(adjacentOnly bool) error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	if !context.command.DestinationLaneSet || context.command.DestinationLane < 0 || context.command.DestinationLane >= LaneCount {
		return fmt.Errorf("%w: a free destination lane is required", ErrInvalidCommand)
	}
	object, err := context.target(0)
	if err != nil || object.Zone != ZoneBoard || (object.Slot != SlotUnit && object.Slot != SlotSupport) {
		return fmt.Errorf("%w: movement target must be a board card", ErrInvalidCommand)
	}
	if adjacentOnly && distance(object.Lane, context.command.DestinationLane) != 1 {
		return fmt.Errorf("%w: destination must be immediately left or right", ErrInvalidCommand)
	}
	if context.state.boardObject(object.ControllerSeat, context.command.DestinationLane, object.Slot) != nil {
		return fmt.Errorf("%w: destination cell is occupied", ErrInvalidCommand)
	}
	object.Lane = context.command.DestinationLane
	return nil
}

func (context *builtinAbilityContext) moveSourceAsRider() error {
	object, err := context.sourceObject()
	if err != nil {
		return err
	}
	context.targets = []ObjectRef{object.Ref()}
	return context.moveBoardTarget(false)
}

func (context *builtinAbilityContext) moveTargetToSourceLane() error {
	source, err := context.sourceObject()
	if err != nil {
		return err
	}
	target, err := context.target(0)
	if err != nil || target.Zone != ZoneBoard || target.Slot != SlotUnit {
		return fmt.Errorf("%w: target must be a board unit", ErrInvalidCommand)
	}
	if context.state.boardObject(context.command.Seat, source.Lane, SlotUnit) != nil {
		return fmt.Errorf("%w: unit cell in front of the enhancement is occupied", ErrInvalidCommand)
	}
	target.ControllerSeat = context.command.Seat
	target.Lane = source.Lane
	return nil
}

func (context *builtinAbilityContext) moveTargetToDeck(top bool) error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	object, err := context.target(0)
	if err != nil {
		return err
	}
	deck := context.state.objectsInZone(object.OwnerSeat, ZoneDeck)
	if top {
		for _, card := range deck {
			card.ZoneIndex++
		}
		moveObject(object, ZoneDeck, 0)
	} else {
		moveObject(object, ZoneDeck, len(deck))
	}
	pruneStaleEffectState(context.state)
	return nil
}

func (context *builtinAbilityContext) shuffleTargetIntoDeck(seat Seat) error {
	if err := context.moveTargetToDeck(false); err != nil {
		return err
	}
	context.state.shuffleDeck(seat)
	return nil
}

func (context *builtinAbilityContext) shuffleTargetsIntoDeck(seat Seat) error {
	if err := context.requireTargetCount(0, 2); err != nil {
		return err
	}
	for len(context.targets) > 0 {
		first := context.targets[0]
		context.targets = context.targets[1:]
		object, err := context.state.findObject(first)
		if err != nil || object.OwnerSeat != seat || (object.Zone != ZoneGraveyard && object.Zone != ZoneVoid) {
			return fmt.Errorf("%w: shuffle target must be in the controller's graveyard or void", ErrInvalidCommand)
		}
		moveObject(object, ZoneDeck, len(context.state.objectsInZone(seat, ZoneDeck)))
	}
	context.state.shuffleDeck(seat)
	return nil
}

func (context *builtinAbilityContext) putTargetOnBoard(controller Seat) error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	if !context.command.DestinationLaneSet || context.command.DestinationLane < 0 || context.command.DestinationLane >= LaneCount {
		return fmt.Errorf("%w: destination lane is required", ErrInvalidCommand)
	}
	object, err := context.target(0)
	if err != nil {
		return err
	}
	definition, err := context.card(object)
	if err != nil || !cardHasType(definition, CardUnit) {
		return fmt.Errorf("%w: only a unit can be placed in the unit cell", ErrInvalidCommand)
	}
	if context.state.boardObject(controller, context.command.DestinationLane, SlotUnit) != nil {
		return fmt.Errorf("%w: destination unit cell is occupied", ErrInvalidCommand)
	}
	moveObject(object, ZoneBoard, 0)
	object.ControllerSeat = controller
	object.Lane = context.command.DestinationLane
	object.Slot = SlotUnit
	if definition.BasePower != nil {
		object.BasePower = *definition.BasePower
		object.Power = *definition.BasePower
	}
	object.Keywords = append([]Keyword(nil), definition.NativeKeywords...)
	object.BaseMagicShieldCost = definition.BaseMagicShieldCost
	return nil
}

func (context *builtinAbilityContext) playTargetForFree() error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	object, err := context.target(0)
	if err != nil {
		return err
	}
	definition, err := context.card(object)
	if err != nil {
		return err
	}
	if definition.Cost != nil && *definition.Cost > 3 {
		return fmt.Errorf("%w: free-play target has base cost above three", ErrInvalidCommand)
	}
	if cardHasType(definition, CardUnit) {
		return context.putTargetOnBoard(context.command.Seat)
	}
	if cardHasType(definition, CardEnhancement) {
		if !context.command.DestinationLaneSet || context.state.boardObject(context.command.Seat, context.command.DestinationLane, SlotSupport) != nil {
			return fmt.Errorf("%w: a free support cell is required", ErrInvalidCommand)
		}
		moveObject(object, ZoneBoard, 0)
		object.ControllerSeat = context.command.Seat
		object.Lane = context.command.DestinationLane
		object.Slot = SlotSupport
		return nil
	}
	moveToGraveyard(context.state, object)
	return nil
}

func (context *builtinAbilityContext) analyzeOpponentTop() error {
	deck := context.state.objectsInZone(context.command.Seat.Other(), ZoneDeck)
	if len(deck) == 0 {
		return context.modifySourcePower(2)
	}
	definition, exists := context.budget.cards[deck[0].DefinitionID]
	if !exists || definition.Cost == nil || *definition.Cost > 3 || len(context.targets) == 0 {
		return context.modifySourcePower(2)
	}
	if context.targets[0] != deck[0].Ref() {
		return fmt.Errorf("%w: free-play target must be the revealed top card", ErrInvalidCommand)
	}
	return context.playTargetForFree()
}

func (context *builtinAbilityContext) interceptMemory() error {
	deck := context.state.objectsInZone(context.command.Seat.Other(), ZoneDeck)
	limit := minInt(5, len(deck))
	var selected *CardObject
	if len(context.targets) > 0 {
		candidate, err := context.target(0)
		if err != nil {
			return err
		}
		for _, object := range deck[:limit] {
			if object.Ref() == candidate.Ref() {
				selected = candidate
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("%w: selected card is not among the revealed five", ErrInvalidCommand)
		}
		definition, err := context.card(selected)
		if err != nil || definition.Cost == nil || *definition.Cost > 3 {
			return fmt.Errorf("%w: selected card cannot be played for free", ErrInvalidCommand)
		}
		context.targets = []ObjectRef{selected.Ref()}
		if err := context.playTargetForFree(); err != nil {
			return err
		}
	}
	remaining := make([]*CardObject, 0, limit)
	for _, object := range deck[:limit] {
		if selected == nil || object.InstanceID != selected.InstanceID {
			remaining = append(remaining, object)
		}
	}
	context.state.randomizeBottom(remaining, context.command.Seat.Other())
	return nil
}

func (context *builtinAbilityContext) pickTopCards(
	seat Seat,
	look int,
	pick int,
	pickedZone Zone,
	restZone Zone,
	randomRest bool,
) error {
	deck := context.state.objectsInZone(seat, ZoneDeck)
	look = minInt(look, len(deck))
	if err := context.requireTargetCount(pick, pick); err != nil {
		return err
	}
	selected := make(map[string]struct{}, pick)
	for _, ref := range context.targets {
		found := false
		for _, object := range deck[:look] {
			if object.Ref() == ref {
				selected[object.InstanceID] = struct{}{}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: selected card is outside the looked-at prefix", ErrInvalidCommand)
		}
	}
	rest := make([]*CardObject, 0, look-pick)
	for _, object := range deck[:look] {
		if _, ok := selected[object.InstanceID]; ok {
			moveObject(object, pickedZone, len(context.state.objectsInZone(seat, pickedZone)))
		} else {
			rest = append(rest, object)
		}
	}
	if restZone == ZoneDeck {
		if randomRest {
			context.state.randomizeBottom(rest, seat)
		} else {
			for _, object := range rest {
				moveObject(object, ZoneDeck, len(context.state.objectsInZone(seat, ZoneDeck)))
			}
		}
	} else {
		for _, object := range rest {
			moveObject(object, restZone, len(context.state.objectsInZone(seat, restZone)))
		}
	}
	return nil
}

func (context *builtinAbilityContext) searchTopForUnit(look int) error {
	deck := context.state.objectsInZone(context.command.Seat, ZoneDeck)
	look = minInt(look, len(deck))
	if err := context.requireTargetCount(0, 1); err != nil {
		return err
	}
	var selected *CardObject
	if len(context.targets) == 1 {
		candidate, err := context.target(0)
		if err != nil {
			return err
		}
		definition, err := context.card(candidate)
		if err != nil || !cardHasType(definition, CardUnit) {
			return fmt.Errorf("%w: selected search result must be a unit", ErrInvalidCommand)
		}
		for _, object := range deck[:look] {
			if object.Ref() == candidate.Ref() {
				selected = candidate
			}
		}
		if selected == nil {
			return fmt.Errorf("%w: selected unit is not in the top four", ErrInvalidCommand)
		}
		moveObject(selected, ZoneHand, len(context.state.objectsInZone(context.command.Seat, ZoneHand)))
	}
	rest := make([]*CardObject, 0, look)
	for _, object := range deck[:look] {
		if selected == nil || object.InstanceID != selected.InstanceID {
			rest = append(rest, object)
		}
	}
	context.state.randomizeBottom(rest, context.command.Seat)
	return nil
}

func (context *builtinAbilityContext) potion(power int, keyword Keyword, drawAndSacrifice bool) error {
	if drawAndSacrifice && context.source != nil {
		if err := context.draw(context.command.Seat, 1); err != nil {
			return err
		}
		if err := context.moveSourceTo(ZoneGraveyard); err != nil {
			return err
		}
	}
	if len(context.targets) == 0 {
		return nil
	}
	if power != 0 {
		if err := context.modifyTargets(power, DurationUntilEndOfTurn); err != nil {
			return err
		}
	}
	if keyword != "" {
		return context.grantKeyword(keyword, DurationUntilEndOfTurn)
	}
	return nil
}

func (context *builtinAbilityContext) enhancedUnit() (*CardObject, error) {
	source, err := context.sourceObject()
	if err != nil || source.Zone != ZoneBoard || source.Slot != SlotSupport {
		return nil, fmt.Errorf("%w: ability source must be a board enhancement", ErrInvalidCommand)
	}
	target := context.state.boardObject(source.ControllerSeat, source.Lane, SlotUnit)
	if target == nil {
		return nil, fmt.Errorf("%w: enhancement has no unit in front of it", ErrInvalidCommand)
	}
	return target, nil
}

func (context *builtinAbilityContext) modifyEnhancedUnit(amount int, duration DurationKind) error {
	target, err := context.enhancedUnit()
	if err != nil {
		return err
	}
	return context.addModifier(target.Ref(), duration, func(modifier *ModifierState) { modifier.PowerDelta = amount })
}

func (context *builtinAbilityContext) modifyAllEnhancedUnits(amount int) error {
	source, err := context.sourceObject()
	if err != nil || source.Zone != ZoneBoard || source.Slot != SlotSupport {
		return fmt.Errorf("%w: ability source must be a board enhancement", ErrInvalidCommand)
	}
	for lane := 0; lane < LaneCount; lane++ {
		unit := context.state.boardObject(context.command.Seat, lane, SlotUnit)
		support := context.state.boardObject(context.command.Seat, lane, SlotSupport)
		if unit == nil || support == nil {
			continue
		}
		if err := context.addModifier(unit.Ref(), DurationWhileSourceValid, func(modifier *ModifierState) {
			modifier.PowerDelta = amount
		}); err != nil {
			return err
		}
	}
	return nil
}

func (context *builtinAbilityContext) addShieldToEnhancedUnit(cost int) error {
	target, err := context.enhancedUnit()
	if err != nil {
		return err
	}
	return context.addModifier(target.Ref(), DurationWhileSourceValid, func(modifier *ModifierState) { modifier.MagicShieldCost = cost })
}

func (context *builtinAbilityContext) opponentDiscardOrSacrifice() error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	object, err := context.target(0)
	if err != nil || object.ControllerSeat != context.command.Seat.Other() {
		return fmt.Errorf("%w: opponent must choose their own card", ErrInvalidCommand)
	}
	if object.Zone == ZoneHand {
		moveToGraveyard(context.state, object)
		return nil
	}
	if object.Zone == ZoneBoard {
		moveToGraveyard(context.state, object)
		return nil
	}
	return fmt.Errorf("%w: choice must be a hand discard or board sacrifice", ErrInvalidCommand)
}

func (context *builtinAbilityContext) opponentDiscardAndSacrifice() error {
	if err := context.requireTargetCount(0, 2); err != nil {
		return err
	}
	handSeen, boardSeen := false, false
	for _, ref := range context.targets {
		object, err := context.state.findObject(ref)
		if err != nil || object.ControllerSeat != context.command.Seat.Other() {
			return fmt.Errorf("%w: selected card is not controlled by the opponent", ErrInvalidCommand)
		}
		switch object.Zone {
		case ZoneHand:
			if handSeen {
				return fmt.Errorf("%w: duplicate discard selection", ErrInvalidCommand)
			}
			handSeen = true
		case ZoneBoard:
			if boardSeen {
				return fmt.Errorf("%w: duplicate sacrifice selection", ErrInvalidCommand)
			}
			boardSeen = true
		default:
			return fmt.Errorf("%w: selected card is not discardable or sacrificable", ErrInvalidCommand)
		}
		moveToGraveyard(context.state, object)
	}
	return nil
}

func (context *builtinAbilityContext) sacrificeHighestCost() error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	target, err := context.target(0)
	if err != nil || target.Zone != ZoneBoard || target.ControllerSeat != context.command.Seat.Other() {
		return fmt.Errorf("%w: sacrifice target must be on the opponent's board", ErrInvalidCommand)
	}
	definition, err := context.card(target)
	if err != nil {
		return err
	}
	cost := 0
	if definition.Cost != nil {
		cost = *definition.Cost
	}
	for _, object := range context.state.player(context.command.Seat.Other()).Objects {
		if object.Zone != ZoneBoard {
			continue
		}
		candidate, ok := context.budget.cards[object.DefinitionID]
		if ok && candidate.Cost != nil && *candidate.Cost > cost {
			return fmt.Errorf("%w: selected card does not have the greatest cost", ErrInvalidCommand)
		}
	}
	moveToGraveyard(context.state, target)
	return nil
}

func (context *builtinAbilityContext) randomHandToDeck(seat Seat) error {
	hand := context.state.objectsInZone(seat, ZoneHand)
	if len(hand) == 0 {
		return nil
	}
	selected := hand[context.state.Random.intn(len(hand))]
	moveObject(selected, ZoneDeck, len(context.state.objectsInZone(seat, ZoneDeck)))
	context.state.shuffleDeck(seat)
	return nil
}

func (context *builtinAbilityContext) randomBoardToDeck(seat Seat) error {
	board := context.state.objectsInZone(seat, ZoneBoard)
	if len(board) == 0 {
		return nil
	}
	selected := board[context.state.Random.intn(len(board))]
	moveObject(selected, ZoneDeck, len(context.state.objectsInZone(selected.OwnerSeat, ZoneDeck)))
	context.state.shuffleDeck(selected.OwnerSeat)
	return nil
}

func (context *builtinAbilityContext) returnRandomOtherGraveyardCard() error {
	graveyard := context.state.objectsInZone(context.command.Seat, ZoneGraveyard)
	candidates := make([]*CardObject, 0, len(graveyard))
	for _, object := range graveyard {
		if context.source == nil || object.Ref() != *context.source {
			candidates = append(candidates, object)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	selected := candidates[context.state.Random.intn(len(candidates))]
	moveObject(selected, ZoneHand, len(context.state.objectsInZone(context.command.Seat, ZoneHand)))
	return nil
}

func (context *builtinAbilityContext) chaoticExchange() error {
	if err := context.modifyEnhancedUnit(2, DurationUntilEndOfTurn); err != nil {
		return err
	}
	if len(context.targets) == 0 {
		return nil
	}
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	enhanced, err := context.enhancedUnit()
	if err != nil {
		return err
	}
	other, err := context.target(0)
	if err != nil || other.Zone != ZoneBoard || other.Slot != SlotUnit || other.ControllerSeat != context.command.Seat || other.InstanceID == enhanced.InstanceID {
		return fmt.Errorf("%w: exchange target must be another controlled unit", ErrInvalidCommand)
	}
	enhanced.Lane, other.Lane = other.Lane, enhanced.Lane
	return nil
}

func (context *builtinAbilityContext) chaoticSummon() error {
	context.state.shuffleDeck(context.command.Seat)
	deck := context.state.objectsInZone(context.command.Seat, ZoneDeck)
	if len(deck) == 0 {
		return context.draw(context.command.Seat, 2)
	}
	definition, ok := context.budget.cards[deck[0].DefinitionID]
	if ok && cardHasType(definition, CardUnit) && definition.Cost != nil && *definition.Cost <= 4 && context.command.DestinationLaneSet {
		context.targets = []ObjectRef{deck[0].Ref()}
		if err := context.putTargetOnBoard(context.command.Seat); err == nil {
			return nil
		}
	}
	return context.draw(context.command.Seat, 2)
}

func (context *builtinAbilityContext) payXAndModify() error {
	if context.command.Value < 0 || context.state.player(context.command.Seat).Influence < context.command.Value {
		return fmt.Errorf("%w: variable influence payment is invalid", ErrInvalidCommand)
	}
	context.state.player(context.command.Seat).Influence -= context.command.Value
	return context.modifyTargets(context.command.Value, DurationUntilEndOfTurn)
}

func (context *builtinAbilityContext) maxBoardUnitPower(seat Seat) int {
	maximum := 0
	for _, object := range context.state.player(seat).Objects {
		if object.Zone == ZoneBoard && object.Slot == SlotUnit && object.Power > maximum {
			maximum = object.Power
		}
	}
	return maximum
}

func (context *builtinAbilityContext) revealedOrders(seat Seat) int {
	count := 0
	for _, order := range context.state.player(seat).Orders {
		if order.Revealed {
			count++
		}
	}
	return count
}

func (context *builtinAbilityContext) modifyAllBoardUnits(seat Seat, amount int, duration DurationKind) error {
	refs := make([]ObjectRef, 0)
	for index := range context.state.player(seat).Objects {
		object := &context.state.player(seat).Objects[index]
		if object.Zone == ZoneBoard && object.Slot == SlotUnit && object.ControllerSeat == seat {
			refs = append(refs, object.Ref())
		}
	}
	if len(refs) == 0 {
		return nil
	}
	original := context.targets
	context.targets = refs
	err := context.modifyTargets(amount, duration)
	context.targets = original
	return err
}

func (context *builtinAbilityContext) burningFire() error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	center, err := context.target(0)
	if err != nil || center.Zone != ZoneBoard || center.Slot != SlotUnit || center.ControllerSeat != context.command.Seat.Other() {
		return fmt.Errorf("%w: center target must be an opponent unit", ErrInvalidCommand)
	}
	refs := []ObjectRef{center.Ref()}
	for _, delta := range []int{-1, 1} {
		if adjacent := context.state.boardObject(center.ControllerSeat, center.Lane+delta, SlotUnit); adjacent != nil {
			refs = append(refs, adjacent.Ref())
		}
	}
	context.targets = refs
	return context.modifyTargets(-3, DurationUntilEndOfTurn)
}

func (context *builtinAbilityContext) bjornBoost() error {
	if err := context.requireTargetCount(1, 1); err != nil {
		return err
	}
	target, err := context.target(0)
	if err != nil || target.Zone != ZoneBoard || target.Slot != SlotUnit {
		return fmt.Errorf("%w: Bjorn target must be a board unit", ErrInvalidCommand)
	}
	amount := 1
	if context.state.boardObject(target.ControllerSeat, target.Lane, SlotSupport) != nil {
		amount++
	}
	return context.modifyTargets(amount, DurationUntilEndOfTurn)
}

func (context *builtinAbilityContext) mill(seat Seat, count int) error {
	for range count {
		deck := context.state.objectsInZone(seat, ZoneDeck)
		if len(deck) == 0 {
			break
		}
		moveToGraveyard(context.state, deck[0])
	}
	return nil
}

func (context *builtinAbilityContext) recordContinuous(detail string) error {
	seat := context.command.Seat
	return emitEffectEvent(context.state, context.budget, "continuous_rule_installed", &seat, detail)
}

func distance(left, right int) int {
	if left > right {
		return left - right
	}
	return right - left
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func sortRefs(refs []ObjectRef) {
	sort.Slice(refs, func(left, right int) bool {
		if refs[left].ZoneObjectID != refs[right].ZoneObjectID {
			return refs[left].ZoneObjectID < refs[right].ZoneObjectID
		}
		return refs[left].Generation < refs[right].Generation
	})
}
