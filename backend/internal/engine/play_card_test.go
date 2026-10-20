package engine

import (
	"errors"
	"testing"
)

func TestPlayCardValidatesPinnedCostTypePlacementGenerationAndAction(t *testing.T) {
	cost := 2
	power := 2
	config := testConfig()
	config.Cards = []CardDefinition{{CardID: "test-unit", Name: "Проверочный юнит", Types: []CardType{CardUnit}, Cost: &cost, BasePower: &power}}
	for seat := range config.Players {
		for index := range config.Players[seat].DeckCardIDs {
			config.Players[seat].DeckCardIDs[index] = "test-unit"
		}
	}
	match := preparedMatch(t, config)
	before := match.State()
	actor := before.ActingSeat
	source := before.objectsInZone(actor, ZoneHand)[0].Ref()
	command := Command{
		ID: "play-unit", Kind: CommandPlayCard, Seat: actor, ExpectedRevision: before.Revision,
		SourceRef: &source, DeclaredType: CardUnit, DestinationLane: 2, DestinationLaneSet: true, DestinationSlot: SlotUnit,
	}
	if err := match.Handle(command); err != nil {
		t.Fatal(err)
	}
	after := match.State()
	if after.Revision != before.Revision+1 || after.ActingSeat != actor.Other() || after.player(actor).ActionsRemaining != before.player(actor).ActionsRemaining-1 {
		t.Fatalf("play lifecycle = revision:%d acting:%d actions:%d", after.Revision, after.ActingSeat, after.player(actor).ActionsRemaining)
	}
	if after.player(actor).Influence != before.player(actor).Influence-cost {
		t.Fatalf("influence = %d", after.player(actor).Influence)
	}
	placed := after.boardObject(actor, 2, SlotUnit)
	if placed == nil || placed.DefinitionID != "test-unit" || placed.Generation != source.Generation+1 {
		t.Fatalf("placed object = %+v", placed)
	}
	if _, err := after.findObject(source); err == nil {
		t.Fatal("pre-play generation remained valid")
	}

	assertRejectedWithoutMutation(t, match, Command{
		ID: "stale-source", Kind: CommandPlayCard, Seat: after.ActingSeat, ExpectedRevision: after.Revision,
		SourceRef: &source, DeclaredType: CardUnit, DestinationLane: 1, DestinationLaneSet: true, DestinationSlot: SlotUnit,
	})
	duplicate := command
	duplicate.ExpectedRevision = after.Revision
	if err := match.Handle(duplicate); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestPlayCardRunsOnlyExactPlayWindowsNotFirstAbility(t *testing.T) {
	cost := 1
	power := 1
	config := testConfig()
	config.Abilities = []AbilityDefinition{
		{AbilityID: "baz-003.front.ability", Status: AbilityReady, BuiltinHandler: "baz-003.front.ability", ConformanceScenarioIDs: []string{"triggered"}},
		{AbilityID: "baz-113.front.ability", Status: AbilityReady, BuiltinHandler: "baz-113.front.ability", ConformanceScenarioIDs: []string{"summon"}},
	}
	config.Cards = []CardDefinition{{
		CardID: "window-unit", Name: "Окна", Types: []CardType{CardUnit}, Cost: &cost,
		BasePower:  &power,
		AbilityIDs: []string{"baz-003.front.ability", "baz-113.front.ability"},
	}}
	for seat := range config.Players {
		for index := range config.Players[seat].DeckCardIDs {
			config.Players[seat].DeckCardIDs[index] = "window-unit"
		}
	}
	match := preparedMatch(t, config)
	before := match.State()
	actor := before.ActingSeat
	source := before.objectsInZone(actor, ZoneHand)[0].Ref()
	if err := match.Handle(Command{
		ID: "window-play", Kind: CommandPlayCard, Seat: actor, ExpectedRevision: before.Revision,
		SourceRef: &source, DeclaredType: CardUnit, DestinationLane: 0, DestinationLaneSet: true, DestinationSlot: SlotUnit,
	}); err != nil {
		t.Fatal(err)
	}
	after := match.State()
	if got, want := len(after.objectsInZone(actor, ZoneHand)), len(before.objectsInZone(actor, ZoneHand)); got != want {
		t.Fatalf("Summon draw count produced hand=%d, want %d", got, want)
	}
	if after.player(actor).Influence != before.player(actor).Influence-cost {
		t.Fatalf("triggered ability ran during play: influence=%d", after.player(actor).Influence)
	}
}

func TestBaz121DrawThenMandatoryCurrentHandChoiceReplayAndPrivacy(t *testing.T) {
	match, config := preparedBaseSpellMatch(t, "baz-121", "baz-121.front.ability")
	before := match.State()
	actor := before.ActingSeat
	source := before.objectsInZone(actor, ZoneHand)[0].Ref()
	play := Command{
		ID: "play-baz-121", Kind: CommandPlayCard, Seat: actor, ExpectedRevision: before.Revision,
		SourceRef: &source, DeclaredType: CardSpell,
	}
	if err := match.Handle(play); err != nil {
		t.Fatal(err)
	}
	waiting := match.State()
	if len(waiting.PendingChoices) != 1 || waiting.PendingChoices[0].Seat != actor || !waiting.PendingChoices[0].Mandatory || waiting.PendingChoices[0].MinSelections != 1 || waiting.PendingChoices[0].MaxSelections != 1 {
		t.Fatalf("baz-121 choice = %+v", waiting.PendingChoices)
	}
	if got := len(waiting.PendingChoices[0].Options); got != len(waiting.objectsInZone(actor, ZoneHand)) || got != 8 {
		t.Fatalf("post-draw options=%d hand=%d", got, len(waiting.objectsInZone(actor, ZoneHand)))
	}
	owner := waiting.PendingChoiceProjections(actor)[0]
	opponent := waiting.PendingChoiceProjections(actor.Other())[0]
	if len(owner.Options) != 8 || owner.ChoiceID == "" || !owner.Private || owner.Mandatory == nil || !*owner.Mandatory ||
		opponent.ChoiceID != "" || opponent.Kind != "" || opponent.Mandatory != nil || opponent.MinSelections != nil ||
		opponent.MaxSelections != nil || opponent.OrderMatters != nil || len(opponent.Options) != 0 || !opponent.Redacted {
		t.Fatalf("choice privacy owner=%+v opponent=%+v", owner, opponent)
	}
	assertRejectedWithoutMutation(t, match, Command{
		ID: "foreign-option", Kind: CommandSubmitChoice, Seat: actor, ExpectedRevision: waiting.Revision,
		ChoiceID: waiting.PendingChoices[0].ChoiceID, OptionIDs: []string{"option-foreign"},
	})
	choice := match.State().PendingChoices[0]
	selectedRef := choice.Options[len(choice.Options)-1].Objects[0]
	selectedObject, err := waiting.findObject(selectedRef)
	if err != nil {
		t.Fatal(err)
	}
	selectedInstanceID := selectedObject.InstanceID
	submit := Command{
		ID: "submit-baz-121", Kind: CommandSubmitChoice, Seat: actor, ExpectedRevision: waiting.Revision,
		ChoiceID: choice.ChoiceID, OptionIDs: []string{choice.Options[len(choice.Options)-1].ID},
	}
	if err := match.Handle(submit); err != nil {
		t.Fatal(err)
	}
	after := match.State()
	if len(after.PendingChoices) != 0 || after.ActionResolution != nil || after.ActingSeat != actor.Other() {
		t.Fatalf("resolved state = %+v", after.PendingChoices)
	}
	deck := after.objectsInZone(actor, ZoneDeck)
	if len(deck) == 0 || deck[0].InstanceID != selectedInstanceID || deck[0].Generation != selectedRef.Generation+1 {
		t.Fatalf("selected current hand card was not generation-bumped onto deck top: old=%+v top=%+v", selectedRef, deck[0])
	}
	commands := make([]Command, len(after.Commands))
	for index, record := range after.Commands {
		commands[index] = record.Command
	}
	replayed, replayHash, err := Replay(config, commands)
	if err != nil {
		t.Fatal(err)
	}
	liveHash, _ := StateHash(after)
	if replayed.Revision != after.Revision {
		t.Fatalf("replay revision=%d, want %d", replayed.Revision, after.Revision)
	}
	if liveHash != replayHash {
		t.Fatalf("replay hash mismatch: %s != %s", liveHash, replayHash)
	}
}

func TestBaz036ChoiceBelongsToOpponentAndNeverLeaksHandRefs(t *testing.T) {
	match, _ := preparedBaseSpellMatch(t, "baz-036", "baz-036.front.ability")
	before := match.State()
	actor := before.ActingSeat
	source := before.objectsInZone(actor, ZoneHand)[0].Ref()
	if err := match.Handle(Command{
		ID: "play-baz-036", Kind: CommandPlayCard, Seat: actor, ExpectedRevision: before.Revision,
		SourceRef: &source, DeclaredType: CardSpell,
	}); err != nil {
		t.Fatal(err)
	}
	waiting := match.State()
	if len(waiting.PendingChoices) != 1 || waiting.PendingChoices[0].Seat != actor.Other() {
		t.Fatalf("choice = %+v", waiting.PendingChoices)
	}
	actorView := waiting.PendingChoiceProjections(actor)[0]
	ownerView := waiting.PendingChoiceProjections(actor.Other())[0]
	if !actorView.Redacted || actorView.ChoiceID != "" || actorView.Kind != "" || len(actorView.Options) != 0 || len(ownerView.Options) != 7 {
		t.Fatalf("baz-036 privacy actor=%+v owner=%+v", actorView, ownerView)
	}
	assertRejectedWithoutMutation(t, match, Command{
		ID: "activator-cannot-answer", Kind: CommandSubmitChoice, Seat: actor, ExpectedRevision: waiting.Revision,
		ChoiceID: waiting.PendingChoices[0].ChoiceID, OptionIDs: []string{waiting.PendingChoices[0].Options[0].ID},
	})
	choice := match.State().PendingChoices[0]
	if err := match.Handle(Command{
		ID: "opponent-answer", Kind: CommandSubmitChoice, Seat: actor.Other(), ExpectedRevision: waiting.Revision,
		ChoiceID: choice.ChoiceID, OptionIDs: []string{choice.Options[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	if got := match.State(); len(got.objectsInZone(actor.Other(), ZoneHand)) != 6 || len(got.PendingChoices) != 0 {
		t.Fatalf("opponent decision did not resolve: hand=%d choices=%d", len(got.objectsInZone(actor.Other(), ZoneHand)), len(got.PendingChoices))
	}
}

func TestBaz102OptionalPostRevealChoiceCanDeclineAndSurvivesResyncSnapshot(t *testing.T) {
	match, _ := preparedBaseSpellMatch(t, "baz-102", "baz-102.front.ability")
	state := match.State()
	actor := state.ActingSeat
	power := 2
	cost := 2
	match.cards["revealed-unit"] = CardDefinition{CardID: "revealed-unit", Name: "Раскрытый юнит", Types: []CardType{CardUnit}, Cost: &cost, BasePower: &power}
	for _, object := range state.objectsInZone(actor, ZoneDeck)[:4] {
		object.DefinitionID = "revealed-unit"
	}
	match.state = state
	source := state.objectsInZone(actor, ZoneHand)[0].Ref()
	if err := match.Handle(Command{ID: "play-baz-102", Kind: CommandPlayCard, Seat: actor, ExpectedRevision: state.Revision, SourceRef: &source, DeclaredType: CardSpell}); err != nil {
		t.Fatal(err)
	}
	waiting := match.State()
	if len(waiting.PendingChoices) != 1 || waiting.PendingChoices[0].Mandatory || waiting.PendingChoices[0].MinSelections != 0 || waiting.PendingChoices[0].MaxSelections != 1 || waiting.PendingChoices[0].Visibility != ChoicePublic {
		t.Fatalf("optional revealed choice = %+v", waiting.PendingChoices)
	}
	resynced := match.State()
	if resynced.PendingChoices[0].ChoiceID != waiting.PendingChoices[0].ChoiceID || resynced.PendingChoices[0].Revision != waiting.Revision {
		t.Fatalf("choice changed across resync snapshot: before=%+v after=%+v", waiting.PendingChoices[0], resynced.PendingChoices[0])
	}
	choice := waiting.PendingChoices[0]
	if err := match.Handle(Command{ID: "decline-baz-102", Kind: CommandSubmitChoice, Seat: actor, ExpectedRevision: waiting.Revision, ChoiceID: choice.ChoiceID}); err != nil {
		t.Fatal(err)
	}
	after := match.State()
	if len(after.PendingChoices) != 0 || after.ActionResolution != nil || after.ActingSeat != actor.Other() {
		t.Fatalf("decline did not complete action: %+v", after.PendingChoices)
	}
}

func TestBaz009UnitPlacementCreatesOptionalMoveChoiceAndCanDecline(t *testing.T) {
	cost := 1
	power := 2
	config := testConfig()
	config.Versions.Protocol = "1.2"
	config.Versions.Engine = "base-abilities-v2"
	config.Abilities = []AbilityDefinition{{
		AbilityID: "baz-009.front.ability", Status: AbilityReady, BuiltinHandler: "baz-009.front.ability",
		ConformanceScenarioIDs: []string{"baz-009.front.ability.conformance"},
	}}
	config.Cards = []CardDefinition{{
		CardID: "baz-009", Name: "Патрульный энобот", Types: []CardType{CardUnit}, Cost: &cost,
		BasePower: &power, AbilityIDs: []string{"baz-009.front.ability"},
	}}
	for seat := range config.Players {
		for index := range config.Players[seat].DeckCardIDs {
			config.Players[seat].DeckCardIDs[index] = "baz-009"
		}
	}
	match := preparedMatch(t, config)
	before := match.State()
	actor := before.ActingSeat
	other := addBoardObject(&before, actor.Other(), "public-target", 1, 1, SlotUnit)
	match.state = before
	source := before.objectsInZone(actor, ZoneHand)[0].Ref()
	if err := match.Handle(Command{
		ID: "play-baz-009", Kind: CommandPlayCard, Seat: actor, ExpectedRevision: before.Revision,
		SourceRef: &source, DeclaredType: CardUnit, DestinationLane: 0, DestinationLaneSet: true, DestinationSlot: SlotUnit,
	}); err != nil {
		t.Fatal(err)
	}
	waiting := match.State()
	if len(waiting.PendingChoices) != 1 || waiting.PendingChoices[0].Mandatory || waiting.PendingChoices[0].MinSelections != 0 ||
		waiting.PendingChoices[0].MaxSelections != 1 || waiting.PendingChoices[0].Visibility != ChoicePublic {
		t.Fatalf("baz-009 optional move choice = %+v", waiting.PendingChoices)
	}
	choice := waiting.PendingChoices[0]
	foundTarget := false
	for _, option := range choice.Options {
		if len(option.Objects) == 1 && option.Objects[0] == other.Ref() {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		t.Fatalf("public board target absent from choice: %+v", choice.Options)
	}
	if err := match.Handle(Command{
		ID: "decline-baz-009", Kind: CommandSubmitChoice, Seat: actor, ExpectedRevision: waiting.Revision,
		ChoiceID: choice.ChoiceID,
	}); err != nil {
		t.Fatal(err)
	}
	after := match.State()
	if len(after.PendingChoices) != 0 || after.ActionResolution != nil || after.ActingSeat != actor.Other() {
		t.Fatalf("decline did not complete baz-009 action: choices=%+v acting=%d", after.PendingChoices, after.ActingSeat)
	}
}

func TestPotionDeathTargetIsChosenAfterDrawAndSacrifice(t *testing.T) {
	config := testConfig()
	config.Abilities = []AbilityDefinition{{
		AbilityID: "baz-060.front.ability", Status: AbilityReady, BuiltinHandler: "baz-060.front.ability",
		ConformanceScenarioIDs: []string{"baz-060.front.ability.conformance"},
	}}
	match := preparedMatch(t, config)
	state := match.State()
	actor := state.ActingSeat
	source := addBoardObject(&state, actor, "baz-060", 0, 0, SlotSupport)
	target := addBoardObject(&state, actor, "potion-target", 2, 1, SlotUnit)
	match.state = state
	handBefore := len(state.objectsInZone(actor, ZoneHand))
	if err := match.Handle(Command{
		ID: "activate-potion", Kind: CommandActivateAbility, Seat: actor, ExpectedRevision: state.Revision,
		AbilityID: "baz-060.front.ability", SourceRef: refPointer(source.Ref()),
	}); err != nil {
		t.Fatal(err)
	}
	waiting := match.State()
	if len(waiting.objectsInZone(actor, ZoneHand)) != handBefore+1 || len(waiting.PendingChoices) != 1 ||
		waiting.PendingChoices[0].Kind != "baz_060_death_target" || !waiting.PendingChoices[0].Mandatory {
		t.Fatalf("post-sacrifice state hand=%d choices=%+v", len(waiting.objectsInZone(actor, ZoneHand)), waiting.PendingChoices)
	}
	if _, err := waiting.findObject(source.Ref()); err == nil {
		t.Fatal("pre-sacrifice source generation remained valid")
	}
	choice := waiting.PendingChoices[0]
	optionID := ""
	for _, option := range choice.Options {
		if len(option.Objects) == 1 && option.Objects[0] == target.Ref() {
			optionID = option.ID
			break
		}
	}
	if optionID == "" {
		t.Fatal("current target was absent from the post-death allowlist")
	}
	if err := match.Handle(Command{
		ID: "resolve-potion", Kind: CommandSubmitChoice, Seat: actor, ExpectedRevision: waiting.Revision,
		ChoiceID: choice.ChoiceID, OptionIDs: []string{optionID},
	}); err != nil {
		t.Fatal(err)
	}
	after := match.State()
	modified, err := after.findObject(target.Ref())
	if err != nil || modified.Power != 3 || !objectHasKeyword(&after, target.Ref(), KeywordCharge) {
		t.Fatalf("potion target=%+v err=%v", modified, err)
	}
}

func preparedBaseSpellMatch(t *testing.T, cardID, abilityID string) (*Match, Config) {
	t.Helper()
	cost := 1
	config := testConfig()
	config.Versions.Protocol = "1.2"
	config.Versions.Engine = "base-abilities-v2"
	config.Abilities = []AbilityDefinition{{
		AbilityID: abilityID, Status: AbilityReady, BuiltinHandler: abilityID,
		ConformanceScenarioIDs: []string{abilityID + ".conformance"},
	}}
	config.Cards = []CardDefinition{{CardID: cardID, Name: cardID, Types: []CardType{CardSpell}, Cost: &cost, AbilityIDs: []string{abilityID}}}
	for seat := range config.Players {
		for index := range config.Players[seat].DeckCardIDs {
			config.Players[seat].DeckCardIDs[index] = cardID
		}
	}
	return preparedMatch(t, config), config
}

func assertRejectedWithoutMutation(t *testing.T, match *Match, command Command) {
	t.Helper()
	before, err := StateHash(match.State())
	if err != nil {
		t.Fatal(err)
	}
	if err := match.Handle(command); err == nil {
		t.Fatalf("command %s was accepted", command.ID)
	}
	after, err := StateHash(match.State())
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("rejected command %s mutated state", command.ID)
	}
}
