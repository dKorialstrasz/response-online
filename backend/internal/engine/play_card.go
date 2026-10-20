package engine

import (
	"fmt"
)

// applyPlayCard owns the complete play transaction. It validates against a
// server-authored LegalAction before changing the clone, pays all costs,
// changes zone generation, and invokes only abilities whose exact timing is a
// play window (spell resolution, Summon, or a continuous enhancement).
func applyPlayCard(
	state *State,
	command Command,
	abilities map[string]AbilityDefinition,
	cards map[string]CardDefinition,
	budget *eventBudget,
) error {
	if state.Phase != PhaseAction || state.ActingSeat != command.Seat || state.ActionResolution != nil {
		return fmt.Errorf("%w: playing a card requires the acting seat and no unresolved action", ErrInvalidCommand)
	}
	if command.SourceRef == nil {
		return fmt.Errorf("%w: card source reference is required", ErrInvalidCommand)
	}
	object, err := state.findObject(*command.SourceRef)
	if err != nil || object.OwnerSeat != command.Seat || object.ControllerSeat != command.Seat || object.Zone != ZoneHand {
		return fmt.Errorf("%w: card source is stale or not in the acting hand", ErrInvalidCommand)
	}
	definition, exists := cards[object.DefinitionID]
	if !exists || !cardHasType(definition, command.DeclaredType) ||
		(command.DeclaredType != CardUnit && command.DeclaredType != CardSpell && command.DeclaredType != CardEnhancement) {
		return fmt.Errorf("%w: declared card type does not match the pinned playable definition", ErrInvalidCommand)
	}

	action, ok := legalPlayAction(*state, command.Seat, object, definition, abilities, cards)
	if !ok || !playCommandMatchesLegalAction(command, action) {
		return fmt.Errorf("%w: play command is outside the server-authored allowlist", ErrInvalidCommand)
	}
	player := state.player(command.Seat)
	if player.ActionsRemaining <= 0 || len(action.InfluenceCosts) != 1 || player.Influence < action.InfluenceCosts[0] {
		return fmt.Errorf("%w: card cost or action budget is insufficient", ErrInvalidCommand)
	}

	// Ability payments are part of the same clone and therefore roll back with
	// placement, cost, effect validation, or an invariant failure.
	for _, abilityID := range definition.AbilityIDs {
		ability, exists := abilities[abilityID]
		if !exists || !builtinRunsOnPlay(ability.BuiltinHandler, command.DeclaredType) {
			continue
		}
		if err := validateAndApplyPayment(state, command, ability.Payment); err != nil {
			return err
		}
	}
	player.Influence -= action.InfluenceCosts[0]
	player.ActionsRemaining--
	state.ConsecutivePasses = 0
	state.ActionResolution = &ActionResolutionState{Seat: command.Seat}

	if command.DeclaredType == CardSpell {
		moveToGraveyard(state, object)
	} else {
		moveObject(object, ZoneBoard, command.DestinationLane)
		object.ControllerSeat = command.Seat
		object.Lane = command.DestinationLane
		object.Slot = command.DestinationSlot
		if definition.BasePower != nil {
			object.BasePower = *definition.BasePower
			object.Power = *definition.BasePower
		}
		object.Keywords = append([]Keyword(nil), definition.NativeKeywords...)
		object.BaseMagicShieldCost = definition.BaseMagicShieldCost
	}
	seat := command.Seat
	if err := budget.emit("card_played", &seat, definition.CardID); err != nil {
		return err
	}

	for _, abilityID := range definition.AbilityIDs {
		ability, exists := abilities[abilityID]
		if !exists || !builtinRunsOnPlay(ability.BuiltinHandler, command.DeclaredType) {
			continue
		}
		abilityCommand := command
		abilityCommand.AbilityID = abilityID
		abilityCommand.Kind = CommandActivateAbility
		var source *ObjectRef
		if command.DeclaredType != CardSpell {
			ref := object.Ref()
			source = &ref
		}
		targets, err := applyMagicShieldPayments(state, abilityCommand, command.TargetRefs)
		if err != nil {
			return err
		}
		if err := executeBuiltinAbility(state, abilityCommand, source, targets, budget); err != nil {
			return err
		}
	}
	if err := resolveEffectQueue(state, budget); err != nil {
		return err
	}
	return finishActionResolution(state, budget)
}

func builtinRunsOnPlay(handler string, cardType CardType) bool {
	number, ok := builtinCardNumber(handler)
	if !ok {
		return false
	}
	if cardType == CardSpell {
		return true
	}
	if cardType == CardUnit {
		switch number {
		case 4, 9, 28, 29, 71, 93, 96, 112, 113:
			return true // exact text: Призыв
		default:
			return false
		}
	}
	if cardType == CardEnhancement {
		switch number {
		case 19, 37, 39, 41, 63, 77, 85, 100, 101, 108, 123, 124, 125, 129, 130:
			return true // static "Усиленный ..." text
		default:
			return false
		}
	}
	return false
}

func playCommandMatchesLegalAction(command Command, action LegalAction) bool {
	if action.SourceRef == nil || command.SourceRef == nil || *action.SourceRef != *command.SourceRef ||
		action.DeclaredType != command.DeclaredType || !sameStringMembers(command.ModeIDs, action.ModeIDs, action.MinModes, action.MaxModes) ||
		!refsWithin(command.TargetRefs, action.TargetRefs, action.MinTargets, action.MaxTargets) ||
		!refsWithin(command.PaymentRefs, action.PaymentRefs, action.PaymentCount, action.PaymentCount) ||
		!refsWithin(command.ShieldPaymentRefs, action.ShieldTargets, 0, len(action.ShieldTargets)) {
		return false
	}
	if command.DeclaredType == CardSpell {
		if len(action.Placements) == 0 {
			return !command.DestinationLaneSet && command.DestinationSlot == SlotNone
		}
		for _, placement := range action.Placements {
			if command.DestinationLaneSet && command.DestinationLane == placement.Lane && command.DestinationSlot == placement.Slot {
				return true
			}
		}
		return false
	}
	for _, placement := range action.Placements {
		if command.DestinationLaneSet && command.DestinationLane == placement.Lane && command.DestinationSlot == placement.Slot {
			return true
		}
	}
	return false
}

func sameStringMembers(actual, allowed []string, minimum, maximum int) bool {
	if len(actual) < minimum || len(actual) > maximum {
		return false
	}
	available := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		available[value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		if _, ok := available[value]; !ok {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func refsWithin(actual, allowed []ObjectRef, minimum, maximum int) bool {
	if len(actual) < minimum || len(actual) > maximum {
		return false
	}
	available := make(map[ObjectRef]struct{}, len(allowed))
	for _, ref := range allowed {
		available[ref] = struct{}{}
	}
	seen := make(map[ObjectRef]struct{}, len(actual))
	for _, ref := range actual {
		if _, ok := available[ref]; !ok {
			return false
		}
		if _, duplicate := seen[ref]; duplicate {
			return false
		}
		seen[ref] = struct{}{}
	}
	return true
}

func legalPlayAction(
	state State,
	seat Seat,
	object *CardObject,
	definition CardDefinition,
	abilities map[string]AbilityDefinition,
	cards map[string]CardDefinition,
) (LegalAction, bool) {
	if state.Phase != PhaseAction || state.ActingSeat != seat || state.ActionResolution != nil || len(state.PendingChoices) > 0 ||
		object.Zone != ZoneHand || object.OwnerSeat != seat || object.ControllerSeat != seat || state.player(seat).ActionsRemaining <= 0 {
		return LegalAction{}, false
	}
	var declared CardType
	for _, candidate := range []CardType{CardUnit, CardSpell, CardEnhancement} {
		if cardHasType(definition, candidate) {
			declared = candidate
			break
		}
	}
	if declared == "" {
		return LegalAction{}, false
	}
	cost := effectiveCardCost(state, seat, definition, abilities, cards)
	if state.player(seat).Influence < cost {
		return LegalAction{}, false
	}
	action := LegalAction{
		Name: "playCard", SourceRef: refPointer(object.Ref()), DefinitionID: definition.CardID,
		DeclaredType: declared, InfluenceCosts: []int{cost},
	}
	if declared == CardUnit {
		action.Placements = legalPlacements(state, seat, SlotUnit)
	} else if declared == CardEnhancement {
		for lane := 0; lane < LaneCount; lane++ {
			if state.boardObject(seat, lane, SlotUnit) != nil && state.boardObject(seat, lane, SlotSupport) == nil {
				action.Placements = append(action.Placements, LegalPlacement{Lane: lane, Slot: SlotSupport})
			}
		}
	}
	if declared != CardSpell && len(action.Placements) == 0 {
		return LegalAction{}, false
	}

	for _, abilityID := range definition.AbilityIDs {
		ability, exists := abilities[abilityID]
		if !exists || !builtinRunsOnPlay(ability.BuiltinHandler, declared) {
			continue
		}
		action.AbilityID = abilityID
		for _, mode := range ability.Modes {
			action.ModeIDs = append(action.ModeIDs, mode.ID)
		}
		action.MinModes, action.MaxModes = ability.MinModes, ability.MaxModes
		number, _ := builtinCardNumber(ability.BuiltinHandler)
		action.TargetRefs, action.MinTargets, action.MaxTargets = legalInitialTargets(state, seat, number, cards)
		if len(action.TargetRefs) < action.MinTargets {
			return LegalAction{}, false
		}
		if declared == CardSpell && (number == 42 || number == 109) {
			action.Placements = legalPlacements(state, seat, SlotUnit)
		}
		for playerIndex := range state.Players {
			for objectIndex := range state.Players[playerIndex].Objects {
				candidate := &state.Players[playerIndex].Objects[objectIndex]
				if candidate.ControllerSeat == seat && zoneAllowed(candidate.Zone, ability.Payment.SacrificeZones) {
					action.PaymentRefs = append(action.PaymentRefs, candidate.Ref())
				}
			}
		}
		action.PaymentCount = ability.Payment.SacrificeCount
		for _, ref := range action.TargetRefs {
			candidate, err := state.findObject(ref)
			if err == nil && candidate.ControllerSeat != seat && candidate.Zone == ZoneBoard && candidateMagicShieldCost(&state, ref) > 0 {
				action.ShieldTargets = append(action.ShieldTargets, ref)
			}
		}
	}
	sortRefs(action.TargetRefs)
	sortRefs(action.PaymentRefs)
	sortRefs(action.ShieldTargets)
	return action, true
}

func (match *Match) LegalActions(seat Seat) []LegalAction {
	if !seat.Valid() || match.state.Status == StatusFinished {
		return nil
	}
	actions := []LegalAction{{Name: "surrender"}}
	state := match.state
	if len(state.PendingChoices) > 0 || state.Phase != PhaseAction || state.ActingSeat != seat || state.ActionResolution != nil {
		return actions
	}
	actions = append(actions, LegalAction{Name: "passAction"})
	for index := range state.player(seat).Objects {
		object := &state.player(seat).Objects[index]
		definition, exists := match.cards[object.DefinitionID]
		if !exists {
			continue
		}
		if action, ok := legalPlayAction(state, seat, object, definition, match.abilities, match.cards); ok {
			actions = append(actions, action)
		}
		if object.Zone == ZoneBoard {
			for _, abilityID := range definition.AbilityIDs {
				ability, ok := match.abilities[abilityID]
				if !ok || ability.Status != AbilityReady || !builtinIsAction(ability.BuiltinHandler) ||
					!ability.SourceRequired || !zoneAllowed(object.Zone, ability.SourceZones) {
					continue
				}
				number, _ := builtinCardNumber(ability.BuiltinHandler)
				targets, minimum, maximum := legalInitialTargets(state, seat, number, match.cards)
				if len(targets) < minimum {
					continue
				}
				action := LegalAction{
					Name: "activateAbility", SourceRef: refPointer(object.Ref()), DefinitionID: definition.CardID,
					AbilityID: abilityID, MinModes: ability.MinModes, MaxModes: ability.MaxModes,
					TargetRefs: targets, MinTargets: minimum, MaxTargets: maximum,
					PaymentCount: ability.Payment.SacrificeCount,
				}
				for _, mode := range ability.Modes {
					action.ModeIDs = append(action.ModeIDs, mode.ID)
				}
				for playerIndex := range state.Players {
					for candidateIndex := range state.Players[playerIndex].Objects {
						candidate := &state.Players[playerIndex].Objects[candidateIndex]
						if candidate.ControllerSeat == seat && zoneAllowed(candidate.Zone, ability.Payment.SacrificeZones) {
							action.PaymentRefs = append(action.PaymentRefs, candidate.Ref())
						}
					}
				}
				if len(action.PaymentRefs) < action.PaymentCount {
					continue
				}
				if number == 76 {
					for value := 0; value <= state.player(seat).Influence; value++ {
						action.InfluenceCosts = append(action.InfluenceCosts, value)
					}
				} else {
					if state.player(seat).Influence < ability.Payment.Influence {
						continue
					}
					action.InfluenceCosts = []int{ability.Payment.Influence}
				}
				sortRefs(action.TargetRefs)
				sortRefs(action.PaymentRefs)
				actions = append(actions, action)
			}
		}
	}
	return actions
}

func legalPlacements(state State, seat Seat, slot BoardSlot) []LegalPlacement {
	result := make([]LegalPlacement, 0, LaneCount)
	for lane := 0; lane < LaneCount; lane++ {
		if state.boardObject(seat, lane, slot) == nil {
			result = append(result, LegalPlacement{Lane: lane, Slot: slot})
		}
	}
	return result
}

func effectiveCardCost(state State, seat Seat, definition CardDefinition, abilities map[string]AbilityDefinition, cards map[string]CardDefinition) int {
	cost := 0
	if definition.Cost != nil {
		cost = *definition.Cost
	}
	if definition.CardID != "baz-073" {
		return cost
	}
	discountSources := 0
	for _, object := range state.player(seat).Objects {
		if object.Zone != ZoneBoard || object.ControllerSeat != seat {
			continue
		}
		card, ok := cards[object.DefinitionID]
		if !ok || (!cardHasType(card, CardUnit) && !cardHasType(card, CardEnhancement)) {
			continue
		}
		for _, abilityID := range card.AbilityIDs {
			if ability, ok := abilities[abilityID]; ok && builtinIsAction(ability.BuiltinHandler) {
				discountSources++
				break
			}
		}
	}
	cost -= 2 * discountSources
	if cost < 0 {
		return 0
	}
	return cost
}

func builtinIsAction(handler string) bool {
	number, ok := builtinCardNumber(handler)
	if !ok {
		return false
	}
	switch number {
	case 1, 16, 23, 27, 34, 45, 48, 60, 67, 70, 75, 76, 78, 80, 89, 94, 99, 111, 119, 126:
		return true
	default:
		return false
	}
}

func candidateMagicShieldCost(state *State, ref ObjectRef) int {
	object, err := state.findObject(ref)
	if err != nil {
		return 0
	}
	return object.BaseMagicShieldCost + modifierInt(state, ref, func(modifier ModifierState) int { return modifier.MagicShieldCost })
}

func refPointer(ref ObjectRef) *ObjectRef { return &ref }

func legalInitialTargets(state State, seat Seat, number int, cards map[string]CardDefinition) ([]ObjectRef, int, int) {
	// Hand/revealed/foreign decisions are intentionally absent here: their
	// allowlists are generated only after the state change as PendingChoice.
	if number == 21 || number == 36 || number == 65 || number == 88 || number == 107 || number == 121 || number == 20 || number == 38 || number == 43 || number == 44 || number == 102 || number == 105 {
		return nil, 0, 0
	}
	refs := make([]ObjectRef, 0)
	add := func(object *CardObject) { refs = append(refs, object.Ref()) }
	for playerIndex := range state.Players {
		for objectIndex := range state.Players[playerIndex].Objects {
			object := &state.Players[playerIndex].Objects[objectIndex]
			definition := cards[object.DefinitionID]
			switch number {
			case 11:
				if object.Zone == ZoneBoard && object.Slot == SlotUnit {
					add(object)
				}
			case 12:
				if object.Zone == ZoneBoard && !cardHasType(definition, CardOrder) {
					add(object)
				}
			case 13:
				if object.OwnerSeat == seat && object.Zone == ZoneGraveyard {
					add(object)
				}
			case 14, 17, 33, 56, 58, 62, 88, 104, 111:
				if object.Zone == ZoneBoard && object.Slot == SlotUnit {
					add(object)
				}
			case 127:
				if object.ControllerSeat == seat && object.Zone == ZoneBoard && object.Slot == SlotUnit {
					add(object)
				}
			case 21, 79, 83, 110:
				if object.ControllerSeat != seat && object.Zone == ZoneBoard && object.Slot == SlotUnit {
					add(object)
				}
			case 22, 40, 84:
				if object.ControllerSeat != seat && object.Zone == ZoneBoard {
					add(object)
				}
			case 35, 82, 103, 122:
				if object.Zone == ZoneBoard {
					add(object)
				}
			case 42:
				if object.Zone == ZoneGraveyard && cardHasType(definition, CardUnit) {
					add(object)
				}
			case 93:
				if object.Zone == ZoneGraveyard && cardHasType(definition, CardUnit) {
					add(object)
				}
			case 55, 61, 64, 66, 87, 106, 128, 131:
				// no object target in a two-player match
			case 57:
				// Tribute uses paymentRefs, not targetRefs.
			case 65:
				if object.OwnerSeat == seat && (object.Zone == ZoneGraveyard || object.Zone == ZoneVoid) {
					add(object)
				}
			case 81:
				if object.ControllerSeat == seat && object.Zone == ZoneBoard {
					add(object)
				}
			case 86:
				// post-draw decision
			case 107:
				if object.Zone != ZoneDeck && object.Zone != ZoneHand {
					add(object)
				}
			case 109:
				if object.Zone == ZoneVoid && cardHasType(definition, CardUnit) {
					add(object)
				}
			case 132:
				if object.Zone == ZoneBoard || object.Zone == ZoneGraveyard {
					add(object)
				}
			}
		}
	}
	switch number {
	case 21, 65:
		return refs, 0, 2
	case 88, 107:
		return refs, 0, 1
	case 55, 57, 61, 64, 66, 87, 106, 128, 131:
		return nil, 0, 0
	default:
		if len(refs) == 0 {
			if builtinRequiresInitialTarget(number) {
				return nil, 1, 1
			}
			return nil, 0, 0
		}
		return refs, 1, 1
	}
}

func builtinRequiresInitialTarget(number int) bool {
	switch number {
	case 1, 6, 11, 12, 13, 14, 17, 22, 33, 35, 40, 42, 45, 48, 56, 58, 62,
		76, 79, 81, 82, 84, 93, 103, 104, 109, 110, 111, 119, 122, 127, 132:
		return true
	default:
		return false
	}
}
