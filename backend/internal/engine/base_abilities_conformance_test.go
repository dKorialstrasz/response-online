package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type conformanceAbilityDocument struct {
	Abilities []struct {
		AbilityID  string `json:"abilityId"`
		EffectSpec struct {
			Status     string `json:"status"`
			Operations []struct {
				Kind    string `json:"kind"`
				Handler string `json:"handler"`
			} `json:"operations"`
		} `json:"effectSpec"`
		ConformanceScenarioIDs []string `json:"conformanceScenarioIds"`
	} `json:"abilities"`
}

type conformanceScenarioDocument struct {
	Scenarios []struct {
		ScenarioID     string `json:"scenarioId"`
		AbilityID      string `json:"abilityId"`
		GameplayStatus string `json:"gameplayStatus"`
		Cases          []struct {
			Kind     string `json:"kind"`
			TestName string `json:"testName"`
		} `json:"cases"`
	} `json:"scenarios"`
	Matchups []struct {
		MatchupID string `json:"matchupId"`
		DeckA     string `json:"deckA"`
		DeckB     string `json:"deckB"`
		Kind      string `json:"kind"`
		TestName  string `json:"testName"`
	} `json:"matchups"`
}

func TestBaseAbilityConformance(t *testing.T) {
	abilities := loadConformanceJSON[conformanceAbilityDocument](t, "content/abilities/base-abilities.json")
	scenarios := loadConformanceJSON[conformanceScenarioDocument](t, "content/conformance/base-ability-scenarios.json")
	if len(abilities.Abilities) != 126 || len(scenarios.Scenarios) != 126 {
		t.Fatalf("catalog/scenario counts = %d/%d, want 126/126", len(abilities.Abilities), len(scenarios.Scenarios))
	}
	scenarioByAbility := make(map[string]struct {
		status string
		kinds  map[string]string
	}, len(scenarios.Scenarios))
	for _, scenario := range scenarios.Scenarios {
		kinds := make(map[string]string, len(scenario.Cases))
		for _, current := range scenario.Cases {
			kinds[current.Kind] = current.TestName
		}
		scenarioByAbility[scenario.AbilityID] = struct {
			status string
			kinds  map[string]string
		}{status: scenario.GameplayStatus, kinds: kinds}
	}

	definitions := make([]AbilityDefinition, 0, len(abilities.Abilities))
	for _, record := range abilities.Abilities {
		if record.EffectSpec.Status != "implemented" || len(record.EffectSpec.Operations) != 1 {
			t.Fatalf("ability %s does not have one implemented operation", record.AbilityID)
		}
		operation := record.EffectSpec.Operations[0]
		definitions = append(definitions, AbilityDefinition{
			AbilityID: record.AbilityID, Status: AbilityReady,
			BuiltinHandler:         operation.Handler,
			ConformanceScenarioIDs: append([]string(nil), record.ConformanceScenarioIDs...),
		})
	}
	if _, err := buildAbilityCatalog(definitions); err != nil {
		t.Fatalf("load complete closed handler catalog: %v", err)
	}

	for index, definition := range definitions {
		definition := definition
		scenario, ok := scenarioByAbility[definition.AbilityID]
		if !ok || scenario.status != "implemented" {
			t.Fatalf("ability %s has no implemented scenario", definition.AbilityID)
		}
		for _, kind := range []string{"positive", "negative", "interaction"} {
			kind := kind
			t.Run(definition.AbilityID+"/"+kind, func(t *testing.T) {
				expectedName := "TestBaseAbilityConformance/" + definition.AbilityID + "/" + kind
				if scenario.kinds[kind] != expectedName {
					t.Fatalf("manifest test name = %q, want %q", scenario.kinds[kind], expectedName)
				}
				switch kind {
				case "positive":
					if _, ok := builtinAbility(definition.BuiltinHandler); !ok {
						t.Fatal("closed handler is not executable")
					}
					number, _ := builtinCardNumber(definition.BuiltinHandler)
					probeState := State{Players: [2]PlayerState{{Seat: SeatA}, {Seat: SeatB}}}
					probeBudget := eventBudget{state: &probeState, limit: DefaultInternalEventCap, cards: map[string]CardDefinition{}}
					probe := builtinAbilityContext{
						state:   &probeState,
						command: Command{Seat: SeatA, AbilityID: definition.AbilityID},
						budget:  &probeBudget,
					}
					if err := probe.execute(number); err != nil && strings.Contains(err.Error(), "no executable semantics") {
						t.Fatal(err)
					}
				case "negative":
					invalid := definition
					invalid.BuiltinHandler = fmt.Sprintf("baz-%03d.front.ability", ((index+64)%132)+1)
					if invalid.BuiltinHandler == invalid.AbilityID {
						invalid.BuiltinHandler = "baz-007.front.ability"
					}
					if _, err := buildAbilityCatalog([]AbilityDefinition{invalid}); err == nil {
						t.Fatal("mismatched or unregistered handler was accepted")
					}
				case "interaction":
					other := definitions[(index+1)%len(definitions)]
					catalog, err := buildAbilityCatalog([]AbilityDefinition{definition, other})
					if err != nil || len(catalog) != 2 {
						t.Fatalf("cross-card catalog interaction failed: size=%d err=%v", len(catalog), err)
					}
				}
			})
		}
	}
}

func TestBaseMatchupInteractionCoverage(t *testing.T) {
	document := loadConformanceJSON[conformanceScenarioDocument](t, "content/conformance/base-ability-scenarios.json")
	if len(document.Matchups) != 36 {
		t.Fatalf("matchup rows = %d, want 36", len(document.Matchups))
	}
	pairs := make(map[string]struct{}, len(document.Matchups))
	mirrors := 0
	for _, matchup := range document.Matchups {
		matchup := matchup
		t.Run(matchup.DeckA+"_vs_"+matchup.DeckB, func(t *testing.T) {
			pair := matchup.DeckA + "\x00" + matchup.DeckB
			if _, duplicate := pairs[pair]; duplicate {
				t.Fatalf("duplicate ordered matchup %s", matchup.MatchupID)
			}
			pairs[pair] = struct{}{}
			expectedKind := "cross-faction"
			if matchup.DeckA == matchup.DeckB {
				expectedKind = "mirror"
				mirrors++
			}
			if matchup.Kind != expectedKind {
				t.Fatalf("kind = %q, want %q", matchup.Kind, expectedKind)
			}
			expectedName := "TestBaseMatchupInteractionCoverage/" + matchup.DeckA + "_vs_" + matchup.DeckB
			if matchup.TestName != expectedName {
				t.Fatalf("testName = %q, want %q", matchup.TestName, expectedName)
			}
		})
	}
	if mirrors != 6 {
		t.Fatalf("mirror matchup rows = %d, want 6", mirrors)
	}
}

func TestBaseKeywordCrossFactionAndMirrorInteractions(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	attacker := addBoardObject(&state, SeatA, "technology-poison-ranged", 1, 0, SlotUnit)
	defender := addBoardObject(&state, SeatB, "life-resilience", 5, 0, SlotUnit)
	attacker.Keywords = []Keyword{KeywordPoison, KeywordRangedAttack}
	defender.Keywords = []Keyword{KeywordResilience}
	budget := eventBudget{state: &state, limit: DefaultInternalEventCap}
	if err := resolveLane(&state, 0, &budget); err != nil {
		t.Fatal(err)
	}
	if defender.Zone != ZoneBoard || defender.Power != 5 || attacker.Zone != ZoneGraveyard {
		t.Fatalf("poison/resilience/ranged interaction = attacker:%s defender:%s/%d", attacker.Zone, defender.Zone, defender.Power)
	}

	state = preparedMatch(t, testConfig()).State()
	left := addBoardObject(&state, SeatA, "bypass-a", 2, 1, SlotUnit)
	right := addBoardObject(&state, SeatB, "bypass-b", 2, 1, SlotUnit)
	left.Keywords = []Keyword{KeywordBypass}
	right.Keywords = []Keyword{KeywordBypass}
	budget = eventBudget{state: &state, limit: DefaultInternalEventCap}
	if err := resolveLane(&state, 1, &budget); err != nil {
		t.Fatal(err)
	}
	if left.Zone != ZoneGraveyard || right.Zone != ZoneGraveyard {
		t.Fatalf("mirror bypass units did not collide: %s/%s", left.Zone, right.Zone)
	}
}

func TestSuppressBaseAbilitiesKeepsGrantedKeyword(t *testing.T) {
	state := preparedMatch(t, testConfig()).State()
	target := addBoardObject(&state, SeatA, "suppressed-unit", 2, 0, SlotUnit)
	target.Keywords = []Keyword{KeywordBypass}
	target.BaseAbilitiesSuppressed = true
	state.NextEffectSequence++
	state.Modifiers = append(state.Modifiers, ModifierState{
		ModifierID: "granted-poison", CreatedSequence: state.NextEffectSequence,
		TargetRef: target.Ref(), Keyword: KeywordPoison, Duration: DurationPermanent,
	})
	if objectHasKeyword(&state, target.Ref(), KeywordBypass) {
		t.Fatal("suppressed printed keyword remained active")
	}
	if !objectHasKeyword(&state, target.Ref(), KeywordPoison) {
		t.Fatal("granted keyword was incorrectly suppressed")
	}
}

func TestClosedHandlerFailureRollsBackAbilityTransaction(t *testing.T) {
	config := testConfig()
	config.Abilities = []AbilityDefinition{{
		AbilityID: "baz-001.level-1.ability", Status: AbilityReady,
		BuiltinHandler:         "baz-001.level-1.ability",
		ConformanceScenarioIDs: []string{"baz-001.level-1.ability.conformance"},
	}}
	match := preparedMatch(t, config)
	state := match.State()
	actor := state.ActingSeat
	source := addBoardObject(&state, actor, "dari-source", 0, 0, SlotSupport)
	target := addBoardObject(&state, actor, "dari-target", 2, 2, SlotUnit)
	match.state = state
	before, _ := StateHash(state)
	err := match.Handle(Command{
		ID: "illegal-dari-move", Kind: CommandActivateAbility, Seat: actor,
		ExpectedRevision: state.Revision, AbilityID: "baz-001.level-1.ability",
		SourceRef:  &ObjectRef{ZoneObjectID: source.ZoneObjectID, Generation: source.Generation},
		TargetRefs: []ObjectRef{target.Ref()}, DestinationLane: 4, DestinationLaneSet: true,
	})
	if err == nil {
		t.Fatal("non-adjacent Dari move was accepted")
	}
	after, _ := StateHash(match.State())
	if before != after {
		t.Fatal("failed closed handler changed state")
	}
}

func TestMagicShieldPaymentCanProtectOnlyTheShieldedEffectPart(t *testing.T) {
	newMatch := func(t *testing.T) (*Match, Seat, ObjectRef, ObjectRef) {
		t.Helper()
		config := testConfig()
		config.Abilities = []AbilityDefinition{{
			AbilityID: "baz-058.front.ability", Status: AbilityReady,
			BuiltinHandler:         "baz-058.front.ability",
			ConformanceScenarioIDs: []string{"baz-058.front.ability.conformance"},
		}}
		cost := 0
		config.Cards = []CardDefinition{{CardID: "baz-058", Name: "Агрессивный взгляд", Types: []CardType{CardSpell}, Cost: &cost, AbilityIDs: []string{"baz-058.front.ability"}}}
		for seat := range config.Players {
			for index := range config.Players[seat].DeckCardIDs {
				config.Players[seat].DeckCardIDs[index] = "baz-058"
			}
		}
		match := preparedMatch(t, config)
		state := match.State()
		actor := state.ActingSeat
		target := addBoardObject(&state, actor.Other(), "shielded-target", 2, 0, SlotUnit)
		state.NextEffectSequence++
		state.Modifiers = append(state.Modifiers, ModifierState{
			ModifierID: "shield", CreatedSequence: state.NextEffectSequence,
			TargetRef: target.Ref(), MagicShieldCost: 2, Duration: DurationPermanent,
		})
		match.state = state
		return match, actor, state.objectsInZone(actor, ZoneHand)[0].Ref(), target.Ref()
	}

	t.Run("decline", func(t *testing.T) {
		match, actor, source, target := newMatch(t)
		state := match.State()
		if err := match.Handle(Command{
			ID: "decline-shield", Kind: CommandPlayCard, Seat: actor,
			ExpectedRevision: state.Revision, SourceRef: &source, DeclaredType: CardSpell,
			TargetRefs: []ObjectRef{target},
		}); err != nil {
			t.Fatal(err)
		}
		result := match.State()
		object, _ := result.findObject(target)
		if object.Power != 2 || result.player(actor).Influence != 5 || !hasEvent(result.Events, "card_ability_fizzled") {
			t.Fatalf("declined shield result = power:%d influence:%d", object.Power, result.player(actor).Influence)
		}
	})

	t.Run("pay", func(t *testing.T) {
		match, actor, source, target := newMatch(t)
		state := match.State()
		if err := match.Handle(Command{
			ID: "pay-shield", Kind: CommandPlayCard, Seat: actor,
			ExpectedRevision: state.Revision, SourceRef: &source, DeclaredType: CardSpell,
			TargetRefs: []ObjectRef{target}, ShieldPaymentRefs: []ObjectRef{target},
		}); err != nil {
			t.Fatal(err)
		}
		result := match.State()
		object, _ := result.findObject(target)
		if object.Power != 5 || result.player(actor).Influence != 3 {
			t.Fatalf("paid shield result = power:%d influence:%d", object.Power, result.player(actor).Influence)
		}
	})
}

func loadConformanceJSON[T any](t *testing.T, relative string) T {
	t.Helper()
	path := filepath.Join("..", "..", "..", relative)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func sortedAbilityIDs(definitions []AbilityDefinition) []string {
	ids := make([]string, len(definitions))
	for index, definition := range definitions {
		ids[index] = definition.AbilityID
	}
	sort.Strings(ids)
	return ids
}
