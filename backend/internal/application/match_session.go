package application

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

// MatchSession is the only application-level writer for one deterministic
// match. Transport handlers submit commands here instead of mutating engine
// state directly.
type MatchSession struct {
	owner       *MatchOwner
	match       *engine.Match
	clock       Clock
	policy      MatchRuntimePolicy
	players     [2]matchPlayerRuntime
	idle        [2]idleRuntime
	subscribers map[uint64]lifecycleSubscriber
	nextSubID   uint64
	runContext  context.Context
	timer       *time.Timer
}

func NewMatchSession(match *engine.Match, mailboxCapacity int) (*MatchSession, error) {
	return NewMatchSessionWithConfig(match, MatchSessionConfig{MailboxCapacity: mailboxCapacity})
}

type MatchSessionConfig struct {
	MailboxCapacity int
	Clock           Clock
	Policy          MatchRuntimePolicy
}

func NewMatchSessionWithConfig(match *engine.Match, config MatchSessionConfig) (*MatchSession, error) {
	if match == nil {
		return nil, errors.New("match is required")
	}
	if config.Clock == nil {
		config.Clock = wallClock{}
	}
	policy, err := normalizeMatchRuntimePolicy(config.Policy)
	if err != nil {
		return nil, err
	}
	session := &MatchSession{
		owner: NewMatchOwner(config.MailboxCapacity), match: match, clock: config.Clock,
		policy: policy, subscribers: make(map[uint64]lifecycleSubscriber),
	}
	session.updateIdleWaiters(match.State(), config.Clock.Now(), nil)
	return session, nil
}

func (session *MatchSession) Run(ctx context.Context) error {
	session.runContext = ctx
	session.scheduleNextDeadline()
	return session.owner.Run(ctx)
}

func (session *MatchSession) Submit(ctx context.Context, command engine.Command) error {
	return session.owner.Submit(ctx, func(context.Context) error {
		now := session.clock.Now()
		session.processDue(now)
		before := session.match.State()
		if before.Status == engine.StatusFinished {
			return engine.ErrMatchFinished
		}
		if err := session.match.Handle(command); err != nil {
			session.scheduleNextDeadline()
			return err
		}
		after := session.match.State()
		session.updateIdleWaiters(after, now, &command.Seat)
		session.scheduleNextDeadline()
		return nil
	})
}

func (session *MatchSession) Snapshot(ctx context.Context) (engine.State, error) {
	var snapshot engine.State
	err := session.owner.Submit(ctx, func(context.Context) error {
		session.processDue(session.clock.Now())
		snapshot = session.match.State()
		session.scheduleNextDeadline()
		return nil
	})
	return snapshot, err
}

// GameplaySnapshot returns state and legal actions from one serialized owner
// operation, so a projection can never combine different revisions.
func (session *MatchSession) GameplaySnapshot(ctx context.Context, seat engine.Seat) (engine.State, []engine.LegalAction, error) {
	var snapshot engine.State
	var actions []engine.LegalAction
	err := session.owner.Submit(ctx, func(context.Context) error {
		session.processDue(session.clock.Now())
		snapshot = session.match.State()
		actions = session.match.LegalActions(seat)
		session.scheduleNextDeadline()
		return nil
	})
	return snapshot, actions, err
}

// CheckDeadlines is the deterministic virtual-clock seam used by tests and by
// operational diagnostics. Production also arms the nearest monotonic timer.
func (session *MatchSession) CheckDeadlines(ctx context.Context) error {
	return session.owner.Submit(ctx, func(context.Context) error {
		session.processDue(session.clock.Now())
		session.scheduleNextDeadline()
		return nil
	})
}

func (session *MatchSession) Connected(ctx context.Context, seat engine.Seat) error {
	return session.owner.Submit(ctx, func(context.Context) error {
		if !seat.Valid() {
			return fmt.Errorf("invalid seat")
		}
		now := session.clock.Now()
		session.processDue(now)
		if session.match.State().Status == engine.StatusFinished {
			return engine.ErrMatchFinished
		}
		player := &session.players[int(seat)]
		wasDisconnected := player.disconnectedAt.IsZero() == false
		if wasDisconnected {
			player.reconnectSpent += now.Sub(player.disconnectedAt)
			player.disconnectedAt = time.Time{}
			player.reconnectDeadline = time.Time{}
		}
		player.connected = true
		player.everConnected = true
		player.lastHeartbeat = now
		if wasDisconnected {
			session.publish(LifecycleEvent{Message: UXConnectionRestored, SnapshotRequired: true})
		}
		session.scheduleNextDeadline()
		return nil
	})
}

func (session *MatchSession) Heartbeat(ctx context.Context, seat engine.Seat) error {
	return session.owner.Submit(ctx, func(context.Context) error {
		if !seat.Valid() {
			return fmt.Errorf("invalid seat")
		}
		now := session.clock.Now()
		session.processDue(now)
		player := &session.players[int(seat)]
		if !player.connected || session.match.State().Status == engine.StatusFinished {
			return ErrConnectionTimedOut
		}
		player.lastHeartbeat = now
		session.scheduleNextDeadline()
		return nil
	})
}

func (session *MatchSession) Subscribe(ctx context.Context, seat engine.Seat) (uint64, <-chan LifecycleEvent, error) {
	var id uint64
	var events chan LifecycleEvent
	err := session.owner.Submit(ctx, func(context.Context) error {
		if !seat.Valid() {
			return fmt.Errorf("invalid seat")
		}
		id = atomic.AddUint64(&session.nextSubID, 1)
		events = make(chan LifecycleEvent, 16)
		session.subscribers[id] = lifecycleSubscriber{seat: seat, events: events}
		return nil
	})
	return id, events, err
}

func (session *MatchSession) Unsubscribe(ctx context.Context, id uint64) {
	_ = session.owner.Submit(ctx, func(context.Context) error {
		if subscriber, ok := session.subscribers[id]; ok {
			delete(session.subscribers, id)
			close(subscriber.events)
		}
		return nil
	})
}

func (session *MatchSession) RuntimeSnapshot(ctx context.Context) (MatchRuntimeSnapshot, error) {
	var snapshot MatchRuntimeSnapshot
	err := session.owner.Submit(ctx, func(context.Context) error {
		now := session.clock.Now()
		session.processDue(now)
		for index := range session.players {
			player := session.players[index]
			spent := player.reconnectSpent
			if !player.disconnectedAt.IsZero() {
				spent += now.Sub(player.disconnectedAt)
			}
			snapshot.Players[index] = PlayerRuntimeSnapshot{
				Connected: player.connected, ReconnectSpent: spent,
				ReconnectRemaining: maxDuration(0, session.policy.ReconnectBudget-spent),
			}
		}
		snapshot.Policy = session.policy
		session.scheduleNextDeadline()
		return nil
	})
	return snapshot, err
}

func (session *MatchSession) ReplaceMatch(ctx context.Context, next *engine.Match) error {
	if next == nil {
		return errors.New("replacement match is required")
	}
	return session.owner.Submit(ctx, func(context.Context) error {
		now := session.clock.Now()
		session.match = next
		for index := range session.players {
			connected := session.players[index].connected
			everConnected := session.players[index].everConnected
			lastHeartbeat := session.players[index].lastHeartbeat
			session.players[index] = matchPlayerRuntime{
				connected: connected, everConnected: everConnected, lastHeartbeat: lastHeartbeat,
			}
		}
		session.updateIdleWaiters(next.State(), now, nil)
		session.scheduleNextDeadline()
		return nil
	})
}
