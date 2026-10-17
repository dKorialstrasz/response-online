package engine

import "fmt"

func completeTurn(state *State, budget *eventBudget) error {
	state.Phase = PhaseCombat
	state.CombatStep = CombatStart
	state.CombatLane = -1
	if err := budget.emit("phase_started", nil, string(PhaseCombat)); err != nil {
		return err
	}
	if err := budget.emit("combat_started", nil, "initiative-relative lane order"); err != nil {
		return err
	}
	for step := 0; step < LaneCount; step++ {
		lane := step
		if state.InitiativeSeat == SeatB {
			lane = LaneCount - 1 - step
		}
		state.CombatStep = CombatLane
		state.CombatLane = lane
		if err := budget.emit("combat_lane_started", nil, fmt.Sprintf("lane=%d", lane)); err != nil {
			return err
		}
		if err := resolveLane(state, lane, budget); err != nil || state.Result != nil {
			return err
		}
	}
	state.CombatStep = CombatEnd
	state.CombatLane = -1
	if err := budget.emit("combat_ended", nil, "all five lanes resolved"); err != nil {
		return err
	}
	expireEffects(state, DurationUntilEndOfCombat)
	state.Phase = PhaseEnd
	state.CombatStep = CombatNone
	if err := budget.emit("phase_started", nil, string(PhaseEnd)); err != nil {
		return err
	}
	state.Phase = PhaseRecovery
	if err := budget.emit("phase_started", nil, string(PhaseRecovery)); err != nil {
		return err
	}
	expireEffects(state, DurationUntilEndOfTurn)
	recoverState(state)
	state.InitiativeSeat = state.InitiativeSeat.Other()
	state.TurnNumber++
	if err := budget.emit("recovery_completed", nil, fmt.Sprintf("initiative=%d", state.InitiativeSeat)); err != nil {
		return err
	}
	return beginTurn(state, budget)
}

func resolveLane(state *State, lane int, budget *eventBudget) error {
	unitA := state.boardObject(SeatA, lane, SlotUnit)
	unitB := state.boardObject(SeatB, lane, SlotUnit)
	if unitA != nil && unitB != nil {
		damageToA := unitB.Power
		damageToB := unitA.Power
		unitA.Power -= damageToA
		unitB.Power -= damageToB
		unitADied := unitA.Power <= 0
		unitBDied := unitB.Power <= 0
		if unitADied {
			moveToGraveyard(state, unitA)
		}
		if unitBDied {
			moveToGraveyard(state, unitB)
		}
		if err := budget.emit("units_collided", nil, fmt.Sprintf("lane=%d", lane)); err != nil {
			return err
		}
		if unitADied {
			if err := budget.emit("unit_died", seatPointer(SeatA), fmt.Sprintf("lane=%d", lane)); err != nil {
				return err
			}
		}
		if unitBDied {
			if err := budget.emit("unit_died", seatPointer(SeatB), fmt.Sprintf("lane=%d", lane)); err != nil {
				return err
			}
		}
		return nil
	}
	if unitA != nil {
		return resolveUnopposedAttack(state, unitA, SeatB, lane, budget)
	}
	if unitB != nil {
		return resolveUnopposedAttack(state, unitB, SeatA, lane, budget)
	}
	return nil
}

func resolveUnopposedAttack(state *State, attacker *CardObject, defender Seat, lane int, budget *eventBudget) error {
	location := state.boardObject(defender, -1, SlotLocation)
	if location != nil {
		location.Durability -= attacker.Power
		destroyed := location.Durability <= 0
		if destroyed {
			moveToGraveyard(state, location)
		}
		if err := budget.emit("location_hit", &defender, fmt.Sprintf("lane=%d damage=%d", lane, attacker.Power)); err != nil {
			return err
		}
		if destroyed {
			return budget.emit("location_destroyed", &defender, fmt.Sprintf("lane=%d", lane))
		}
		return nil
	}
	if attacker.EnteredInCombat {
		return budget.emit("hero_attack_skipped", &defender, "attacker entered or changed field during combat")
	}
	if err := budget.emit("hero_hit", &defender, fmt.Sprintf("lane=%d", lane)); err != nil {
		return err
	}
	return revealOrder(state, defender, "hero_hit", budget)
}

func revealOrder(state *State, seat Seat, cause string, budget *eventBudget) error {
	if state.Result != nil {
		return nil
	}
	order := state.nextClosedOrder(seat)
	if order == nil {
		winner := seat.Other()
		loser := seat
		state.finish(ResultRulesLoss, &winner, &loser, "required Order unavailable: "+cause)
		return budget.emit("match_finished", &loser, "required_order_unavailable")
	}
	order.Revealed = true
	return budget.emit("order_revealed", &seat, order.DefinitionID+":"+cause)
}

func recoverState(state *State) {
	targets := make([]ObjectRef, 0)
	for playerIndex := range state.Players {
		player := &state.Players[playerIndex]
		player.Influence = player.BaseInfluence
		for objectIndex := range player.Objects {
			object := &player.Objects[objectIndex]
			object.Tapped = false
			object.EnteredInCombat = false
			if object.Zone != ZoneBoard {
				continue
			}
			if object.Slot == SlotUnit {
				object.Power = object.BasePower
				targets = append(targets, object.Ref())
			}
			if object.Slot == SlotLocation {
				object.Durability = object.BaseDurability
			}
		}
	}
	for _, target := range targets {
		recalculatePower(state, target)
	}
}

func moveToGraveyard(state *State, object *CardObject) {
	graveyard := state.objectsInZone(object.OwnerSeat, ZoneGraveyard)
	object.ControllerSeat = object.OwnerSeat
	moveObject(object, ZoneGraveyard, len(graveyard))
}

func seatPointer(seat Seat) *Seat {
	value := seat
	return &value
}
