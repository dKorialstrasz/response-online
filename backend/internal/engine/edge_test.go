package engine

import (
	"errors"
	"io"
	"testing"
	"time"
)

type entropyStub struct {
	data []byte
	err  error
}

func (stub entropyStub) Read(target []byte) (int, error) {
	if stub.err != nil {
		return 0, stub.err
	}
	return copy(target, stub.data), nil
}

func TestSeedFromEntropy(t *testing.T) {
	seed, err := SeedFromEntropy(entropyStub{data: []byte{1, 2, 3, 4, 5, 6, 7, 8}})
	if err != nil || seed != 0x0807060504030201 {
		t.Fatalf("seed=%x err=%v", seed, err)
	}
	zero, err := SeedFromEntropy(entropyStub{data: make([]byte, 8)})
	if err != nil || zero == 0 {
		t.Fatalf("zero entropy fallback seed=%x err=%v", zero, err)
	}
	if _, err := SeedFromEntropy(nil); err == nil {
		t.Fatal("nil entropy accepted")
	}
	if _, err := SeedFromEntropy(entropyStub{data: []byte{1}}); err == nil {
		t.Fatal("short entropy accepted")
	}
	if _, err := SeedFromEntropy(entropyStub{err: io.ErrUnexpectedEOF}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("entropy error=%v", err)
	}
}

func TestConfigValidationRejectsIncompleteInputs(t *testing.T) {
	cases := map[string]func(*Config){
		"missing match":      func(config *Config) { config.MatchID = "" },
		"missing time":       func(config *Config) { config.StartedAt = time.Time{} },
		"missing version":    func(config *Config) { config.Versions.Engine = "" },
		"wrong seat":         func(config *Config) { config.Players[0].Seat = SeatB },
		"negative influence": func(config *Config) { config.Players[0].BaseInfluence = -1 },
		"short deck":         func(config *Config) { config.Players[0].DeckCardIDs = config.Players[0].DeckCardIDs[:29] },
		"short orders":       func(config *Config) { config.Players[0].AvailableIDs = config.Players[0].AvailableIDs[:4] },
		"empty definition":   func(config *Config) { config.Players[0].DeckCardIDs[0] = "" },
		"duplicate order":    func(config *Config) { config.Players[0].AvailableIDs[1] = config.Players[0].AvailableIDs[0] },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			config := testConfig()
			mutate(&config)
			if _, err := NewMatch(config); err == nil {
				t.Fatal("invalid config was accepted")
			}
		})
	}
}

func TestCommandValidationAndSurrender(t *testing.T) {
	match := preparedMatch(t, testConfig())
	state := match.State()
	cases := []Command{
		{Kind: CommandPass, Seat: state.ActingSeat, ExpectedRevision: state.Revision},
		{ID: "bad-seat", Kind: CommandPass, Seat: Seat(9), ExpectedRevision: state.Revision},
		{ID: "unknown-kind", Kind: CommandKind("unknown"), Seat: state.ActingSeat, ExpectedRevision: state.Revision},
		{ID: "missing-action", Kind: CommandCommitAction, Seat: state.ActingSeat, ExpectedRevision: state.Revision},
		{ID: "orders-late", Kind: CommandArrangeOrders, Seat: state.ActingSeat, ExpectedRevision: state.Revision, OrderIDs: testOrders(state.ActingSeat)},
		{ID: "mulligan-late", Kind: CommandMulligan, Seat: state.ActingSeat, ExpectedRevision: state.Revision},
	}
	for _, current := range cases {
		if err := match.Handle(current); err == nil {
			t.Fatalf("invalid command %+v was accepted", current)
		}
	}
	loser := state.ActingSeat
	if err := match.Handle(Command{ID: "surrender", Kind: CommandSurrender, Seat: loser, ExpectedRevision: state.Revision}); err != nil {
		t.Fatal(err)
	}
	finished := match.State()
	if finished.Result == nil || finished.Result.Reason != ResultSurrender || finished.Result.WinnerSeat == nil || *finished.Result.WinnerSeat != loser.Other() {
		t.Fatalf("surrender result=%+v", finished.Result)
	}
	if err := match.Handle(Command{ID: "after-finish", Kind: CommandPass, Seat: loser, ExpectedRevision: finished.Revision}); !errors.Is(err, ErrMatchFinished) {
		t.Fatalf("post-finish error=%v", err)
	}
}

func TestAcceptedCommandDoesNotAliasCallerSlices(t *testing.T) {
	match, err := NewMatch(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	orders := testOrders(SeatA)
	if err := match.Handle(Command{
		ID: "orders-a", Kind: CommandArrangeOrders, Seat: SeatA,
		ExpectedRevision: 0, OrderIDs: orders,
	}); err != nil {
		t.Fatal(err)
	}
	orders[0] = "mutated-by-caller"
	state := match.State()
	if state.player(SeatA).Orders[0].DefinitionID == "mutated-by-caller" || state.Commands[0].Command.OrderIDs[0] == "mutated-by-caller" {
		t.Fatal("engine state aliases the caller command slice")
	}
}

func TestSnapshotDoesNotAliasInternalPointers(t *testing.T) {
	match := preparedMatch(t, testConfig())
	beforeHash, err := StateHash(match.State())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := match.State()
	for index := range snapshot.Events {
		if snapshot.Events[index].Seat != nil {
			*snapshot.Events[index].Seat = snapshot.Events[index].Seat.Other()
			break
		}
	}
	afterHash, err := StateHash(match.State())
	if err != nil {
		t.Fatal(err)
	}
	if afterHash != beforeHash {
		t.Fatal("mutating a snapshot changed internal match state")
	}
}

func TestCombatSurvivingLocationAndCombatEntryRestriction(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	attacker := addBoardObject(&state, SeatA, "late-unit", 2, 1, SlotUnit)
	attacker.EnteredInCombat = true
	budget := eventBudget{state: &state, limit: DefaultInternalEventCap}
	if err := resolveLane(&state, 1, &budget); err != nil {
		t.Fatal(err)
	}
	if state.player(SeatB).Orders[0].Revealed {
		t.Fatal("unit that entered during combat attacked the hero")
	}

	state = preparedMatch(t, testConfig()).State()
	addBoardObject(&state, SeatB, "unit-b", 2, 3, SlotUnit)
	location := addBoardObject(&state, SeatA, "location-a", 0, -1, SlotLocation)
	location.BaseDurability, location.Durability = 5, 5
	budget = eventBudget{state: &state, limit: DefaultInternalEventCap}
	if err := resolveLane(&state, 3, &budget); err != nil {
		t.Fatal(err)
	}
	if location.Durability != 3 || location.Zone != ZoneBoard {
		t.Fatalf("surviving location=%+v", location)
	}
}

func TestRecoveryRestoresBoardAndInfluence(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	unit := addBoardObject(&state, SeatA, "unit-a", 4, 0, SlotUnit)
	unit.Power, unit.Tapped, unit.EnteredInCombat = 1, true, true
	location := addBoardObject(&state, SeatB, "location-b", 0, -1, SlotLocation)
	location.BaseDurability, location.Durability, location.Tapped = 6, 2, true
	state.player(SeatA).Influence = 0
	recoverState(&state)
	if unit.Power != 4 || unit.Tapped || unit.EnteredInCombat {
		t.Fatalf("unit not recovered: %+v", unit)
	}
	if location.Durability != 6 || location.Tapped {
		t.Fatalf("location not recovered: %+v", location)
	}
	if state.player(SeatA).Influence != state.player(SeatA).BaseInfluence {
		t.Fatal("influence not restored")
	}
}

func TestBoardLookupUsesControllerNotOwner(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	object := addBoardObject(&state, SeatA, "controlled-unit", 2, 2, SlotUnit)
	object.ControllerSeat = SeatB
	if state.boardObject(SeatA, 2, SlotUnit) != nil {
		t.Fatal("former controller still sees the unit")
	}
	if got := state.boardObject(SeatB, 2, SlotUnit); got == nil || got.OwnerSeat != SeatA {
		t.Fatalf("new controller lookup=%+v", got)
	}
}

func TestReplayReportsInvalidCommand(t *testing.T) {
	_, _, err := Replay(testConfig(), []Command{{
		ID: "wrong-phase", Kind: CommandPass, Seat: SeatA, ExpectedRevision: 0,
	}})
	if err == nil {
		t.Fatal("invalid replay succeeded")
	}
}

func TestInvariantResultAndReferenceFailures(t *testing.T) {
	base := preparedMatch(t, testConfig()).State()
	tests := map[string]func(*State){
		"finished without result": func(state *State) { state.Status = StatusFinished },
		"result without finish":   func(state *State) { state.Result = &MatchResult{Reason: ResultRulesLoss} },
		"same winner loser": func(state *State) {
			seat := SeatA
			state.Status, state.Phase = StatusFinished, PhaseFinished
			state.Result = &MatchResult{Reason: ResultRulesLoss, WinnerSeat: &seat, LoserSeat: &seat}
		},
		"negative actions": func(state *State) { state.player(SeatA).ActionsRemaining = -1 },
		"bad generation":   func(state *State) { state.player(SeatA).Objects[0].ZoneObjectID = "mismatch" },
		"dead board unit":  func(state *State) { object := addBoardObject(state, SeatA, "dead", 1, 0, SlotUnit); object.Power = 0 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			state := base.Clone()
			mutate(&state)
			if err := ValidateState(state); err == nil {
				t.Fatal("invalid state passed")
			}
		})
	}
}
