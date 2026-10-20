package engine

import (
	"errors"
	"fmt"
	"sort"
)

var (
	ErrStaleRevision  = errors.New("stale match revision")
	ErrInvalidCommand = errors.New("invalid command")
	ErrMatchFinished  = errors.New("match is finished")
	errEventLimit     = errors.New("internal event limit exceeded")
)

type Match struct {
	state     State
	eventCap  int
	abilities map[string]AbilityDefinition
	cards     map[string]CardDefinition
}

type eventBudget struct {
	state *State
	used  int
	limit int
	cards map[string]CardDefinition
}

func NewMatch(config Config) (*Match, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	abilities, err := buildAbilityCatalog(config.Abilities)
	if err != nil {
		return nil, err
	}
	cards, err := buildCardCatalog(config.Cards, abilities)
	if err != nil {
		return nil, err
	}
	eventCap := config.InternalEventCap
	if eventCap <= 0 {
		eventCap = DefaultInternalEventCap
	}
	state := State{
		MatchID:    config.MatchID,
		Versions:   config.Versions,
		StartedAt:  config.StartedAt.UTC(),
		Status:     StatusPreparing,
		Phase:      PhaseSetup,
		CombatLane: -1,
		Random:     newRandom(config.Seed),
	}
	state.InitiativeSeat = Seat(state.Random.intn(PlayerCount))
	state.ActingSeat = state.InitiativeSeat
	for index, playerConfig := range config.Players {
		player := PlayerState{
			Seat:              playerConfig.Seat,
			HeroID:            playerConfig.HeroID,
			BaseInfluence:     playerConfig.BaseInfluence,
			Influence:         playerConfig.BaseInfluence,
			AvailableOrderIDs: append([]string(nil), playerConfig.AvailableIDs...),
		}
		for cardIndex, definitionID := range playerConfig.DeckCardIDs {
			instanceID := fmt.Sprintf("%s:s%d:c%02d", config.MatchID, index, cardIndex)
			object := CardObject{
				InstanceID:     instanceID,
				DefinitionID:   definitionID,
				OwnerSeat:      playerConfig.Seat,
				ControllerSeat: playerConfig.Seat,
				Zone:           ZoneDeck,
				ZoneObjectID:   instanceID + ":g1",
				Generation:     1,
				ZoneIndex:      cardIndex,
				Lane:           -1,
			}
			if definition, ok := cards[definitionID]; ok {
				object.Keywords = append([]Keyword(nil), definition.NativeKeywords...)
				object.BaseMagicShieldCost = definition.BaseMagicShieldCost
				if definition.BasePower != nil {
					object.BasePower = *definition.BasePower
					object.Power = *definition.BasePower
				}
			}
			player.Objects = append(player.Objects, object)
		}
		state.Players[index] = player
	}
	state.Events = append(state.Events, Event{
		Sequence: 1,
		Kind:     "match_created",
		Detail:   fmt.Sprintf("initiative=%d", state.InitiativeSeat),
	})
	if err := ValidateState(state); err != nil {
		return nil, err
	}
	state.Phase = PhaseOrderSelection
	state.Events = append(state.Events, Event{
		Sequence: 2,
		Kind:     "setup_completed",
		Detail:   "version pins, players, objects, initiative",
	})
	if err := ValidateState(state); err != nil {
		return nil, err
	}
	return &Match{state: state, eventCap: eventCap, abilities: abilities, cards: cards}, nil
}

func (match *Match) State() State { return match.state.Clone() }

// FinishTechnical commits a server-owned lifecycle result through the same
// single-writer boundary as player commands. A second terminal transition is
// rejected and therefore cannot overwrite the first committed result.
func (match *Match) FinishTechnical(reason ResultReason, winner, loser *Seat, detail string) error {
	if match.state.Status == StatusFinished {
		return ErrMatchFinished
	}
	if reason != ResultDisconnectForfeit && reason != ResultAbortedTechnical {
		return fmt.Errorf("%w: unsupported technical result %q", ErrInvalidCommand, reason)
	}
	if reason == ResultDisconnectForfeit {
		if winner == nil || loser == nil || !winner.Valid() || !loser.Valid() || *winner == *loser {
			return fmt.Errorf("%w: disconnect forfeit requires distinct winner and loser", ErrInvalidCommand)
		}
	} else if winner != nil || loser != nil {
		return fmt.Errorf("%w: technical abort cannot declare a winner", ErrInvalidCommand)
	}

	next := match.state.Clone()
	next.finish(reason, winner, loser, detail)
	next.Events = append(next.Events, Event{
		Sequence: uint64(len(next.Events) + 1), Kind: "match_" + string(reason), Detail: detail,
	})
	next.Revision++
	if err := ValidateState(next); err != nil {
		return fmt.Errorf("technical-result invariant: %w", err)
	}
	match.state = next
	return nil
}

func (match *Match) Handle(command Command) error {
	command = normalizeCommand(command)
	if command.ID == "" || !command.Seat.Valid() {
		return fmt.Errorf("%w: command id and valid seat are required", ErrInvalidCommand)
	}
	if match.state.Status == StatusFinished {
		return ErrMatchFinished
	}
	if command.ExpectedRevision != match.state.Revision {
		return ErrStaleRevision
	}
	for _, record := range match.state.Commands {
		if record.Command.ID == command.ID {
			return fmt.Errorf("%w: duplicate command id", ErrInvalidCommand)
		}
	}
	if len(match.state.PendingChoices) > 0 && command.Kind != CommandSubmitChoice {
		return fmt.Errorf("%w: a pending choice must be resolved first", ErrInvalidCommand)
	}

	next := match.state.Clone()
	budget := eventBudget{state: &next, limit: match.eventCap, cards: match.cards}
	var err error
	switch command.Kind {
	case CommandArrangeOrders:
		err = applyArrangeOrders(&next, command, &budget)
	case CommandMulligan:
		err = applyMulligan(&next, command, &budget)
	case CommandPlayCard:
		err = applyPlayCard(&next, command, match.abilities, match.cards, &budget)
	case CommandCommitAction:
		err = applyDecision(&next, command, false, &budget)
	case CommandPass:
		err = applyDecision(&next, command, true, &budget)
	case CommandSurrender:
		err = applySurrender(&next, command, &budget)
	case CommandActivateAbility:
		err = applyActivateAbility(&next, command, match.abilities, &budget)
	case CommandSubmitChoice:
		err = applySubmitChoice(&next, command, &budget)
	default:
		err = fmt.Errorf("%w: unsupported command kind %q", ErrInvalidCommand, command.Kind)
	}
	if errors.Is(err, errEventLimit) {
		return match.commitTechnicalAbort(command, budget.used)
	}
	if err != nil {
		return err
	}
	next.Revision++
	next.Commands = append(next.Commands, CommandRecord{Sequence: uint64(len(next.Commands) + 1), Command: command})
	if err := ValidateState(next); err != nil {
		return fmt.Errorf("post-command invariant: %w", err)
	}
	match.state = next
	return nil
}

func normalizeCommand(command Command) Command {
	command.OrderIDs = append([]string(nil), command.OrderIDs...)
	command.ObjectRefs = append([]ObjectRef(nil), command.ObjectRefs...)
	command.SourceRef = cloneObjectRef(command.SourceRef)
	command.ModeIDs = append([]string(nil), command.ModeIDs...)
	command.TargetRefs = append([]ObjectRef(nil), command.TargetRefs...)
	command.PaymentRefs = append([]ObjectRef(nil), command.PaymentRefs...)
	command.ShieldPaymentRefs = append([]ObjectRef(nil), command.ShieldPaymentRefs...)
	command.OptionIDs = append([]string(nil), command.OptionIDs...)
	sort.Slice(command.PaymentRefs, func(left, right int) bool {
		if command.PaymentRefs[left].ZoneObjectID != command.PaymentRefs[right].ZoneObjectID {
			return command.PaymentRefs[left].ZoneObjectID < command.PaymentRefs[right].ZoneObjectID
		}
		return command.PaymentRefs[left].Generation < command.PaymentRefs[right].Generation
	})
	sort.Slice(command.ShieldPaymentRefs, func(left, right int) bool {
		if command.ShieldPaymentRefs[left].ZoneObjectID != command.ShieldPaymentRefs[right].ZoneObjectID {
			return command.ShieldPaymentRefs[left].ZoneObjectID < command.ShieldPaymentRefs[right].ZoneObjectID
		}
		return command.ShieldPaymentRefs[left].Generation < command.ShieldPaymentRefs[right].Generation
	})
	if command.Kind == CommandMulligan {
		sort.Slice(command.ObjectRefs, func(left, right int) bool {
			if command.ObjectRefs[left].ZoneObjectID != command.ObjectRefs[right].ZoneObjectID {
				return command.ObjectRefs[left].ZoneObjectID < command.ObjectRefs[right].ZoneObjectID
			}
			return command.ObjectRefs[left].Generation < command.ObjectRefs[right].Generation
		})
	}
	return command
}

func (match *Match) commitTechnicalAbort(command Command, used int) error {
	next := match.state.Clone()
	detail := fmt.Sprintf("internal event cap %d exceeded after %d events", match.eventCap, used)
	next.DiagnosticTrace = append(next.DiagnosticTrace, detail, "command="+command.ID)
	next.finish(ResultAbortedTechnical, nil, nil, detail)
	next.Events = append(next.Events, Event{
		Sequence: uint64(len(next.Events) + 1),
		Kind:     "match_aborted_technical",
		Detail:   detail,
	})
	next.Revision++
	next.Commands = append(next.Commands, CommandRecord{Sequence: uint64(len(next.Commands) + 1), Command: command})
	if err := ValidateState(next); err != nil {
		return fmt.Errorf("technical-abort invariant: %w", err)
	}
	match.state = next
	return nil
}

func (budget *eventBudget) emit(kind string, seat *Seat, detail string) error {
	budget.used++
	if budget.used > budget.limit {
		return errEventLimit
	}
	budget.state.Events = append(budget.state.Events, Event{
		Sequence: uint64(len(budget.state.Events) + 1),
		Kind:     kind,
		Seat:     seat,
		Detail:   detail,
	})
	if err := ValidateState(*budget.state); err != nil {
		return fmt.Errorf("event %s invariant: %w", kind, err)
	}
	return nil
}

func applyArrangeOrders(state *State, command Command, budget *eventBudget) error {
	if state.Phase != PhaseOrderSelection {
		return fmt.Errorf("%w: orders can only be arranged during order selection", ErrInvalidCommand)
	}
	player := state.player(command.Seat)
	if player.OrdersLocked {
		return fmt.Errorf("%w: orders are already locked", ErrInvalidCommand)
	}
	if !sameSet(command.OrderIDs, player.AvailableOrderIDs) {
		return fmt.Errorf("%w: order arrangement must be a permutation of the five available orders", ErrInvalidCommand)
	}
	player.Orders = make([]OrderState, len(command.OrderIDs))
	for index, definitionID := range command.OrderIDs {
		player.Orders[index] = OrderState{DefinitionID: definitionID, Position: index}
	}
	player.OrdersLocked = true
	seat := command.Seat
	if err := budget.emit("orders_arranged", &seat, "five orders locked"); err != nil {
		return err
	}
	if !state.Players[0].OrdersLocked || !state.Players[1].OrdersLocked {
		return nil
	}
	for seatIndex := 0; seatIndex < PlayerCount; seatIndex++ {
		seat := Seat(seatIndex)
		state.shuffleDeck(seat)
		for range StartingHandSize {
			drawn, err := state.draw(seat)
			if err != nil || !drawn {
				return fmt.Errorf("%w: initial deck cannot provide seven cards", ErrInvalidCommand)
			}
		}
	}
	state.Phase = PhaseMulligan
	return budget.emit("starting_hands_dealt", nil, "seven cards per player")
}

func applyMulligan(state *State, command Command, budget *eventBudget) error {
	if state.Phase != PhaseMulligan {
		return fmt.Errorf("%w: mulligan can only be submitted during mulligan", ErrInvalidCommand)
	}
	player := state.player(command.Seat)
	if player.MulliganSubmitted {
		return fmt.Errorf("%w: mulligan may only be submitted once", ErrInvalidCommand)
	}
	selected := make([]*CardObject, 0, len(command.ObjectRefs))
	seen := make(map[string]struct{})
	for _, ref := range command.ObjectRefs {
		object, err := state.findObject(ref)
		if err != nil || object.OwnerSeat != command.Seat || object.Zone != ZoneHand {
			return fmt.Errorf("%w: mulligan contains a stale or non-hand object", ErrInvalidCommand)
		}
		if _, exists := seen[object.InstanceID]; exists {
			return fmt.Errorf("%w: mulligan contains a duplicate object", ErrInvalidCommand)
		}
		seen[object.InstanceID] = struct{}{}
		selected = append(selected, object)
	}
	sort.Slice(selected, func(left, right int) bool {
		return selected[left].InstanceID < selected[right].InstanceID
	})
	state.randomizeBottom(selected, command.Seat)
	for range selected {
		drawn, err := state.draw(command.Seat)
		if err != nil || !drawn {
			return fmt.Errorf("%w: deck exhausted during mulligan", ErrInvalidCommand)
		}
	}
	player.MulliganSubmitted = true
	seat := command.Seat
	if err := budget.emit("mulligan_submitted", &seat, fmt.Sprintf("count=%d", len(selected))); err != nil {
		return err
	}
	if !state.Players[0].MulliganSubmitted || !state.Players[1].MulliganSubmitted {
		return nil
	}
	state.Status = StatusActive
	state.TurnNumber = 1
	return beginTurn(state, budget)
}

func applyDecision(state *State, command Command, passed bool, budget *eventBudget) error {
	if state.Phase != PhaseAction || state.ActingSeat != command.Seat {
		return fmt.Errorf("%w: only the acting seat can decide during the action phase", ErrInvalidCommand)
	}
	player := state.player(command.Seat)
	if player.ActionsRemaining <= 0 {
		return fmt.Errorf("%w: action budget is exhausted", ErrInvalidCommand)
	}
	if !passed && command.ActionID == "" {
		return fmt.Errorf("%w: committed action requires an action id", ErrInvalidCommand)
	}
	player.ActionsRemaining--
	seat := command.Seat
	if passed {
		state.ConsecutivePasses++
		if err := budget.emit("action_passed", &seat, fmt.Sprintf("remaining=%d", player.ActionsRemaining)); err != nil {
			return err
		}
	} else {
		state.ConsecutivePasses = 0
		if err := budget.emit("action_committed", &seat, command.ActionID); err != nil {
			return err
		}
	}
	return advanceActionTurn(state, budget)
}

func advanceActionTurn(state *State, budget *eventBudget) error {
	if state.ConsecutivePasses >= 2 {
		return completeTurn(state, budget)
	}
	state.ActingSeat = state.ActingSeat.Other()
	for state.player(state.ActingSeat).ActionsRemaining == 0 {
		seat := state.ActingSeat
		state.ConsecutivePasses++
		if err := budget.emit("action_auto_passed", &seat, "action budget exhausted"); err != nil {
			return err
		}
		if state.ConsecutivePasses >= 2 {
			return completeTurn(state, budget)
		}
		state.ActingSeat = state.ActingSeat.Other()
	}
	return nil
}

func applySurrender(state *State, command Command, budget *eventBudget) error {
	if state.Status != StatusActive {
		return fmt.Errorf("%w: surrender requires an active match", ErrInvalidCommand)
	}
	winner := command.Seat.Other()
	loser := command.Seat
	state.finish(ResultSurrender, &winner, &loser, "player surrendered")
	return budget.emit("match_finished", &loser, string(ResultSurrender))
}

func beginTurn(state *State, budget *eventBudget) error {
	state.Phase = PhaseStart
	state.CombatStep = CombatNone
	state.CombatLane = -1
	if err := budget.emit("phase_started", nil, string(PhaseStart)); err != nil {
		return err
	}
	state.Phase = PhaseDraw
	if err := budget.emit("phase_started", nil, string(PhaseDraw)); err != nil {
		return err
	}
	if state.TurnNumber == 1 {
		if err := budget.emit("draw_skipped", nil, "first turn"); err != nil {
			return err
		}
	} else if err := drawPhase(state, budget); err != nil || state.Result != nil {
		return err
	}
	state.Phase = PhaseAction
	state.ActingSeat = state.InitiativeSeat
	state.ConsecutivePasses = 0
	for index := range state.Players {
		state.Players[index].ActionsRemaining = ActionsPerPlayer
	}
	return budget.emit("phase_started", nil, string(PhaseAction))
}

func drawPhase(state *State, budget *eventBudget) error {
	// Simultaneous Order requirements are processed non-initiative first, as S1
	// requires when both players would reveal an Order at the same time.
	seats := []Seat{state.InitiativeSeat.Other(), state.InitiativeSeat}
	for _, seat := range seats {
		drawn, err := state.draw(seat)
		if err != nil {
			return err
		}
		if drawn {
			seatCopy := seat
			if err := budget.emit("card_drawn", &seatCopy, "one card"); err != nil {
				return err
			}
			continue
		}
		if err := revealOrder(state, seat, "empty_deck", budget); err != nil || state.Result != nil {
			return err
		}
	}
	return nil
}

func sameSet(actual, expected []string) bool {
	if len(actual) != OrdersPerPlayer || len(expected) != OrdersPerPlayer {
		return false
	}
	counts := make(map[string]int, len(expected))
	for _, value := range expected {
		counts[value]++
	}
	for _, value := range actual {
		counts[value]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func validateConfig(config Config) error {
	if config.MatchID == "" || config.StartedAt.IsZero() {
		return errors.New("matchId and injected start time are required")
	}
	if err := validateVersions(config.Versions); err != nil {
		return err
	}
	for index, player := range config.Players {
		if player.Seat != Seat(index) || player.HeroID == "" || player.BaseInfluence < 0 {
			return fmt.Errorf("player %d configuration is invalid", index)
		}
		if len(player.DeckCardIDs) != 30 {
			return fmt.Errorf("seat %d deck must contain exactly 30 cards", player.Seat)
		}
		if len(player.AvailableIDs) != OrdersPerPlayer {
			return fmt.Errorf("seat %d must have exactly five Orders", player.Seat)
		}
		seen := make(map[string]struct{})
		for _, id := range append(append([]string(nil), player.DeckCardIDs...), player.AvailableIDs...) {
			if id == "" {
				return fmt.Errorf("seat %d contains an empty definition id", player.Seat)
			}
		}
		for _, id := range player.AvailableIDs {
			if _, exists := seen[id]; exists {
				return fmt.Errorf("seat %d contains duplicate Order %s", player.Seat, id)
			}
			seen[id] = struct{}{}
		}
	}
	return nil
}
