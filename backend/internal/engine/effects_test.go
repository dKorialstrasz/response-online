package engine

import (
	"errors"
	"fmt"
	"testing"
)

func TestAbilityActivationValidatesModesTargetsAndPaymentAtomically(t *testing.T) {
	config := testConfig()
	config.Abilities = []AbilityDefinition{boostAbility("boost", true)}
	match := preparedMatch(t, config)
	state := match.State()
	actor := state.ActingSeat
	target := addBoardObject(&state, actor, "boost-target", 2, 0, SlotUnit)
	match.state = state

	before := match.State()
	before.player(actor).Influence = 1
	match.state = before
	bad := Command{
		ID: "boost-no-payment", Kind: CommandActivateAbility, Seat: actor,
		ExpectedRevision: before.Revision, AbilityID: "boost", ModeIDs: []string{"plus-two"},
		TargetRefs: []ObjectRef{target.Ref()},
	}
	beforeHash, _ := StateHash(before)
	if err := match.Handle(bad); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("insufficient payment error = %v", err)
	}
	afterHash, _ := StateHash(match.State())
	if afterHash != beforeHash {
		t.Fatalf("failed payment changed state: before=%s after=%s", beforeHash, afterHash)
	}

	state = match.State()
	state.player(actor).Influence = 5
	match.state = state
	command := bad
	command.ID = "boost-paid"
	if err := match.Handle(command); err != nil {
		t.Fatal(err)
	}
	state = match.State()
	modified, err := state.findObject(target.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if modified.Power != 4 || state.player(actor).Influence != 3 {
		t.Fatalf("power/influence = %d/%d, want 4/3", modified.Power, state.player(actor).Influence)
	}
	if state.player(actor).ActionsRemaining != ActionsPerPlayer-1 || state.ActingSeat != actor.Other() {
		t.Fatal("resolved ability did not consume one action and alternate the acting seat")
	}
	if len(state.Modifiers) != 1 || state.Modifiers[0].Duration != DurationUntilEndOfTurn {
		t.Fatalf("modifiers = %#v", state.Modifiers)
	}
}

func TestSacrificePaymentOrderIsCanonical(t *testing.T) {
	config := testConfig()
	config.Abilities = []AbilityDefinition{{
		AbilityID: "sacrifice", Status: AbilityReady,
		ConformanceScenarioIDs: []string{"sacrifice.conformance"},
		Targets:                TargetDefinition{Controller: TargetAny},
		Payment: PaymentDefinition{
			SacrificeCount: 2,
			SacrificeZones: []Zone{ZoneHand},
		},
	}}
	hashes := make([]string, 0, 2)
	for reversed := false; ; reversed = true {
		match := preparedMatch(t, config)
		state := match.State()
		payment := refs(state.objectsInZone(state.ActingSeat, ZoneHand)[:2])
		if reversed {
			payment[0], payment[1] = payment[1], payment[0]
		}
		if err := match.Handle(Command{
			ID: "sacrifice-payment", Kind: CommandActivateAbility, Seat: state.ActingSeat,
			ExpectedRevision: state.Revision, AbilityID: "sacrifice", PaymentRefs: payment,
		}); err != nil {
			t.Fatal(err)
		}
		result := match.State()
		if len(result.objectsInZone(state.ActingSeat, ZoneGraveyard)) != 2 {
			t.Fatal("sacrifice payment did not move both objects")
		}
		hash, err := StateHash(result)
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, hash)
		if reversed {
			break
		}
	}
	if hashes[0] != hashes[1] {
		t.Fatalf("payment input order changed state hash: %v", hashes)
	}
}

func TestPendingChoiceCommandsReplayDeterministically(t *testing.T) {
	config := testConfig()
	config.Abilities = []AbilityDefinition{{
		AbilityID: "replay-choice", Status: AbilityReady,
		ConformanceScenarioIDs: []string{"replay-choice.conformance"},
		Targets:                TargetDefinition{Controller: TargetAny},
		Effects: []EffectInstruction{{
			Kind: EffectRequestChoice,
			Choice: &ChoiceDefinition{
				Kind: "confirm", Mandatory: false, Visibility: ChoicePublic,
				MinSelections: 0, MaxSelections: 1, QueuePosition: QueueAfterExisting,
				Options: []ChoiceOptionDefinition{{ID: "confirm"}},
			},
		}},
		ConsumesAction: true,
	}}
	commands := []Command{
		command("orders-a", CommandArrangeOrders, SeatA, 0, testOrders(SeatA), nil),
		command("orders-b", CommandArrangeOrders, SeatB, 1, testOrders(SeatB), nil),
		command("mulligan-a", CommandMulligan, SeatA, 2, nil, nil),
		command("mulligan-b", CommandMulligan, SeatB, 3, nil, nil),
	}
	match, err := NewMatch(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range commands {
		if err := match.Handle(current); err != nil {
			t.Fatal(err)
		}
	}
	state := match.State()
	activate := Command{
		ID: "activate-choice", Kind: CommandActivateAbility, Seat: state.ActingSeat,
		ExpectedRevision: state.Revision, AbilityID: "replay-choice",
	}
	if err := match.Handle(activate); err != nil {
		t.Fatal(err)
	}
	commands = append(commands, activate)
	state = match.State()
	submit := Command{
		ID: "submit-empty-choice", Kind: CommandSubmitChoice, Seat: state.PendingChoices[0].Seat,
		ExpectedRevision: state.Revision, ChoiceID: state.PendingChoices[0].ChoiceID,
	}
	if err := match.Handle(submit); err != nil {
		t.Fatal(err)
	}
	commands = append(commands, submit)
	live := match.State()
	replayed, replayHash, err := Replay(config, commands)
	if err != nil {
		t.Fatal(err)
	}
	liveHash, _ := StateHash(live)
	if replayHash != liveHash || len(replayed.PendingChoices) != 0 || replayed.ActionResolution != nil {
		t.Fatalf("effect replay mismatch live=%s replay=%s state=%+v", liveHash, replayHash, replayed)
	}
}

func TestMandatoryAndOptionalChoicesPersistWithPrivateProjection(t *testing.T) {
	config := testConfig()
	config.Abilities = []AbilityDefinition{
		choiceAbility("mandatory-choice", true, true),
		choiceAbility("optional-choice", false, false),
	}
	match := preparedMatch(t, config)
	state := match.State()
	actor := state.ActingSeat
	target := addBoardObject(&state, actor, "choice-target", 2, 1, SlotUnit)
	match.state = state

	if err := match.Handle(Command{
		ID: "request-mandatory", Kind: CommandActivateAbility, Seat: actor,
		ExpectedRevision: state.Revision, AbilityID: "mandatory-choice", TargetRefs: []ObjectRef{target.Ref()},
	}); err != nil {
		t.Fatal(err)
	}
	state = match.State()
	if len(state.PendingChoices) != 1 || state.ActionResolution == nil || state.ActingSeat != actor {
		t.Fatalf("pending action state = choices=%d action=%+v actor=%d", len(state.PendingChoices), state.ActionResolution, state.ActingSeat)
	}
	ownerView := state.PendingChoiceProjections(actor)
	opponentView := state.PendingChoiceProjections(actor.Other())
	if len(ownerView) != 1 || !ownerView[0].Private || len(ownerView[0].Options) != 2 {
		t.Fatalf("owner projection = %#v", ownerView)
	}
	if len(opponentView) != 1 || !opponentView[0].Redacted || opponentView[0].ChoiceID != "" || len(opponentView[0].Options) != 0 {
		t.Fatalf("opponent projection leaks private choice: %#v", opponentView)
	}

	choice := state.PendingChoices[0]
	beforeHash, _ := StateHash(state)
	if err := match.Handle(Command{
		ID: "choice-by-opponent", Kind: CommandSubmitChoice, Seat: actor.Other(),
		ExpectedRevision: state.Revision, ChoiceID: choice.ChoiceID, OptionIDs: []string{"gain-one"},
	}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("opponent choice error = %v", err)
	}
	afterHash, _ := StateHash(match.State())
	if afterHash != beforeHash {
		t.Fatal("invalid choice changed the transaction")
	}

	if err := match.Handle(Command{
		ID: "choice-by-owner", Kind: CommandSubmitChoice, Seat: actor,
		ExpectedRevision: state.Revision, ChoiceID: choice.ChoiceID, OptionIDs: []string{"gain-one"},
	}); err != nil {
		t.Fatal(err)
	}
	state = match.State()
	modified, err := state.findObject(target.Ref())
	if err != nil || modified.Power != 3 || len(state.PendingChoices) != 0 || state.ActionResolution != nil {
		t.Fatalf("resolved mandatory choice: object=%+v choices=%d action=%+v err=%v", modified, len(state.PendingChoices), state.ActionResolution, err)
	}

	// Optional activation does not consume an action and accepts an explicit empty
	// selection without inventing an implicit option.
	actor = state.ActingSeat
	if err := match.Handle(Command{
		ID: "request-optional", Kind: CommandActivateAbility, Seat: actor,
		ExpectedRevision: state.Revision, AbilityID: "optional-choice", TargetRefs: []ObjectRef{target.Ref()},
	}); err != nil {
		t.Fatal(err)
	}
	state = match.State()
	optional := state.PendingChoices[0]
	if optional.Mandatory || optional.MinSelections != 0 {
		t.Fatalf("optional choice = %#v", optional)
	}
	if err := match.Handle(Command{
		ID: "decline-optional", Kind: CommandSubmitChoice, Seat: actor,
		ExpectedRevision: state.Revision, ChoiceID: optional.ChoiceID,
	}); err != nil {
		t.Fatal(err)
	}
	if got := match.State(); len(got.PendingChoices) != 0 || got.ActingSeat != actor {
		t.Fatalf("optional decline changed action ownership: choices=%d actor=%d", len(got.PendingChoices), got.ActingSeat)
	}
}

func TestExplicitQueueOrderHandlesBothPlayersAndNewEffectsWithoutLIFO(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	target := addBoardObject(&state, SeatA, "ordered-target", 1, 2, SlotUnit)
	targetRef := target.Ref()
	effects := []ResolvedEffect{
		powerEffect(SeatA, targetRef, 1, DurationPermanent),
		{
			Kind: EffectEnqueue, Controller: SeatB, QueuePosition: QueueAfterExisting,
			Effects: []ResolvedEffect{powerEffect(SeatB, targetRef, 10, DurationPermanent)},
		},
		powerEffect(SeatB, targetRef, 100, DurationPermanent),
	}
	enqueueResolvedEffects(&state, "simultaneous-explicit-order", effects, QueueAfterExisting)
	budget := eventBudget{state: &state, limit: DefaultInternalEventCap}
	if err := resolveEffectQueue(&state, &budget); err != nil {
		t.Fatal(err)
	}
	object, err := state.findObject(targetRef)
	if err != nil || object.Power != 112 {
		t.Fatalf("ordered result object=%+v err=%v", object, err)
	}
	details := make([]string, 0, 4)
	for _, event := range state.Events {
		if event.Kind == "power_modified" || event.Kind == "effects_enqueued" {
			details = append(details, event.Kind+":"+event.Detail)
		}
	}
	want := []string{
		"power_modified:delta=1",
		"effects_enqueued:after_existing",
		"power_modified:delta=100",
		"power_modified:delta=10",
	}
	if fmt.Sprint(details) != fmt.Sprint(want) {
		t.Fatalf("queue order = %v, want %v", details, want)
	}
	hash, err := StateHash(state)
	if err != nil {
		t.Fatal(err)
	}
	const golden = "e6424a7046f888e4eed81bd8f42794df0c81cf1a377ae0f57dfd45baed4b1a28"
	if hash != golden {
		t.Fatalf("effect queue golden hash = %s, want %s", hash, golden)
	}
}

func TestReplacementDelayedTriggerDurationsAndStaleTargets(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	target := addBoardObject(&state, SeatA, "replacement-target", 1, 3, SlotUnit)
	targetRef := target.Ref()
	replacementEffect := ResolvedEffect{
		Kind: EffectInstallReplacement, Controller: SeatA, TargetRef: cloneObjectRef(&targetRef),
		Replacement: &ResolvedReplacementDefinition{
			EffectKind: EffectMoveZone, Once: true, Duration: DurationUntilEndOfTurn,
			Effects: []ResolvedEffect{powerEffect(SeatA, targetRef, 3, DurationPermanent)},
		},
	}
	delayedEffect := ResolvedEffect{
		Kind: EffectScheduleDelayed, Controller: SeatB,
		Trigger: &ResolvedDelayedDefinition{
			EventKind: "power_modified", Once: true, Duration: DurationUntilEndOfTurn,
			Effects: []ResolvedEffect{powerEffect(SeatB, targetRef, 4, DurationUntilEndOfCombat)},
		},
	}
	move := ResolvedEffect{Kind: EffectMoveZone, Controller: SeatB, TargetRef: cloneObjectRef(&targetRef), Destination: ZoneGraveyard}
	enqueueResolvedEffects(&state, "replacement-delayed", []ResolvedEffect{replacementEffect, delayedEffect, move}, QueueAfterExisting)
	budget := eventBudget{state: &state, limit: DefaultInternalEventCap}
	if err := resolveEffectQueue(&state, &budget); err != nil {
		t.Fatal(err)
	}
	object, err := state.findObject(targetRef)
	if err != nil || object.Zone != ZoneBoard || object.Power != 8 {
		t.Fatalf("replacement/delayed result object=%+v err=%v", object, err)
	}
	if len(state.Replacements) != 0 || len(state.DelayedTriggers) != 0 {
		t.Fatalf("one-shot state remained: replacements=%d triggers=%d", len(state.Replacements), len(state.DelayedTriggers))
	}
	expireEffects(&state, DurationUntilEndOfCombat)
	object, _ = state.findObject(targetRef)
	if object.Power != 4 {
		t.Fatalf("end-of-combat duration left power=%d, want 4", object.Power)
	}
	source := addBoardObject(&state, SeatA, "duration-source", 1, 4, SlotUnit)
	sourceRef := source.Ref()
	whileSource := powerEffect(SeatA, targetRef, 2, DurationWhileSourceValid)
	whileSource.SourceRef = cloneObjectRef(&sourceRef)
	enqueueResolvedEffects(&state, "while-source", []ResolvedEffect{whileSource}, QueueAfterExisting)
	if err := resolveEffectQueue(&state, &budget); err != nil {
		t.Fatal(err)
	}
	object, _ = state.findObject(targetRef)
	if object.Power != 6 {
		t.Fatalf("while-source modifier power=%d, want 6", object.Power)
	}
	sourceMove := ResolvedEffect{Kind: EffectMoveZone, Controller: SeatA, TargetRef: cloneObjectRef(&sourceRef), Destination: ZoneGraveyard}
	enqueueResolvedEffects(&state, "invalidate-source", []ResolvedEffect{sourceMove}, QueueAfterExisting)
	if err := resolveEffectQueue(&state, &budget); err != nil {
		t.Fatal(err)
	}
	object, _ = state.findObject(targetRef)
	if object.Power != 4 {
		t.Fatalf("invalid source retained modifier power=%d, want 4", object.Power)
	}

	firstMove := ResolvedEffect{Kind: EffectMoveZone, Controller: SeatA, TargetRef: cloneObjectRef(&targetRef), Destination: ZoneHand}
	staleMove := ResolvedEffect{Kind: EffectMoveZone, Controller: SeatA, TargetRef: cloneObjectRef(&targetRef), Destination: ZoneVoid}
	enqueueResolvedEffects(&state, "stale-generation", []ResolvedEffect{firstMove, staleMove}, QueueAfterExisting)
	if err := resolveEffectQueue(&state, &budget); err != nil {
		t.Fatal(err)
	}
	var moved *CardObject
	for index := range state.player(SeatA).Objects {
		if state.player(SeatA).Objects[index].InstanceID == "replacement-target" {
			moved = &state.player(SeatA).Objects[index]
			break
		}
	}
	if moved == nil || moved.Zone != ZoneHand || moved.Generation != targetRef.Generation+1 {
		t.Fatalf("stale second move affected new generation: %+v", moved)
	}
	if !hasEvent(state.Events, "effect_target_stale") {
		t.Fatal("stale target did not emit a deterministic fizzle event")
	}
}

func TestBlockedAbilityAndCatalogEvidenceAreEnforced(t *testing.T) {
	config := testConfig()
	invalid := boostAbility("missing-evidence", false)
	invalid.ConformanceScenarioIDs = nil
	config.Abilities = []AbilityDefinition{invalid}
	if _, err := NewMatch(config); err == nil {
		t.Fatal("ability without a conformance scenario was accepted")
	}
	invalid = boostAbility("missing-source", false)
	invalid.Modes[0].Effects[0].Duration = DurationWhileSourceValid
	config.Abilities = []AbilityDefinition{invalid}
	if _, err := NewMatch(config); err == nil {
		t.Fatal("while-source duration without a required source was accepted")
	}

	blocked := boostAbility("blocked", false)
	blocked.Status = AbilityBlocked
	blocked.BlockingDecisionRef = "OQ-02: owner ruling required"
	config.Abilities = []AbilityDefinition{blocked}
	match := preparedMatch(t, config)
	state := match.State()
	target := addBoardObject(&state, state.ActingSeat, "blocked-target", 1, 4, SlotUnit)
	match.state = state
	before, _ := StateHash(state)
	err := match.Handle(Command{
		ID: "blocked-attempt", Kind: CommandActivateAbility, Seat: state.ActingSeat,
		ExpectedRevision: state.Revision, AbilityID: "blocked", ModeIDs: []string{"plus-two"},
		TargetRefs: []ObjectRef{target.Ref()},
	})
	if !errors.Is(err, ErrAbilityBlocked) {
		t.Fatalf("blocked ability error = %v", err)
	}
	after, _ := StateHash(match.State())
	if before != after {
		t.Fatal("blocked ability mutated the state")
	}
}

func boostAbility(id string, consumesAction bool) AbilityDefinition {
	return AbilityDefinition{
		AbilityID: id, Status: AbilityReady, ConformanceScenarioIDs: []string{id + ".conformance"},
		MinModes: 1, MaxModes: 1,
		Modes: []ModeDefinition{{
			ID: "plus-two",
			Effects: []EffectInstruction{{
				Kind: EffectModifyPower, TargetIndex: 0, Amount: 2, Layer: 20, Duration: DurationUntilEndOfTurn,
			}},
		}},
		Targets: TargetDefinition{Min: 1, Max: 1, Zones: []Zone{ZoneBoard}, Controller: TargetAny},
		Payment: PaymentDefinition{Influence: 2}, ConsumesAction: consumesAction,
	}
}

func choiceAbility(id string, mandatory, consumesAction bool) AbilityDefinition {
	minimum := 0
	if mandatory {
		minimum = 1
	}
	return AbilityDefinition{
		AbilityID: id, Status: AbilityReady, ConformanceScenarioIDs: []string{id + ".conformance"},
		Targets: TargetDefinition{Min: 1, Max: 1, Zones: []Zone{ZoneBoard}, Controller: TargetAny},
		Effects: []EffectInstruction{{
			Kind: EffectRequestChoice,
			Choice: &ChoiceDefinition{
				Kind: "mode", Mandatory: mandatory, Visibility: ChoiceOwnerOnly,
				MinSelections: minimum, MaxSelections: 1, QueuePosition: QueueBeforeExisting,
				Options: []ChoiceOptionDefinition{
					{ID: "gain-one", TargetIndexes: []int{0}, Effects: []EffectInstruction{{Kind: EffectModifyPower, TargetIndex: 0, Amount: 1, Layer: 10, Duration: DurationPermanent}}},
					{ID: "gain-two", TargetIndexes: []int{0}, Effects: []EffectInstruction{{Kind: EffectModifyPower, TargetIndex: 0, Amount: 2, Layer: 10, Duration: DurationPermanent}}},
				},
			},
		}},
		ConsumesAction: consumesAction,
	}
}

func powerEffect(controller Seat, target ObjectRef, amount int, duration DurationKind) ResolvedEffect {
	return ResolvedEffect{
		Kind: EffectModifyPower, Controller: controller, TargetRef: cloneObjectRef(&target),
		Amount: amount, Layer: 10, Duration: duration,
	}
}

func hasEvent(events []Event, kind string) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}
