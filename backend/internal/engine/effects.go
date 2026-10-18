package engine

import (
	"errors"
	"fmt"
	"sort"
)

var ErrAbilityBlocked = errors.New("ability is blocked by an unresolved ruling")

type effectHandler func(*State, ResolvedEffect, *eventBudget) error

// effectHandlerRegistry returns the closed registry of deterministic primitives.
// Card-specific EffectSpec documents compose these handlers in #12; they cannot
// register callbacks that read time, randomness, maps, or external services.
func effectHandlerRegistry() map[EffectKind]effectHandler {
	return map[EffectKind]effectHandler{
		EffectModifyPower:        handleModifyPower,
		EffectMoveZone:           handleMoveZone,
		EffectEnqueue:            handleEnqueue,
		EffectRequestChoice:      handleRequestChoice,
		EffectScheduleDelayed:    handleScheduleDelayed,
		EffectInstallReplacement: handleInstallReplacement,
	}
}

func cloneObjectRef(ref *ObjectRef) *ObjectRef {
	if ref == nil {
		return nil
	}
	result := *ref
	return &result
}

func cloneResolvedEffects(effects []ResolvedEffect) []ResolvedEffect {
	result := make([]ResolvedEffect, len(effects))
	for index, effect := range effects {
		result[index] = cloneResolvedEffect(effect)
	}
	return result
}

func cloneResolvedEffect(effect ResolvedEffect) ResolvedEffect {
	result := effect
	result.SourceRef = cloneObjectRef(effect.SourceRef)
	result.TargetRef = cloneObjectRef(effect.TargetRef)
	result.Effects = cloneResolvedEffects(effect.Effects)
	if effect.Choice != nil {
		choice := *effect.Choice
		choice.Options = make([]ChoiceOption, len(effect.Choice.Options))
		for index, option := range effect.Choice.Options {
			choice.Options[index] = option
			choice.Options[index].Objects = append([]ObjectRef(nil), option.Objects...)
			choice.Options[index].Effects = cloneResolvedEffects(option.Effects)
		}
		result.Choice = &choice
	}
	if effect.Trigger != nil {
		trigger := *effect.Trigger
		trigger.Effects = cloneResolvedEffects(effect.Trigger.Effects)
		result.Trigger = &trigger
	}
	if effect.Replacement != nil {
		replacement := *effect.Replacement
		replacement.Effects = cloneResolvedEffects(effect.Replacement.Effects)
		result.Replacement = &replacement
	}
	return result
}

func (state State) PendingChoiceProjections(viewer Seat) []PendingChoiceProjection {
	result := make([]PendingChoiceProjection, 0, len(state.PendingChoices))
	for _, choice := range state.PendingChoices {
		projection := PendingChoiceProjection{
			WaitingSeat: choice.Seat,
			Waiting:     true,
			Revision:    choice.Revision,
		}
		if choice.Visibility == ChoiceOwnerOnly && viewer != choice.Seat {
			projection.Redacted = true
			result = append(result, projection)
			continue
		}
		projection.ChoiceID = choice.ChoiceID
		projection.Kind = choice.Kind
		projection.Mandatory = choice.Mandatory
		projection.OptionCount = len(choice.Options)
		if viewer == choice.Seat {
			projection.Private = true
			projection.Options = make([]ChoiceOption, len(choice.Options))
			for index, option := range choice.Options {
				projection.Options[index] = ChoiceOption{
					ID:      option.ID,
					Objects: append([]ObjectRef(nil), option.Objects...),
				}
			}
		}
		result = append(result, projection)
	}
	return result
}

func buildAbilityCatalog(definitions []AbilityDefinition) (map[string]AbilityDefinition, error) {
	catalog := make(map[string]AbilityDefinition, len(definitions))
	for _, definition := range definitions {
		if definition.BuiltinHandler != "" {
			spec, ok := builtinAbility(definition.BuiltinHandler)
			if !ok || definition.BuiltinHandler != definition.AbilityID {
				return nil, fmt.Errorf("ability %q: builtin handler is not in the closed Base registry", definition.AbilityID)
			}
			definition.SourceRequired = spec.sourceRequired
			definition.SourceZones = append([]Zone(nil), spec.sourceZones...)
			definition.ConsumesAction = spec.consumesAction
			definition.Payment = spec.payment
			definition.Targets = TargetDefinition{Min: 0, Max: 8, Controller: TargetAny}
		}
		if err := validateAbilityDefinition(definition); err != nil {
			return nil, fmt.Errorf("ability %q: %w", definition.AbilityID, err)
		}
		if _, exists := catalog[definition.AbilityID]; exists {
			return nil, fmt.Errorf("ability %q is duplicated", definition.AbilityID)
		}
		catalog[definition.AbilityID] = cloneAbilityDefinition(definition)
	}
	return catalog, nil
}

func cloneAbilityDefinition(definition AbilityDefinition) AbilityDefinition {
	result := definition
	result.ConformanceScenarioIDs = append([]string(nil), definition.ConformanceScenarioIDs...)
	result.SourceZones = append([]Zone(nil), definition.SourceZones...)
	result.Targets.Zones = append([]Zone(nil), definition.Targets.Zones...)
	result.Payment.SacrificeZones = append([]Zone(nil), definition.Payment.SacrificeZones...)
	result.Effects = cloneInstructions(definition.Effects)
	result.Modes = make([]ModeDefinition, len(definition.Modes))
	for index, mode := range definition.Modes {
		result.Modes[index] = mode
		result.Modes[index].Effects = cloneInstructions(mode.Effects)
	}
	return result
}

func cloneInstructions(instructions []EffectInstruction) []EffectInstruction {
	result := make([]EffectInstruction, len(instructions))
	for index, instruction := range instructions {
		result[index] = instruction
		result[index].Effects = cloneInstructions(instruction.Effects)
		if instruction.Choice != nil {
			choice := *instruction.Choice
			choice.Options = make([]ChoiceOptionDefinition, len(instruction.Choice.Options))
			for optionIndex, option := range instruction.Choice.Options {
				choice.Options[optionIndex] = option
				choice.Options[optionIndex].TargetIndexes = append([]int(nil), option.TargetIndexes...)
				choice.Options[optionIndex].Effects = cloneInstructions(option.Effects)
			}
			result[index].Choice = &choice
		}
		if instruction.Trigger != nil {
			trigger := *instruction.Trigger
			trigger.Effects = cloneInstructions(instruction.Trigger.Effects)
			result[index].Trigger = &trigger
		}
		if instruction.Replacement != nil {
			replacement := *instruction.Replacement
			replacement.Effects = cloneInstructions(instruction.Replacement.Effects)
			result[index].Replacement = &replacement
		}
	}
	return result
}

func validateAbilityDefinition(definition AbilityDefinition) error {
	if definition.AbilityID == "" || len(definition.ConformanceScenarioIDs) == 0 {
		return errors.New("abilityId and conformance scenarios are required")
	}
	scenarios := make(map[string]struct{}, len(definition.ConformanceScenarioIDs))
	for _, scenarioID := range definition.ConformanceScenarioIDs {
		if scenarioID == "" {
			return errors.New("conformance scenario id cannot be empty")
		}
		if _, exists := scenarios[scenarioID]; exists {
			return fmt.Errorf("conformance scenario %q is duplicated", scenarioID)
		}
		scenarios[scenarioID] = struct{}{}
	}
	if definition.Status == AbilityBlocked {
		if definition.BlockingDecisionRef == "" {
			return errors.New("blocked ability requires an OQ/ruling reference")
		}
		return nil
	}
	if definition.Status != AbilityReady {
		return errors.New("status must be ready or blocked")
	}
	if definition.BuiltinHandler != "" {
		if _, ok := builtinAbility(definition.BuiltinHandler); !ok || definition.BuiltinHandler != definition.AbilityID {
			return errors.New("builtin handler is not in the closed Base registry")
		}
	}
	if definition.MinModes < 0 || definition.MaxModes < definition.MinModes || definition.MaxModes > len(definition.Modes) {
		return errors.New("invalid mode bounds")
	}
	modeIDs := make(map[string]struct{}, len(definition.Modes))
	for _, mode := range definition.Modes {
		if mode.ID == "" {
			return errors.New("mode id is required")
		}
		if _, exists := modeIDs[mode.ID]; exists {
			return fmt.Errorf("mode %q is duplicated", mode.ID)
		}
		modeIDs[mode.ID] = struct{}{}
		if err := validateInstructions(mode.Effects); err != nil {
			return fmt.Errorf("mode %q: %w", mode.ID, err)
		}
	}
	if !definition.SourceRequired {
		if instructionsRequireSource(definition.Effects) {
			return errors.New("while-source duration requires a source")
		}
		for _, mode := range definition.Modes {
			if instructionsRequireSource(mode.Effects) {
				return errors.New("while-source mode duration requires a source")
			}
		}
	}
	if definition.Targets.Min < 0 || definition.Targets.Max < definition.Targets.Min {
		return errors.New("invalid target bounds")
	}
	for _, zone := range append(append([]Zone(nil), definition.SourceZones...), definition.Targets.Zones...) {
		if !validZone(zone) || zone == ZoneQueue {
			return fmt.Errorf("invalid source/target zone %q", zone)
		}
	}
	if definition.Targets.Controller != TargetAny && definition.Targets.Controller != TargetSelf && definition.Targets.Controller != TargetOpponent {
		return errors.New("invalid target controller")
	}
	if definition.Payment.Influence < 0 || definition.Payment.SacrificeCount < 0 {
		return errors.New("payment values cannot be negative")
	}
	if definition.Payment.SacrificeCount > 0 && len(definition.Payment.SacrificeZones) == 0 {
		return errors.New("sacrifice payment requires allowed zones")
	}
	for _, zone := range definition.Payment.SacrificeZones {
		if !validZone(zone) || zone == ZoneQueue {
			return fmt.Errorf("invalid payment zone %q", zone)
		}
	}
	return validateInstructions(definition.Effects)
}

func instructionsRequireSource(instructions []EffectInstruction) bool {
	for _, instruction := range instructions {
		if instruction.Duration == DurationWhileSourceValid ||
			(instruction.Trigger != nil && instruction.Trigger.Duration == DurationWhileSourceValid) ||
			(instruction.Replacement != nil && instruction.Replacement.Duration == DurationWhileSourceValid) {
			return true
		}
		if instructionsRequireSource(instruction.Effects) {
			return true
		}
		if instruction.Choice != nil {
			for _, option := range instruction.Choice.Options {
				if instructionsRequireSource(option.Effects) {
					return true
				}
			}
		}
		if instruction.Trigger != nil && instructionsRequireSource(instruction.Trigger.Effects) {
			return true
		}
		if instruction.Replacement != nil && instructionsRequireSource(instruction.Replacement.Effects) {
			return true
		}
	}
	return false
}

func validateInstructions(instructions []EffectInstruction) error {
	for _, instruction := range instructions {
		if _, exists := effectHandlerRegistry()[instruction.Kind]; !exists {
			return fmt.Errorf("unknown effect kind %q", instruction.Kind)
		}
		switch instruction.Kind {
		case EffectModifyPower:
			if instruction.Amount == 0 || !validDuration(instruction.Duration) {
				return errors.New("power modifier requires non-zero amount and duration")
			}
		case EffectMoveZone:
			if !validZone(instruction.Destination) || instruction.Destination == ZoneQueue {
				return errors.New("move effect requires a concrete destination zone")
			}
		case EffectEnqueue:
			if !validQueuePosition(instruction.QueuePosition) || len(instruction.Effects) == 0 {
				return errors.New("enqueue effect requires position and children")
			}
			if err := validateInstructions(instruction.Effects); err != nil {
				return err
			}
		case EffectRequestChoice:
			if instruction.Choice == nil {
				return errors.New("choice effect requires a definition")
			}
			if err := validateChoiceDefinition(*instruction.Choice); err != nil {
				return err
			}
		case EffectScheduleDelayed:
			if instruction.Trigger == nil || instruction.Trigger.EventKind == "" || len(instruction.Trigger.Effects) == 0 || !validDuration(instruction.Trigger.Duration) {
				return errors.New("delayed effect requires event, duration, and effects")
			}
			if err := validateInstructions(instruction.Trigger.Effects); err != nil {
				return err
			}
		case EffectInstallReplacement:
			if instruction.Replacement == nil || instruction.Replacement.EffectKind == "" || len(instruction.Replacement.Effects) == 0 || !validDuration(instruction.Replacement.Duration) {
				return errors.New("replacement requires effect kind, duration, and effects")
			}
			if instruction.Replacement.EffectKind == EffectInstallReplacement {
				return errors.New("replacement cannot replace replacement installation")
			}
			if err := validateInstructions(instruction.Replacement.Effects); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateChoiceDefinition(choice ChoiceDefinition) error {
	if choice.Kind == "" || !validQueuePosition(choice.QueuePosition) ||
		(choice.Visibility != ChoicePublic && choice.Visibility != ChoiceOwnerOnly) {
		return errors.New("choice kind, visibility, and queue position are required")
	}
	if choice.MinSelections < 0 || choice.MaxSelections < choice.MinSelections || choice.MaxSelections > len(choice.Options) {
		return errors.New("invalid choice selection bounds")
	}
	if choice.Mandatory && choice.MinSelections == 0 {
		return errors.New("mandatory choice must require a selection")
	}
	if !choice.Mandatory && choice.MinSelections != 0 {
		return errors.New("optional choice must allow an empty selection")
	}
	seen := make(map[string]struct{}, len(choice.Options))
	for _, option := range choice.Options {
		if option.ID == "" {
			return errors.New("choice option id is required")
		}
		if _, exists := seen[option.ID]; exists {
			return fmt.Errorf("choice option %q is duplicated", option.ID)
		}
		seen[option.ID] = struct{}{}
		if err := validateInstructions(option.Effects); err != nil {
			return fmt.Errorf("choice option %q: %w", option.ID, err)
		}
	}
	return nil
}

func validZone(zone Zone) bool {
	return zone == ZoneDeck || zone == ZoneHand || zone == ZoneBoard || zone == ZoneGraveyard || zone == ZoneVoid || zone == ZoneQueue
}

func validDuration(duration DurationKind) bool {
	return duration == DurationPermanent || duration == DurationUntilEndOfCombat || duration == DurationUntilEndOfTurn || duration == DurationWhileSourceValid
}

func validQueuePosition(position QueuePosition) bool {
	return position == QueueBeforeExisting || position == QueueAfterExisting
}

func applyActivateAbility(state *State, command Command, catalog map[string]AbilityDefinition, budget *eventBudget) error {
	if state.Phase != PhaseAction || state.ActingSeat != command.Seat || state.ActionResolution != nil {
		return fmt.Errorf("%w: ability activation requires the acting seat and no unresolved action", ErrInvalidCommand)
	}
	definition, exists := catalog[command.AbilityID]
	if !exists {
		return fmt.Errorf("%w: unknown ability %q", ErrInvalidCommand, command.AbilityID)
	}
	if definition.Status == AbilityBlocked {
		return fmt.Errorf("%w: %s", ErrAbilityBlocked, definition.BlockingDecisionRef)
	}
	source, err := validateAbilitySource(state, command, definition)
	if err != nil {
		return err
	}
	targets, err := validateAbilityTargets(state, command, definition)
	if err != nil {
		return err
	}
	modes, err := selectAbilityModes(command.ModeIDs, definition)
	if err != nil {
		return err
	}
	if err := validateAndApplyPayment(state, command, definition.Payment); err != nil {
		return err
	}
	if definition.BuiltinHandler != "" {
		targets, err = applyMagicShieldPayments(state, command, targets)
		if err != nil {
			return err
		}
	}
	instructions := append([]EffectInstruction(nil), definition.Effects...)
	for _, mode := range modes {
		instructions = append(instructions, mode.Effects...)
	}
	effects, err := resolveInstructions(instructions, command.Seat, source, targets)
	if err != nil {
		return fmt.Errorf("%w: invalid resolved effect: %v", ErrInvalidCommand, err)
	}
	if definition.ConsumesAction {
		player := state.player(command.Seat)
		if player.ActionsRemaining <= 0 {
			return fmt.Errorf("%w: action budget is exhausted", ErrInvalidCommand)
		}
		player.ActionsRemaining--
		state.ActionResolution = &ActionResolutionState{Seat: command.Seat}
		state.ConsecutivePasses = 0
	}
	if definition.BuiltinHandler != "" {
		if err := executeBuiltinAbility(state, command, source, targets, budget); err != nil {
			return err
		}
	}
	batchID := fmt.Sprintf("ability:%s:r%d", definition.AbilityID, state.Revision+1)
	enqueueResolvedEffects(state, batchID, effects, QueueAfterExisting)
	seat := command.Seat
	if err := emitEffectEvent(state, budget, "ability_activated", &seat, definition.AbilityID); err != nil {
		return err
	}
	if err := resolveEffectQueue(state, budget); err != nil {
		return err
	}
	return finishActionResolution(state, budget)
}

func validateAbilitySource(state *State, command Command, definition AbilityDefinition) (*ObjectRef, error) {
	if command.SourceRef == nil {
		if definition.SourceRequired {
			return nil, fmt.Errorf("%w: ability source is required", ErrInvalidCommand)
		}
		return nil, nil
	}
	object, err := state.findObject(*command.SourceRef)
	if err != nil || object.ControllerSeat != command.Seat || !zoneAllowed(object.Zone, definition.SourceZones) {
		return nil, fmt.Errorf("%w: source is stale, uncontrolled, or in an invalid zone", ErrInvalidCommand)
	}
	ref := object.Ref()
	return &ref, nil
}

func validateAbilityTargets(state *State, command Command, definition AbilityDefinition) ([]ObjectRef, error) {
	if len(command.TargetRefs) < definition.Targets.Min || len(command.TargetRefs) > definition.Targets.Max {
		return nil, fmt.Errorf("%w: target count is outside the ability bounds", ErrInvalidCommand)
	}
	seen := make(map[string]struct{}, len(command.TargetRefs))
	result := make([]ObjectRef, 0, len(command.TargetRefs))
	for _, ref := range command.TargetRefs {
		object, err := state.findObject(ref)
		if err != nil || !zoneAllowed(object.Zone, definition.Targets.Zones) {
			return nil, fmt.Errorf("%w: target is stale or in an invalid zone", ErrInvalidCommand)
		}
		if definition.Targets.Controller == TargetSelf && object.ControllerSeat != command.Seat {
			return nil, fmt.Errorf("%w: target is not controlled by the acting seat", ErrInvalidCommand)
		}
		if definition.Targets.Controller == TargetOpponent && object.ControllerSeat == command.Seat {
			return nil, fmt.Errorf("%w: target is not controlled by the opponent", ErrInvalidCommand)
		}
		key := fmt.Sprintf("%s:%d", ref.ZoneObjectID, ref.Generation)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate target", ErrInvalidCommand)
		}
		seen[key] = struct{}{}
		result = append(result, object.Ref())
	}
	return result, nil
}

func selectAbilityModes(requested []string, definition AbilityDefinition) ([]ModeDefinition, error) {
	if len(requested) < definition.MinModes || len(requested) > definition.MaxModes {
		return nil, fmt.Errorf("%w: mode count is outside the ability bounds", ErrInvalidCommand)
	}
	byID := make(map[string]ModeDefinition, len(definition.Modes))
	for _, mode := range definition.Modes {
		byID[mode.ID] = mode
	}
	seen := make(map[string]struct{}, len(requested))
	result := make([]ModeDefinition, 0, len(requested))
	for _, id := range requested {
		mode, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("%w: unknown mode %q", ErrInvalidCommand, id)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate mode %q", ErrInvalidCommand, id)
		}
		seen[id] = struct{}{}
		result = append(result, mode)
	}
	return result, nil
}

func validateAndApplyPayment(state *State, command Command, payment PaymentDefinition) error {
	player := state.player(command.Seat)
	if player.Influence < payment.Influence || len(command.PaymentRefs) != payment.SacrificeCount {
		return fmt.Errorf("%w: payment is insufficient or has the wrong number of objects", ErrInvalidCommand)
	}
	objects := make([]*CardObject, 0, len(command.PaymentRefs))
	seen := make(map[string]struct{}, len(command.PaymentRefs))
	for _, ref := range command.PaymentRefs {
		object, err := state.findObject(ref)
		if err != nil || object.ControllerSeat != command.Seat || !zoneAllowed(object.Zone, payment.SacrificeZones) {
			return fmt.Errorf("%w: payment object is stale, uncontrolled, or in an invalid zone", ErrInvalidCommand)
		}
		if _, duplicate := seen[object.InstanceID]; duplicate {
			return fmt.Errorf("%w: payment object is duplicated", ErrInvalidCommand)
		}
		seen[object.InstanceID] = struct{}{}
		objects = append(objects, object)
	}
	player.Influence -= payment.Influence
	for _, object := range objects {
		moveToGraveyard(state, object)
	}
	return nil
}

func zoneAllowed(zone Zone, allowed []Zone) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if zone == candidate {
			return true
		}
	}
	return false
}

func resolveInstructions(instructions []EffectInstruction, controller Seat, source *ObjectRef, targets []ObjectRef) ([]ResolvedEffect, error) {
	result := make([]ResolvedEffect, 0, len(instructions))
	for _, instruction := range instructions {
		effect := ResolvedEffect{
			Kind:          instruction.Kind,
			Controller:    controller,
			SourceRef:     cloneObjectRef(source),
			Amount:        instruction.Amount,
			Layer:         instruction.Layer,
			Destination:   instruction.Destination,
			Duration:      instruction.Duration,
			QueuePosition: instruction.QueuePosition,
		}
		if instruction.Kind == EffectModifyPower || instruction.Kind == EffectMoveZone || instruction.Kind == EffectInstallReplacement {
			if instruction.TargetIndex < 0 || instruction.TargetIndex >= len(targets) {
				return nil, fmt.Errorf("target index %d is unavailable", instruction.TargetIndex)
			}
			ref := targets[instruction.TargetIndex]
			effect.TargetRef = &ref
		}
		var err error
		effect.Effects, err = resolveInstructions(instruction.Effects, controller, source, targets)
		if err != nil {
			return nil, err
		}
		if instruction.Choice != nil {
			resolved := ResolvedChoiceDefinition{
				Kind: instruction.Choice.Kind, Mandatory: instruction.Choice.Mandatory,
				Visibility: instruction.Choice.Visibility, MinSelections: instruction.Choice.MinSelections,
				MaxSelections: instruction.Choice.MaxSelections, OrderMatters: instruction.Choice.OrderMatters,
				QueuePosition: instruction.Choice.QueuePosition,
			}
			for _, option := range instruction.Choice.Options {
				choiceOption := ChoiceOption{ID: option.ID}
				for _, index := range option.TargetIndexes {
					if index < 0 || index >= len(targets) {
						return nil, fmt.Errorf("choice target index %d is unavailable", index)
					}
					choiceOption.Objects = append(choiceOption.Objects, targets[index])
				}
				choiceOption.Effects, err = resolveInstructions(option.Effects, controller, source, targets)
				if err != nil {
					return nil, err
				}
				resolved.Options = append(resolved.Options, choiceOption)
			}
			effect.Choice = &resolved
		}
		if instruction.Trigger != nil {
			children, err := resolveInstructions(instruction.Trigger.Effects, controller, source, targets)
			if err != nil {
				return nil, err
			}
			effect.Trigger = &ResolvedDelayedDefinition{
				EventKind: instruction.Trigger.EventKind, Once: instruction.Trigger.Once,
				Duration: instruction.Trigger.Duration, Effects: children,
			}
		}
		if instruction.Replacement != nil {
			children, err := resolveInstructions(instruction.Replacement.Effects, controller, source, targets)
			if err != nil {
				return nil, err
			}
			effect.Replacement = &ResolvedReplacementDefinition{
				EffectKind: instruction.Replacement.EffectKind, Once: instruction.Replacement.Once,
				Duration: instruction.Replacement.Duration, Effects: children,
			}
		}
		result = append(result, effect)
	}
	return result, nil
}

func enqueueResolvedEffects(state *State, batchID string, effects []ResolvedEffect, position QueuePosition) {
	entries := make([]EffectQueueEntry, 0, len(effects))
	for _, effect := range effects {
		state.NextEffectSequence++
		entries = append(entries, EffectQueueEntry{
			EntryID: fmt.Sprintf("effect-%d", state.NextEffectSequence), BatchID: batchID,
			EnqueueSequence: state.NextEffectSequence, Effect: cloneResolvedEffect(effect),
		})
	}
	if position == QueueBeforeExisting {
		state.EffectQueue = append(entries, state.EffectQueue...)
	} else {
		state.EffectQueue = append(state.EffectQueue, entries...)
	}
}

func resolveEffectQueue(state *State, budget *eventBudget) error {
	for len(state.EffectQueue) > 0 && len(state.PendingChoices) == 0 && state.Result == nil {
		entry := state.EffectQueue[0]
		state.EffectQueue = state.EffectQueue[1:]
		if replacement := matchingReplacement(state, entry.Effect); replacement != nil {
			children := cloneResolvedEffects(replacement.Effects)
			for index := range children {
				children[index].BypassReplacement = true
			}
			if replacement.Once {
				removeReplacement(state, replacement.ReplacementID)
			}
			enqueueResolvedEffects(state, entry.BatchID+":replacement", children, QueueBeforeExisting)
			if err := emitEffectEvent(state, budget, "effect_replaced", &replacement.Controller, replacement.ReplacementID); err != nil {
				return err
			}
			continue
		}
		handler := effectHandlerRegistry()[entry.Effect.Kind]
		if handler == nil {
			return fmt.Errorf("unsupported resolved effect %q", entry.Effect.Kind)
		}
		if err := handler(state, entry.Effect, budget); err != nil {
			return err
		}
	}
	return nil
}

func matchingReplacement(state *State, effect ResolvedEffect) *ReplacementState {
	if effect.BypassReplacement {
		return nil
	}
	for index := range state.Replacements {
		replacement := &state.Replacements[index]
		if replacement.EffectKind != effect.Kind || !durationSourceValid(state, replacement.Duration, replacement.SourceRef) {
			continue
		}
		if replacement.TargetRef != nil {
			if effect.TargetRef == nil || *replacement.TargetRef != *effect.TargetRef {
				continue
			}
		}
		return replacement
	}
	return nil
}

func removeReplacement(state *State, id string) {
	for index := range state.Replacements {
		if state.Replacements[index].ReplacementID == id {
			state.Replacements = append(state.Replacements[:index], state.Replacements[index+1:]...)
			return
		}
	}
}

func handleModifyPower(state *State, effect ResolvedEffect, budget *eventBudget) error {
	if effect.TargetRef == nil {
		return errors.New("power modifier has no target")
	}
	object, err := state.findObject(*effect.TargetRef)
	if err != nil {
		return emitEffectEvent(state, budget, "effect_target_stale", &effect.Controller, string(effect.Kind))
	}
	if object.Zone != ZoneBoard || object.Slot != SlotUnit {
		return emitEffectEvent(state, budget, "effect_target_illegal", &effect.Controller, string(effect.Kind))
	}
	state.NextEffectSequence++
	state.Modifiers = append(state.Modifiers, ModifierState{
		ModifierID: fmt.Sprintf("modifier-%d", state.NextEffectSequence), CreatedSequence: state.NextEffectSequence,
		SourceRef: cloneObjectRef(effect.SourceRef), TargetRef: object.Ref(), Layer: effect.Layer,
		PowerDelta: effect.Amount, Duration: effect.Duration, CreatedTurn: state.TurnNumber,
	})
	recalculatePower(state, object.Ref())
	return emitEffectEvent(state, budget, "power_modified", &effect.Controller, fmt.Sprintf("delta=%d", effect.Amount))
}

func handleMoveZone(state *State, effect ResolvedEffect, budget *eventBudget) error {
	if effect.TargetRef == nil {
		return errors.New("move effect has no target")
	}
	object, err := state.findObject(*effect.TargetRef)
	if err != nil {
		return emitEffectEvent(state, budget, "effect_target_stale", &effect.Controller, string(effect.Kind))
	}
	index := len(state.objectsInZone(object.OwnerSeat, effect.Destination))
	moveObject(object, effect.Destination, index)
	pruneStaleEffectState(state)
	return emitEffectEvent(state, budget, "object_moved", &effect.Controller, string(effect.Destination))
}

func handleEnqueue(state *State, effect ResolvedEffect, budget *eventBudget) error {
	enqueueResolvedEffects(state, fmt.Sprintf("nested:%d", state.NextEffectSequence+1), effect.Effects, effect.QueuePosition)
	return emitEffectEvent(state, budget, "effects_enqueued", &effect.Controller, string(effect.QueuePosition))
}

func handleRequestChoice(state *State, effect ResolvedEffect, budget *eventBudget) error {
	if effect.Choice == nil {
		return errors.New("choice effect has no resolved definition")
	}
	state.NextChoiceSequence++
	choice := PendingChoice{
		ChoiceID: fmt.Sprintf("choice-%d", state.NextChoiceSequence), Seat: effect.Controller,
		Kind: effect.Choice.Kind, Mandatory: effect.Choice.Mandatory, Visibility: effect.Choice.Visibility,
		MinSelections: effect.Choice.MinSelections, MaxSelections: effect.Choice.MaxSelections,
		OrderMatters: effect.Choice.OrderMatters, QueuePosition: effect.Choice.QueuePosition,
		Options: make([]ChoiceOption, len(effect.Choice.Options)), Revision: state.Revision + 1,
	}
	for index, option := range effect.Choice.Options {
		choice.Options[index] = ChoiceOption{ID: option.ID, Objects: append([]ObjectRef(nil), option.Objects...), Effects: cloneResolvedEffects(option.Effects)}
	}
	state.PendingChoices = append(state.PendingChoices, choice)
	return emitEffectEvent(state, budget, "choice_requested", &effect.Controller, choice.Kind)
}

func handleScheduleDelayed(state *State, effect ResolvedEffect, budget *eventBudget) error {
	if effect.Trigger == nil {
		return errors.New("delayed effect has no resolved definition")
	}
	state.NextEffectSequence++
	trigger := DelayedTriggerState{
		TriggerID: fmt.Sprintf("trigger-%d", state.NextEffectSequence), CreatedSequence: state.NextEffectSequence,
		Controller: effect.Controller, SourceRef: cloneObjectRef(effect.SourceRef), EventKind: effect.Trigger.EventKind,
		Once: effect.Trigger.Once, Duration: effect.Trigger.Duration, CreatedTurn: state.TurnNumber,
		Effects: cloneResolvedEffects(effect.Trigger.Effects),
	}
	state.DelayedTriggers = append(state.DelayedTriggers, trigger)
	return emitEffectEvent(state, budget, "delayed_trigger_scheduled", &effect.Controller, trigger.EventKind)
}

func handleInstallReplacement(state *State, effect ResolvedEffect, budget *eventBudget) error {
	if effect.Replacement == nil {
		return errors.New("replacement effect has no resolved definition")
	}
	state.NextEffectSequence++
	replacement := ReplacementState{
		ReplacementID: fmt.Sprintf("replacement-%d", state.NextEffectSequence), CreatedSequence: state.NextEffectSequence,
		Controller: effect.Controller, SourceRef: cloneObjectRef(effect.SourceRef), TargetRef: cloneObjectRef(effect.TargetRef),
		EffectKind: effect.Replacement.EffectKind, Once: effect.Replacement.Once,
		Duration: effect.Replacement.Duration, CreatedTurn: state.TurnNumber,
		Effects: cloneResolvedEffects(effect.Replacement.Effects),
	}
	state.Replacements = append(state.Replacements, replacement)
	return emitEffectEvent(state, budget, "replacement_installed", &effect.Controller, replacement.ReplacementID)
}

func emitEffectEvent(state *State, budget *eventBudget, kind string, seat *Seat, detail string) error {
	if err := budget.emit(kind, seat, detail); err != nil {
		return err
	}
	remaining := state.DelayedTriggers[:0]
	for _, trigger := range state.DelayedTriggers {
		if !durationSourceValid(state, trigger.Duration, trigger.SourceRef) {
			continue
		}
		if trigger.EventKind == kind {
			enqueueResolvedEffects(state, trigger.TriggerID+":fired", trigger.Effects, QueueAfterExisting)
			if trigger.Once {
				continue
			}
		}
		remaining = append(remaining, trigger)
	}
	state.DelayedTriggers = remaining
	return nil
}

func applySubmitChoice(state *State, command Command, budget *eventBudget) error {
	choiceIndex := -1
	for index := range state.PendingChoices {
		if state.PendingChoices[index].ChoiceID == command.ChoiceID {
			choiceIndex = index
			break
		}
	}
	if choiceIndex < 0 {
		return fmt.Errorf("%w: pending choice was not found", ErrInvalidCommand)
	}
	choice := state.PendingChoices[choiceIndex]
	if choice.Seat != command.Seat || len(command.OptionIDs) < choice.MinSelections || len(command.OptionIDs) > choice.MaxSelections {
		return fmt.Errorf("%w: choice recipient or selection count is invalid", ErrInvalidCommand)
	}
	byID := make(map[string]ChoiceOption, len(choice.Options))
	for _, option := range choice.Options {
		byID[option.ID] = option
	}
	seen := make(map[string]struct{}, len(command.OptionIDs))
	selected := make([]ChoiceOption, 0, len(command.OptionIDs))
	for _, id := range command.OptionIDs {
		option, exists := byID[id]
		if !exists {
			return fmt.Errorf("%w: choice option %q is not legal", ErrInvalidCommand, id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: choice option %q is duplicated", ErrInvalidCommand, id)
		}
		seen[id] = struct{}{}
		selected = append(selected, option)
	}
	if !choice.OrderMatters {
		sort.Slice(selected, func(left, right int) bool { return selected[left].ID < selected[right].ID })
	}
	state.PendingChoices = append(state.PendingChoices[:choiceIndex], state.PendingChoices[choiceIndex+1:]...)
	selectedEffects := make([]ResolvedEffect, 0)
	for _, option := range selected {
		selectedEffects = append(selectedEffects, cloneResolvedEffects(option.Effects)...)
	}
	enqueueResolvedEffects(state, choice.ChoiceID+":selection", selectedEffects, choice.QueuePosition)
	seat := command.Seat
	if err := emitEffectEvent(state, budget, "choice_submitted", &seat, choice.Kind); err != nil {
		return err
	}
	if err := resolveEffectQueue(state, budget); err != nil {
		return err
	}
	return finishActionResolution(state, budget)
}

func finishActionResolution(state *State, budget *eventBudget) error {
	if state.ActionResolution == nil || len(state.PendingChoices) > 0 || len(state.EffectQueue) > 0 {
		return nil
	}
	state.ActionResolution = nil
	return advanceActionTurn(state, budget)
}

func recalculatePower(state *State, target ObjectRef) {
	object, err := state.findObject(target)
	if err != nil || object.Zone != ZoneBoard || object.Slot != SlotUnit {
		return
	}
	modifiers := append([]ModifierState(nil), state.Modifiers...)
	sort.SliceStable(modifiers, func(left, right int) bool {
		if modifiers[left].Layer != modifiers[right].Layer {
			return modifiers[left].Layer < modifiers[right].Layer
		}
		return modifiers[left].CreatedSequence < modifiers[right].CreatedSequence
	})
	previousPower := object.Power
	basePower := object.BasePower
	for _, modifier := range modifiers {
		if modifier.TargetRef == target && modifier.BasePowerSet != nil && durationSourceValid(state, modifier.Duration, modifier.SourceRef) {
			basePower = *modifier.BasePowerSet
		}
	}
	object.Power = basePower
	for _, modifier := range modifiers {
		if modifier.TargetRef == target && durationSourceValid(state, modifier.Duration, modifier.SourceRef) {
			if modifier.PowerDelta < 0 && objectHasKeyword(state, target, KeywordResilience) {
				continue
			}
			object.Power += modifier.PowerDelta
		}
	}
	if objectHasKeyword(state, target, KeywordResilience) && object.Power < previousPower {
		object.Power = previousPower
	}
	if modifierPreventsPowerIncrease(state, target) && object.Power > basePower {
		object.Power = basePower
	}
	if object.Power <= 0 {
		moveToGraveyard(state, object)
		pruneStaleEffectState(state)
	}
}

func objectHasKeyword(state *State, target ObjectRef, keyword Keyword) bool {
	object, err := state.findObject(target)
	if err != nil {
		return false
	}
	suppressed := object.BaseAbilitiesSuppressed || modifierBool(state, target, func(modifier ModifierState) bool { return modifier.SuppressBaseAbilities })
	if !suppressed {
		for _, current := range object.Keywords {
			if current == keyword {
				return true
			}
		}
	}
	for _, modifier := range state.Modifiers {
		if modifier.TargetRef == target && modifier.Keyword == keyword && durationSourceValid(state, modifier.Duration, modifier.SourceRef) {
			return true
		}
	}
	return false
}

func modifierPreventsPowerIncrease(state *State, target ObjectRef) bool {
	for _, modifier := range state.Modifiers {
		if modifier.TargetRef == target && modifier.CannotIncreasePower && durationSourceValid(state, modifier.Duration, modifier.SourceRef) {
			return true
		}
	}
	return false
}

func modifierInt(state *State, target ObjectRef, selectValue func(ModifierState) int) int {
	total := 0
	for _, modifier := range state.Modifiers {
		if modifier.TargetRef == target && durationSourceValid(state, modifier.Duration, modifier.SourceRef) {
			total += selectValue(modifier)
		}
	}
	return total
}

func modifierBool(state *State, target ObjectRef, selectValue func(ModifierState) bool) bool {
	for _, modifier := range state.Modifiers {
		if modifier.TargetRef == target && durationSourceValid(state, modifier.Duration, modifier.SourceRef) && selectValue(modifier) {
			return true
		}
	}
	return false
}

func expireEffects(state *State, duration DurationKind) {
	modifiers := state.Modifiers[:0]
	targets := make([]ObjectRef, 0, len(state.Modifiers))
	for _, modifier := range state.Modifiers {
		targets = append(targets, modifier.TargetRef)
		if modifier.Duration != duration && durationSourceValid(state, modifier.Duration, modifier.SourceRef) {
			modifiers = append(modifiers, modifier)
		}
	}
	state.Modifiers = modifiers
	triggers := state.DelayedTriggers[:0]
	for _, trigger := range state.DelayedTriggers {
		if trigger.Duration != duration && durationSourceValid(state, trigger.Duration, trigger.SourceRef) {
			triggers = append(triggers, trigger)
		}
	}
	state.DelayedTriggers = triggers
	replacements := state.Replacements[:0]
	for _, replacement := range state.Replacements {
		if replacement.Duration != duration && durationSourceValid(state, replacement.Duration, replacement.SourceRef) {
			replacements = append(replacements, replacement)
		}
	}
	state.Replacements = replacements
	for _, target := range targets {
		recalculatePower(state, target)
	}
}

func pruneStaleEffectState(state *State) {
	modifiers := state.Modifiers[:0]
	removedTargets := make([]ObjectRef, 0)
	for _, modifier := range state.Modifiers {
		if durationSourceValid(state, modifier.Duration, modifier.SourceRef) {
			if _, err := state.findObject(modifier.TargetRef); err == nil {
				modifiers = append(modifiers, modifier)
				continue
			}
		}
		removedTargets = append(removedTargets, modifier.TargetRef)
	}
	state.Modifiers = modifiers
	triggers := state.DelayedTriggers[:0]
	for _, trigger := range state.DelayedTriggers {
		if durationSourceValid(state, trigger.Duration, trigger.SourceRef) {
			triggers = append(triggers, trigger)
		}
	}
	state.DelayedTriggers = triggers
	replacements := state.Replacements[:0]
	for _, replacement := range state.Replacements {
		if durationSourceValid(state, replacement.Duration, replacement.SourceRef) {
			replacements = append(replacements, replacement)
		}
	}
	state.Replacements = replacements
	for _, target := range removedTargets {
		recalculatePower(state, target)
	}
}

func sourceStillValid(state *State, source *ObjectRef) bool {
	if source == nil {
		return true
	}
	_, err := state.findObject(*source)
	return err == nil
}

func durationSourceValid(state *State, duration DurationKind, source *ObjectRef) bool {
	return duration != DurationWhileSourceValid || sourceStillValid(state, source)
}
