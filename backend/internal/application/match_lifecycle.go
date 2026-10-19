package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

const (
	UXOwnConnectionLost       = "Связь с сервером потеряна. Пытаемся восстановить соединение. На восстановление отведено до 20 секунд"
	UXOpponentConnectionLost  = "Связь с соперником потеряна. Ожидаем восстановление до 20 секунд"
	UXConnectionRestored      = "Соединение восстановлено. Состояние игры обновлено"
	UXDisconnectForfeit       = "Игрок не восстановил соединение за 20 секунд. Ему засчитано техническое поражение"
	UXBothDisconnected        = "Оба игрока не восстановили соединение. Матч завершён технически без победителя"
	UXIdleWarningActor        = "Нет принятого действия уже 30 секунд. Если сервер не примет действие в течение следующих 60 секунд, матч завершится технически без победителя"
	UXIdleWarningOpponent     = "Ожидаем действие соперника. Если бездействие продолжится, матч завершится технически без победителя"
	UXIdleTechnicalCompletion = "Матч завершён технически из-за бездействия. Победитель не определяется"
)

var ErrConnectionTimedOut = errors.New("connection heartbeat timed out")

type MatchRuntimePolicy struct {
	HeartbeatInterval time.Duration `json:"heartbeatInterval"`
	DisconnectAfter   time.Duration `json:"disconnectAfter"`
	ReconnectWindow   time.Duration `json:"reconnectWindow"`
	ReconnectBudget   time.Duration `json:"reconnectBudget"`
	IdleWarningAfter  time.Duration `json:"idleWarningAfter"`
	IdleTimeoutAfter  time.Duration `json:"idleTimeoutAfter"`
}

func normalizeMatchRuntimePolicy(policy MatchRuntimePolicy) (MatchRuntimePolicy, error) {
	defaults := MatchRuntimePolicy{
		HeartbeatInterval: time.Second, DisconnectAfter: 5 * time.Second,
		ReconnectWindow: 20 * time.Second, ReconnectBudget: 60 * time.Second,
		IdleWarningAfter: 30 * time.Second, IdleTimeoutAfter: 90 * time.Second,
	}
	if policy == (MatchRuntimePolicy{}) {
		return defaults, nil
	}
	values := []*time.Duration{
		&policy.HeartbeatInterval, &policy.DisconnectAfter, &policy.ReconnectWindow,
		&policy.ReconnectBudget, &policy.IdleWarningAfter, &policy.IdleTimeoutAfter,
	}
	defaultValues := []time.Duration{
		defaults.HeartbeatInterval, defaults.DisconnectAfter, defaults.ReconnectWindow,
		defaults.ReconnectBudget, defaults.IdleWarningAfter, defaults.IdleTimeoutAfter,
	}
	for index, value := range values {
		if *value == 0 {
			*value = defaultValues[index]
		}
		if *value < 0 {
			return MatchRuntimePolicy{}, fmt.Errorf("runtime durations must be positive")
		}
	}
	if policy.IdleWarningAfter >= policy.IdleTimeoutAfter {
		return MatchRuntimePolicy{}, fmt.Errorf("idle warning must precede idle timeout")
	}
	return policy, nil
}

type LifecycleEvent struct {
	Message          string
	Audience         *engine.Seat
	SnapshotRequired bool
	MatchFinished    bool
}

type lifecycleSubscriber struct {
	seat   engine.Seat
	events chan LifecycleEvent
}

type matchPlayerRuntime struct {
	connected         bool
	everConnected     bool
	lastHeartbeat     time.Time
	disconnectedAt    time.Time
	reconnectDeadline time.Time
	reconnectSpent    time.Duration
}

type idleRuntime struct {
	waiting bool
	since   time.Time
	warned  bool
}

type PlayerRuntimeSnapshot struct {
	Connected          bool          `json:"connected"`
	ReconnectSpent     time.Duration `json:"reconnectSpent"`
	ReconnectRemaining time.Duration `json:"reconnectRemaining"`
}

type MatchRuntimeSnapshot struct {
	Policy  MatchRuntimePolicy       `json:"policy"`
	Players [2]PlayerRuntimeSnapshot `json:"players"`
}

func (session *MatchSession) processDue(now time.Time) {
	state := session.match.State()
	if state.Status == engine.StatusFinished {
		return
	}
	for index := range session.players {
		player := &session.players[index]
		if player.connected && player.everConnected && !now.Before(player.lastHeartbeat.Add(session.policy.DisconnectAfter)) {
			seat := engine.Seat(index)
			player.connected = false
			player.disconnectedAt = player.lastHeartbeat.Add(session.policy.DisconnectAfter)
			remaining := session.policy.ReconnectBudget - player.reconnectSpent
			window := minDuration(session.policy.ReconnectWindow, remaining)
			player.reconnectDeadline = player.disconnectedAt.Add(window)
			session.publish(LifecycleEvent{Message: UXOwnConnectionLost, Audience: seatPointer(seat)})
			session.publish(LifecycleEvent{Message: UXOpponentConnectionLost, Audience: seatPointer(seat.Other())})
		}
	}
	for index := range session.players {
		player := &session.players[index]
		if player.disconnectedAt.IsZero() || now.Before(player.reconnectDeadline) {
			continue
		}
		seat := engine.Seat(index)
		other := &session.players[int(seat.Other())]
		if !other.disconnectedAt.IsZero() {
			if session.match.FinishTechnical(engine.ResultAbortedTechnical, nil, nil, "both players exceeded reconnect window") == nil {
				session.publish(LifecycleEvent{Message: UXBothDisconnected, SnapshotRequired: true, MatchFinished: true})
			}
		} else {
			winner, loser := seat.Other(), seat
			if session.match.FinishTechnical(engine.ResultDisconnectForfeit, &winner, &loser, "player exceeded reconnect window or budget") == nil {
				session.publish(LifecycleEvent{Message: UXDisconnectForfeit, SnapshotRequired: true, MatchFinished: true})
			}
		}
		return
	}
	for index := range session.idle {
		idle := &session.idle[index]
		if !idle.waiting {
			continue
		}
		seat := engine.Seat(index)
		if !idle.warned && !now.Before(idle.since.Add(session.policy.IdleWarningAfter)) {
			idle.warned = true
			session.publish(LifecycleEvent{Message: UXIdleWarningActor, Audience: seatPointer(seat)})
			session.publish(LifecycleEvent{Message: UXIdleWarningOpponent, Audience: seatPointer(seat.Other())})
		}
		if !now.Before(idle.since.Add(session.policy.IdleTimeoutAfter)) {
			if session.match.FinishTechnical(engine.ResultAbortedTechnical, nil, nil, "accepted action timeout") == nil {
				session.publish(LifecycleEvent{Message: UXIdleTechnicalCompletion, SnapshotRequired: true, MatchFinished: true})
			}
			return
		}
	}
}

func (session *MatchSession) updateIdleWaiters(state engine.State, now time.Time, acceptedSeat *engine.Seat) {
	var expected [2]bool
	if state.Status != engine.StatusFinished {
		if len(state.PendingChoices) > 0 {
			for _, choice := range state.PendingChoices {
				if choice.Seat.Valid() {
					expected[int(choice.Seat)] = true
				}
			}
		} else {
			switch state.Phase {
			case engine.PhaseOrderSelection:
				for index := range state.Players {
					expected[index] = !state.Players[index].OrdersLocked
				}
			case engine.PhaseMulligan:
				for index := range state.Players {
					expected[index] = !state.Players[index].MulliganSubmitted
				}
			case engine.PhaseAction:
				expected[int(state.ActingSeat)] = true
			}
		}
	}
	for index := range session.idle {
		idle := &session.idle[index]
		if !expected[index] {
			*idle = idleRuntime{}
			continue
		}
		reset := !idle.waiting || (acceptedSeat != nil && int(*acceptedSeat) == index)
		if reset {
			*idle = idleRuntime{waiting: true, since: now}
		}
	}
}

func (session *MatchSession) publish(event LifecycleEvent) {
	for _, subscriber := range session.subscribers {
		if event.Audience != nil && subscriber.seat != *event.Audience {
			continue
		}
		select {
		case subscriber.events <- event:
		default:
		}
	}
}

func (session *MatchSession) scheduleNextDeadline() {
	if session.runContext == nil || session.runContext.Err() != nil {
		return
	}
	if session.timer != nil {
		session.timer.Stop()
		session.timer = nil
	}
	state := session.match.State()
	if state.Status == engine.StatusFinished {
		return
	}
	var next time.Time
	consider := func(candidate time.Time) {
		if candidate.IsZero() {
			return
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	for index := range session.players {
		player := session.players[index]
		if player.connected && player.everConnected {
			consider(player.lastHeartbeat.Add(session.policy.DisconnectAfter))
		} else {
			consider(player.reconnectDeadline)
		}
	}
	for _, idle := range session.idle {
		if !idle.waiting {
			continue
		}
		if !idle.warned {
			consider(idle.since.Add(session.policy.IdleWarningAfter))
		}
		consider(idle.since.Add(session.policy.IdleTimeoutAfter))
	}
	if next.IsZero() {
		return
	}
	delay := maxDuration(0, next.Sub(session.clock.Now()))
	ctx := session.runContext
	session.timer = time.AfterFunc(delay, func() {
		_ = session.owner.Submit(ctx, func(context.Context) error {
			session.processDue(session.clock.Now())
			session.scheduleNextDeadline()
			return nil
		})
	})
}

func seatPointer(seat engine.Seat) *engine.Seat { return &seat }
func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
func maxDuration(left, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}
