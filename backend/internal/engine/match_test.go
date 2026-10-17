package engine

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSetupMulliganAndReplayAreDeterministic(t *testing.T) {
	config := testConfig()
	match, err := NewMatch(config)
	if err != nil {
		t.Fatal(err)
	}
	commands := []Command{
		command("orders-a", CommandArrangeOrders, SeatA, 0, testOrders(SeatA), nil),
		command("orders-b", CommandArrangeOrders, SeatB, 1, reverse(testOrders(SeatB)), nil),
	}
	for _, current := range commands {
		if err := match.Handle(current); err != nil {
			t.Fatalf("Handle(%s): %v", current.ID, err)
		}
	}
	state := match.State()
	if state.Phase != PhaseMulligan {
		t.Fatalf("phase = %s, want mulligan", state.Phase)
	}
	fullHand := refs(state.objectsInZone(SeatA, ZoneHand))
	mulligans := []Command{
		command("mulligan-a", CommandMulligan, SeatA, 2, nil, fullHand),
		command("mulligan-b", CommandMulligan, SeatB, 3, nil, nil),
	}
	for _, current := range mulligans {
		if err := match.Handle(current); err != nil {
			t.Fatalf("Handle(%s): %v", current.ID, err)
		}
		commands = append(commands, current)
	}
	state = match.State()
	if state.Status != StatusActive || state.Phase != PhaseAction || state.TurnNumber != 1 {
		t.Fatalf("unexpected active state: status=%s phase=%s turn=%d", state.Status, state.Phase, state.TurnNumber)
	}
	for seat := SeatA; seat <= SeatB; seat++ {
		if got := len(state.objectsInZone(seat, ZoneHand)); got != StartingHandSize {
			t.Fatalf("seat %d hand = %d, want %d", seat, got, StartingHandSize)
		}
	}
	if state.Random.Cursor == 0 {
		t.Fatal("random cursor did not advance")
	}

	replayed, replayHash, err := Replay(config, commands)
	if err != nil {
		t.Fatal(err)
	}
	liveHash, err := StateHash(state)
	if err != nil {
		t.Fatal(err)
	}
	if replayHash != liveHash {
		t.Fatalf("replay hash %s != live hash %s", replayHash, liveHash)
	}
	const goldenHash = "3a390ad4affd4ae802ba39cd1ace26996d29cfbdfc1f3927ae969de64526995c"
	if liveHash != goldenHash {
		t.Fatalf("golden replay hash = %s, want %s", liveHash, goldenHash)
	}
	if replayed.Random != state.Random {
		t.Fatalf("replayed RNG %+v != live RNG %+v", replayed.Random, state.Random)
	}
}

func TestMulliganSelectionIsAnUnorderedSet(t *testing.T) {
	config := testConfig()
	hashes := make([]string, 0, 2)
	for reverseRefs := false; ; reverseRefs = true {
		match, err := NewMatch(config)
		if err != nil {
			t.Fatal(err)
		}
		for _, current := range []Command{
			command("orders-a", CommandArrangeOrders, SeatA, 0, testOrders(SeatA), nil),
			command("orders-b", CommandArrangeOrders, SeatB, 1, testOrders(SeatB), nil),
		} {
			if err := match.Handle(current); err != nil {
				t.Fatal(err)
			}
		}
		state := match.State()
		selected := refs(state.objectsInZone(SeatA, ZoneHand)[:3])
		if reverseRefs {
			for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
				selected[left], selected[right] = selected[right], selected[left]
			}
		}
		if err := match.Handle(command("mulligan-a", CommandMulligan, SeatA, 2, nil, selected)); err != nil {
			t.Fatal(err)
		}
		if err := match.Handle(command("mulligan-b", CommandMulligan, SeatB, 3, nil, nil)); err != nil {
			t.Fatal(err)
		}
		hash, err := StateHash(match.State())
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, hash)
		if reverseRefs {
			break
		}
	}
	if hashes[0] != hashes[1] {
		t.Fatalf("mulligan ref order changed state hash: %v", hashes)
	}
}

func TestExhaustedActionBudgetsAutoPassAndEndPhase(t *testing.T) {
	match := preparedMatch(t, testConfig())
	for index := 0; match.State().TurnNumber == 1; index++ {
		if index > ActionsPerPlayer*PlayerCount {
			t.Fatal("action phase did not terminate after both budgets were exhausted")
		}
		state := match.State()
		current := Command{
			ID:               fmt.Sprintf("budget-action-%d", index),
			Kind:             CommandCommitAction,
			Seat:             state.ActingSeat,
			ExpectedRevision: state.Revision,
			ActionID:         fmt.Sprintf("accepted-%d", index),
		}
		if err := match.Handle(current); err != nil {
			t.Fatal(err)
		}
	}
	state := match.State()
	if state.TurnNumber != 2 || state.Phase != PhaseAction {
		t.Fatalf("automatic passes ended at turn=%d phase=%s", state.TurnNumber, state.Phase)
	}
}

func TestEmptyDeckUsesNonInitiativeOrderFirstAndCanLose(t *testing.T) {
	match := preparedMatch(t, testConfig())
	state := match.State()
	// Recovery flips initiative before the turn-two draw. The seat without the
	// new initiative must resolve a simultaneous empty-deck Order first.
	newInitiative := state.InitiativeSeat.Other()
	firstRequired := newInitiative.Other()
	for playerIndex := range state.Players {
		player := &state.Players[playerIndex]
		graveIndex := len(state.objectsInZone(player.Seat, ZoneGraveyard))
		for objectIndex := range player.Objects {
			object := &player.Objects[objectIndex]
			if object.Zone == ZoneDeck {
				moveObject(object, ZoneGraveyard, graveIndex)
				graveIndex++
			}
		}
	}
	for index := range state.player(firstRequired).Orders {
		state.player(firstRequired).Orders[index].Revealed = true
	}
	if err := ValidateState(state); err != nil {
		t.Fatal(err)
	}
	match.state = state
	for index := 0; index < 2; index++ {
		current := match.State()
		if err := match.Handle(Command{
			ID:               fmt.Sprintf("empty-pass-%d", index),
			Kind:             CommandPass,
			Seat:             current.ActingSeat,
			ExpectedRevision: current.Revision,
		}); err != nil {
			t.Fatal(err)
		}
	}
	state = match.State()
	if state.Result == nil || state.Result.LoserSeat == nil || *state.Result.LoserSeat != firstRequired {
		t.Fatalf("empty-deck result = %+v, want loser %d", state.Result, firstRequired)
	}
}

func TestInvalidCommandRollsBackWholeTransaction(t *testing.T) {
	match := preparedMatch(t, testConfig())
	before := match.State()
	beforeHash, _ := StateHash(before)
	command := Command{
		ID:               "not-your-turn",
		Kind:             CommandPass,
		Seat:             before.ActingSeat.Other(),
		ExpectedRevision: before.Revision,
	}
	if err := match.Handle(command); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("error = %v, want ErrInvalidCommand", err)
	}
	afterHash, _ := StateHash(match.State())
	if afterHash != beforeHash {
		t.Fatalf("invalid command changed state: before=%s after=%s", beforeHash, afterHash)
	}

	stale := Command{ID: "stale", Kind: CommandPass, Seat: before.ActingSeat, ExpectedRevision: before.Revision - 1}
	if err := match.Handle(stale); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("stale error = %v, want ErrStaleRevision", err)
	}
}

func TestActionsPassesAndAutomaticTurnLifecycle(t *testing.T) {
	match := preparedMatch(t, testConfig())
	start := match.State()
	first := start.ActingSeat
	if err := match.Handle(Command{ID: "action-1", Kind: CommandCommitAction, Seat: first, ExpectedRevision: start.Revision, ActionID: "future-card-action"}); err != nil {
		t.Fatal(err)
	}
	state := match.State()
	if state.player(first).ActionsRemaining != ActionsPerPlayer-1 || state.ActingSeat != first.Other() {
		t.Fatalf("action did not consume and alternate: %+v", state)
	}
	if err := match.Handle(Command{ID: "pass-1", Kind: CommandPass, Seat: state.ActingSeat, ExpectedRevision: state.Revision}); err != nil {
		t.Fatal(err)
	}
	state = match.State()
	if state.ConsecutivePasses != 1 || state.ActingSeat != first {
		t.Fatalf("first pass state = passes %d actor %d", state.ConsecutivePasses, state.ActingSeat)
	}
	if err := match.Handle(Command{ID: "pass-2", Kind: CommandPass, Seat: state.ActingSeat, ExpectedRevision: state.Revision}); err != nil {
		t.Fatal(err)
	}
	state = match.State()
	if state.TurnNumber != 2 || state.Phase != PhaseAction {
		t.Fatalf("turn lifecycle ended at turn=%d phase=%s", state.TurnNumber, state.Phase)
	}
	if state.InitiativeSeat != start.InitiativeSeat.Other() || state.ActingSeat != state.InitiativeSeat {
		t.Fatal("recovery did not transfer initiative")
	}
	for seat := SeatA; seat <= SeatB; seat++ {
		if state.player(seat).ActionsRemaining != ActionsPerPlayer {
			t.Fatalf("seat %d action budget was not restored", seat)
		}
		if got := len(state.objectsInZone(seat, ZoneHand)); got != StartingHandSize+1 {
			t.Fatalf("seat %d hand after second-turn draw = %d, want %d", seat, got, StartingHandSize+1)
		}
	}
	assertEventOrder(t, state.Events, []string{
		"phase_started", "combat_started", "combat_lane_started", "combat_ended",
		"phase_started", "phase_started", "recovery_completed", "phase_started", "phase_started", "card_drawn", "phase_started",
	})
}

func TestBothStartingInitiativesDriveOppositeLaneOrder(t *testing.T) {
	seen := map[Seat]bool{}
	for seed := uint64(1); seed <= 64 && len(seen) < PlayerCount; seed++ {
		config := testConfig()
		config.Seed = seed
		match := preparedMatch(t, config)
		starting := match.State().InitiativeSeat
		if seen[starting] {
			continue
		}
		seen[starting] = true
		for index := 0; index < 2; index++ {
			state := match.State()
			if err := match.Handle(Command{
				ID: fmt.Sprintf("initiative-%d-pass-%d", starting, index), Kind: CommandPass,
				Seat: state.ActingSeat, ExpectedRevision: state.Revision,
			}); err != nil {
				t.Fatal(err)
			}
		}
		firstLane := ""
		for _, event := range match.State().Events {
			if event.Kind == "combat_lane_started" {
				firstLane = event.Detail
				break
			}
		}
		want := "lane=0"
		if starting == SeatB {
			want = "lane=4"
		}
		if firstLane != want {
			t.Fatalf("initiative %d first lane=%q, want %q", starting, firstLane, want)
		}
	}
	if !seen[SeatA] || !seen[SeatB] {
		t.Fatalf("seed fixtures did not cover both initiatives: %v", seen)
	}
}

func TestCombatCollisionLocationAndHeroOrder(t *testing.T) {
	t.Run("units collide simultaneously", func(t *testing.T) {
		state := preparedMatch(t, testConfig()).State()
		addBoardObject(&state, SeatA, "unit-a", 2, 0, SlotUnit)
		addBoardObject(&state, SeatB, "unit-b", 3, 0, SlotUnit)
		budget := eventBudget{state: &state, limit: DefaultInternalEventCap}
		if err := resolveLane(&state, 0, &budget); err != nil {
			t.Fatal(err)
		}
		if state.boardObject(SeatA, 0, SlotUnit) != nil {
			t.Fatal("seat A unit with non-positive power survived")
		}
		unitB := state.boardObject(SeatB, 0, SlotUnit)
		if unitB == nil || unitB.Power != 1 {
			t.Fatalf("seat B unit = %+v, want power 1", unitB)
		}
	})

	t.Run("location stops attacker even when destroyed", func(t *testing.T) {
		state := preparedMatch(t, testConfig()).State()
		addBoardObject(&state, SeatA, "unit-a", 4, 2, SlotUnit)
		location := addBoardObject(&state, SeatB, "location-b", 0, -1, SlotLocation)
		location.BaseDurability, location.Durability = 3, 3
		budget := eventBudget{state: &state, limit: DefaultInternalEventCap}
		if err := resolveLane(&state, 2, &budget); err != nil {
			t.Fatal(err)
		}
		if state.boardObject(SeatB, -1, SlotLocation) != nil {
			t.Fatal("destroyed location remained on board")
		}
		if state.player(SeatB).Orders[0].Revealed {
			t.Fatal("hero was hit through a location destroyed by the same collision")
		}
	})

	t.Run("unopposed attack reveals leftmost order", func(t *testing.T) {
		state := preparedMatch(t, testConfig()).State()
		addBoardObject(&state, SeatA, "unit-a", 1, 4, SlotUnit)
		budget := eventBudget{state: &state, limit: DefaultInternalEventCap}
		if err := resolveLane(&state, 4, &budget); err != nil {
			t.Fatal(err)
		}
		if !state.player(SeatB).Orders[0].Revealed || state.player(SeatB).Orders[1].Revealed {
			t.Fatal("hero hit did not reveal exactly the leftmost closed order")
		}
	})
}

func TestMissingOrderProducesSingleRulesLoss(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	for index := range state.player(SeatB).Orders {
		state.player(SeatB).Orders[index].Revealed = true
	}
	budget := eventBudget{state: &state, limit: DefaultInternalEventCap}
	if err := revealOrder(&state, SeatB, "hero_hit", &budget); err != nil {
		t.Fatal(err)
	}
	if state.Result == nil || state.Result.Reason != ResultRulesLoss || state.Result.WinnerSeat == nil || *state.Result.WinnerSeat != SeatA {
		t.Fatalf("result = %+v", state.Result)
	}
	first := *state.Result
	eventCount := len(state.Events)
	revealOrder(&state, SeatB, "second_attempt", &budget) // must not replace the result
	if *state.Result.WinnerSeat != *first.WinnerSeat || state.Result.Detail != first.Detail {
		t.Fatal("an existing result was overwritten")
	}
	if len(state.Events) != eventCount {
		t.Fatal("finished match emitted a duplicate result event")
	}
}

func TestZoneMoveInvalidatesOldGenerationReference(t *testing.T) {
	match := preparedMatch(t, testConfig())
	state := match.State()
	object := state.objectsInZone(SeatA, ZoneHand)[0]
	old := object.Ref()
	moveObject(object, ZoneGraveyard, 0)
	if object.Generation != old.Generation+1 || object.ZoneObjectID == old.ZoneObjectID {
		t.Fatalf("move did not issue a new generation: old=%+v new=%+v", old, object.Ref())
	}
	if _, err := state.findObject(old); err == nil {
		t.Fatal("stale generation reference still resolved")
	}
	if _, err := state.findObject(object.Ref()); err != nil {
		t.Fatalf("fresh generation reference failed: %v", err)
	}
}

func TestRunawayGuardCommitsTechnicalAbort(t *testing.T) {
	config := testConfig()
	config.InternalEventCap = 1
	match, err := NewMatch(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := match.Handle(command("orders-a", CommandArrangeOrders, SeatA, 0, testOrders(SeatA), nil)); err != nil {
		t.Fatal(err)
	}
	if err := match.Handle(command("orders-b", CommandArrangeOrders, SeatB, 1, testOrders(SeatB), nil)); err != nil {
		t.Fatal(err)
	}
	state := match.State()
	if state.Result == nil || state.Result.Reason != ResultAbortedTechnical || state.Result.WinnerSeat != nil {
		t.Fatalf("runaway result = %+v", state.Result)
	}
	if len(state.DiagnosticTrace) < 2 {
		t.Fatalf("diagnostic trace = %v", state.DiagnosticTrace)
	}
}

func TestInvariantRejectsDoubleOccupiedCellAndDanglingChoice(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	addBoardObject(&state, SeatA, "unit-a", 1, 1, SlotUnit)
	addBoardObject(&state, SeatA, "unit-b", 1, 1, SlotUnit)
	if err := ValidateState(state); err == nil {
		t.Fatal("double occupied cell passed validation")
	}
	state = preparedMatch(t, testConfig()).State()
	state.PendingChoices = []PendingChoice{{ChoiceID: "choice-1", Seat: SeatA, Kind: "target"}}
	if err := ValidateState(state); err == nil {
		t.Fatal("choice without legal options passed validation")
	}
}

func preparedMatch(t *testing.T, config Config) *Match {
	t.Helper()
	match, err := NewMatch(config)
	if err != nil {
		t.Fatal(err)
	}
	commands := []Command{
		command("orders-a", CommandArrangeOrders, SeatA, 0, testOrders(SeatA), nil),
		command("orders-b", CommandArrangeOrders, SeatB, 1, testOrders(SeatB), nil),
		command("mulligan-a", CommandMulligan, SeatA, 2, nil, nil),
		command("mulligan-b", CommandMulligan, SeatB, 3, nil, nil),
	}
	for _, current := range commands {
		if err := match.Handle(current); err != nil {
			t.Fatalf("prepare %s: %v", current.ID, err)
		}
	}
	return match
}

func testConfig() Config {
	return Config{
		MatchID: "match-test",
		Versions: Versions{
			Content:       "base-cards-v1",
			DeckManifests: "base-decks-v1",
			EffectSpec:    "effect-spec-v1",
			Rules:         "response-rules-2026-10-17",
			Protocol:      "scaffold-v1",
			Engine:        "effect-pipeline-v1",
		},
		Players: [2]PlayerConfig{
			testPlayer(SeatA),
			testPlayer(SeatB),
		},
		Seed:      42,
		StartedAt: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC),
	}
}

func testPlayer(seat Seat) PlayerConfig {
	deck := make([]string, 30)
	for index := range deck {
		deck[index] = fmt.Sprintf("seat-%d-card-%02d", seat, index)
	}
	return PlayerConfig{
		Seat:          seat,
		HeroID:        fmt.Sprintf("seat-%d-hero", seat),
		DeckCardIDs:   deck,
		AvailableIDs:  testOrders(seat),
		BaseInfluence: 5,
	}
}

func testOrders(seat Seat) []string {
	orders := make([]string, OrdersPerPlayer)
	for index := range orders {
		orders[index] = fmt.Sprintf("seat-%d-order-%d", seat, index)
	}
	return orders
}

func command(id string, kind CommandKind, seat Seat, revision uint64, orders []string, objects []ObjectRef) Command {
	return Command{ID: id, Kind: kind, Seat: seat, ExpectedRevision: revision, OrderIDs: orders, ObjectRefs: objects}
}

func reverse(values []string) []string {
	result := append([]string(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func refs(objects []*CardObject) []ObjectRef {
	result := make([]ObjectRef, len(objects))
	for index, object := range objects {
		result[index] = object.Ref()
	}
	return result
}

func addBoardObject(state *State, seat Seat, id string, power, lane int, slot BoardSlot) *CardObject {
	object := CardObject{
		InstanceID:     id,
		DefinitionID:   id,
		OwnerSeat:      seat,
		ControllerSeat: seat,
		Zone:           ZoneBoard,
		ZoneObjectID:   id + ":g1",
		Generation:     1,
		ZoneIndex:      -1,
		Lane:           lane,
		Slot:           slot,
		BasePower:      power,
		Power:          power,
	}
	state.player(seat).Objects = append(state.player(seat).Objects, object)
	return &state.player(seat).Objects[len(state.player(seat).Objects)-1]
}

func assertEventOrder(t *testing.T, events []Event, wanted []string) {
	t.Helper()
	position := 0
	for _, event := range events {
		if position < len(wanted) && event.Kind == wanted[position] {
			position++
		}
	}
	if position != len(wanted) {
		t.Fatalf("event sequence matched %d/%d; wanted %v", position, len(wanted), wanted)
	}
}
