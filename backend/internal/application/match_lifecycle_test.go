package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

type lifecycleClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *lifecycleClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *lifecycleClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(duration)
	clock.mu.Unlock()
}

func TestRuntimePolicyAndReconnectBoundaries(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, false)
	defer cancel()
	ctx := context.Background()

	runtime, err := session.RuntimeSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	policy := runtime.Policy
	if policy.HeartbeatInterval != time.Second || policy.DisconnectAfter != 5*time.Second ||
		policy.ReconnectWindow != 20*time.Second || policy.ReconnectBudget != 60*time.Second ||
		policy.IdleWarningAfter != 30*time.Second || policy.IdleTimeoutAfter != 90*time.Second {
		t.Fatalf("unexpected runtime policy: %+v", policy)
	}

	if err := session.Connected(ctx, engine.SeatA); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if err := session.Heartbeat(ctx, engine.SeatA); err != nil {
		t.Fatalf("heartbeat at the one-second boundary: %v", err)
	}
	clock.Advance(5*time.Second - time.Nanosecond)
	if err := session.CheckDeadlines(ctx); err != nil {
		t.Fatal(err)
	}
	runtime, _ = session.RuntimeSnapshot(ctx)
	if !runtime.Players[0].Connected {
		t.Fatal("connection was detected before five seconds from the last heartbeat")
	}
	clock.Advance(time.Nanosecond)
	_ = session.CheckDeadlines(ctx)
	runtime, _ = session.RuntimeSnapshot(ctx)
	if runtime.Players[0].Connected {
		t.Fatal("connection remained active at the five-second boundary")
	}

	clock.Advance(20*time.Second - time.Nanosecond)
	if err := session.Connected(ctx, engine.SeatA); err != nil {
		t.Fatalf("reconnect immediately before deadline: %v", err)
	}
	state, _ := session.Snapshot(ctx)
	if state.Result != nil {
		t.Fatalf("pre-deadline reconnect produced result: %+v", state.Result)
	}
}

func TestReconnectAtDeadlineLosesDeterministically(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, false)
	defer cancel()
	ctx := context.Background()
	_ = session.Connected(ctx, engine.SeatA)
	clock.Advance(5 * time.Second)
	_ = session.CheckDeadlines(ctx)
	clock.Advance(20 * time.Second)
	errorsSeen := make(chan error, 2)
	go func() { errorsSeen <- session.Connected(ctx, engine.SeatA) }()
	go func() { errorsSeen <- session.CheckDeadlines(ctx) }()
	for range 2 {
		err := <-errorsSeen
		if err != nil && !errors.Is(err, engine.ErrMatchFinished) {
			t.Fatalf("reconnect/timeout race error=%v", err)
		}
	}
	state, _ := session.Snapshot(ctx)
	if state.Result == nil || state.Result.Reason != engine.ResultDisconnectForfeit ||
		state.Result.WinnerSeat == nil || *state.Result.WinnerSeat != engine.SeatB {
		t.Fatalf("deadline result=%+v", state.Result)
	}
}

func TestReconnectAfterDeadlineLoses(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, false)
	defer cancel()
	ctx := context.Background()
	_ = session.Connected(ctx, engine.SeatA)
	clock.Advance(5 * time.Second)
	_ = session.CheckDeadlines(ctx)
	clock.Advance(20*time.Second + time.Nanosecond)
	if err := session.Connected(ctx, engine.SeatA); !errors.Is(err, engine.ErrMatchFinished) {
		t.Fatalf("reconnect after deadline error=%v, want match finished", err)
	}
	state, _ := session.Snapshot(ctx)
	if state.Result == nil || state.Result.Reason != engine.ResultDisconnectForfeit {
		t.Fatalf("post-deadline result=%+v", state.Result)
	}
}

func TestRepeatedDisconnectsConsumeSixtySecondBudget(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, false)
	defer cancel()
	ctx := context.Background()
	_ = session.Connected(ctx, engine.SeatA)
	for cycle := 0; cycle < 3; cycle++ {
		clock.Advance(5 * time.Second)
		_ = session.CheckDeadlines(ctx)
		clock.Advance(19 * time.Second)
		if err := session.Connected(ctx, engine.SeatA); err != nil {
			t.Fatalf("cycle %d reconnect: %v", cycle, err)
		}
	}
	runtime, _ := session.RuntimeSnapshot(ctx)
	if runtime.Players[0].ReconnectSpent != 57*time.Second || runtime.Players[0].ReconnectRemaining != 3*time.Second {
		t.Fatalf("budget after repeated reconnects: %+v", runtime.Players[0])
	}
	clock.Advance(5 * time.Second)
	_ = session.CheckDeadlines(ctx)
	clock.Advance(3*time.Second - time.Nanosecond)
	_ = session.CheckDeadlines(ctx)
	state, _ := session.Snapshot(ctx)
	if state.Result != nil {
		t.Fatal("budget expired before the exact sixty-second boundary")
	}
	clock.Advance(time.Nanosecond)
	_ = session.CheckDeadlines(ctx)
	state, _ = session.Snapshot(ctx)
	if state.Result == nil || state.Result.Reason != engine.ResultDisconnectForfeit {
		t.Fatalf("budget result=%+v", state.Result)
	}
}

func TestBothPlayersMissingProduceTechnicalAbortWithoutWinner(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, false)
	defer cancel()
	ctx := context.Background()
	_ = session.Connected(ctx, engine.SeatA)
	_ = session.Connected(ctx, engine.SeatB)
	clock.Advance(5 * time.Second)
	_ = session.CheckDeadlines(ctx)
	clock.Advance(20 * time.Second)
	_ = session.CheckDeadlines(ctx)
	state, _ := session.Snapshot(ctx)
	if state.Result == nil || state.Result.Reason != engine.ResultAbortedTechnical ||
		state.Result.WinnerSeat != nil || state.Result.LoserSeat != nil {
		t.Fatalf("both-disconnected result=%+v", state.Result)
	}
}

func TestIdleWarningAndTimeoutUseApprovedTexts(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, true)
	defer cancel()
	ctx := context.Background()
	state, _ := session.Snapshot(ctx)
	actor := state.ActingSeat
	_, events, err := session.Subscribe(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}

	clock.Advance(30*time.Second - time.Nanosecond)
	_ = session.CheckDeadlines(ctx)
	select {
	case event := <-events:
		t.Fatalf("warning arrived early: %+v", event)
	default:
	}
	clock.Advance(time.Nanosecond)
	_ = session.CheckDeadlines(ctx)
	if event := <-events; event.Message != UXIdleWarningActor {
		t.Fatalf("warning=%q", event.Message)
	}
	clock.Advance(60*time.Second - time.Nanosecond)
	_ = session.CheckDeadlines(ctx)
	state, _ = session.Snapshot(ctx)
	if state.Result != nil {
		t.Fatal("idle timeout fired early")
	}
	clock.Advance(time.Nanosecond)
	_ = session.CheckDeadlines(ctx)
	state, _ = session.Snapshot(ctx)
	if state.Result == nil || state.Result.Reason != engine.ResultAbortedTechnical || state.Result.WinnerSeat != nil {
		t.Fatalf("idle result=%+v", state.Result)
	}
	if event := <-events; event.Message != UXIdleTechnicalCompletion || !event.MatchFinished {
		t.Fatalf("completion event=%+v", event)
	}
}

func TestAcceptedActionWinsBeforeIdleDeadlineButNotAtDeadline(t *testing.T) {
	ctx := context.Background()
	beforeSession, beforeClock, beforeCancel := newLifecycleSession(t, true)
	defer beforeCancel()
	state, _ := beforeSession.Snapshot(ctx)
	beforeClock.Advance(90*time.Second - time.Nanosecond)
	err := beforeSession.Submit(ctx, engine.Command{
		ID: "just-before-idle", Kind: engine.CommandPass, Seat: state.ActingSeat, ExpectedRevision: state.Revision,
	})
	if err != nil {
		t.Fatalf("accepted action immediately before timeout: %v", err)
	}
	beforeClock.Advance(time.Nanosecond)
	_ = beforeSession.CheckDeadlines(ctx)
	state, _ = beforeSession.Snapshot(ctx)
	if state.Result != nil {
		t.Fatalf("accepted action did not reset idle timer: %+v", state.Result)
	}

	atSession, atClock, atCancel := newLifecycleSession(t, true)
	defer atCancel()
	state, _ = atSession.Snapshot(ctx)
	atClock.Advance(90 * time.Second)
	err = atSession.Submit(ctx, engine.Command{
		ID: "at-idle-deadline", Kind: engine.CommandPass, Seat: state.ActingSeat, ExpectedRevision: state.Revision,
	})
	if !errors.Is(err, engine.ErrMatchFinished) {
		t.Fatalf("action at deadline error=%v, want match finished", err)
	}
	state, _ = atSession.Snapshot(ctx)
	if state.Result == nil || state.Result.Reason != engine.ResultAbortedTechnical {
		t.Fatalf("at-deadline result=%+v", state.Result)
	}
}

func TestRejectedActionDoesNotResetIdleTimer(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, true)
	defer cancel()
	ctx := context.Background()
	state, _ := session.Snapshot(ctx)
	clock.Advance(89 * time.Second)
	err := session.Submit(ctx, engine.Command{
		ID: "rejected-stale", Kind: engine.CommandPass, Seat: state.ActingSeat,
		ExpectedRevision: state.Revision - 1,
	})
	if !errors.Is(err, engine.ErrStaleRevision) {
		t.Fatalf("rejected action error=%v", err)
	}
	clock.Advance(time.Second)
	_ = session.CheckDeadlines(ctx)
	state, _ = session.Snapshot(ctx)
	if state.Result == nil || state.Result.Reason != engine.ResultAbortedTechnical {
		t.Fatalf("rejected action reset idle timer: %+v", state.Result)
	}
}

func TestConcurrentTerminalPathsCommitExactlyOneResult(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, true)
	defer cancel()
	ctx := context.Background()
	before, _ := session.Snapshot(ctx)
	clock.Advance(90 * time.Second)
	errorsSeen := make(chan error, 2)
	go func() { errorsSeen <- session.CheckDeadlines(ctx) }()
	go func() {
		errorsSeen <- session.Submit(ctx, engine.Command{
			ID: "racing-surrender", Kind: engine.CommandSurrender,
			Seat: before.ActingSeat, ExpectedRevision: before.Revision,
		})
	}()
	for range 2 {
		err := <-errorsSeen
		if err != nil && !errors.Is(err, engine.ErrMatchFinished) {
			t.Fatalf("unexpected race error: %v", err)
		}
	}
	after, _ := session.Snapshot(ctx)
	if after.Result == nil || after.Result.Reason != engine.ResultAbortedTechnical || after.Revision != before.Revision+1 {
		t.Fatalf("terminal race result=%+v revision=%d want=%d", after.Result, after.Revision, before.Revision+1)
	}
}

func TestCompetingDisconnectAndIdleTimeoutsCommitOneResult(t *testing.T) {
	session, clock, cancel := newLifecycleSession(t, true)
	defer cancel()
	ctx := context.Background()
	before, _ := session.Snapshot(ctx)
	clock.Advance(65 * time.Second)
	if err := session.Connected(ctx, engine.SeatA); err != nil {
		t.Fatal(err)
	}
	clock.Advance(25 * time.Second)
	if err := session.CheckDeadlines(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := session.Snapshot(ctx)
	if after.Result == nil || after.Result.Reason != engine.ResultDisconnectForfeit ||
		after.Revision != before.Revision+1 {
		t.Fatalf("competing technical outcomes result=%+v revision=%d want=%d", after.Result, after.Revision, before.Revision+1)
	}
}

func newLifecycleSession(t *testing.T, active bool) (*MatchSession, *lifecycleClock, context.CancelFunc) {
	t.Helper()
	match, err := engine.NewMatch(sessionTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	clock := &lifecycleClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	session, err := NewMatchSessionWithConfig(match, MatchSessionConfig{MailboxCapacity: 32, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = session.Run(ctx) }()
	commands := []engine.Command{
		{ID: "orders-a", Kind: engine.CommandArrangeOrders, Seat: engine.SeatA, ExpectedRevision: 0, OrderIDs: sessionOrders(engine.SeatA)},
		{ID: "orders-b", Kind: engine.CommandArrangeOrders, Seat: engine.SeatB, ExpectedRevision: 1, OrderIDs: sessionOrders(engine.SeatB)},
	}
	if active {
		commands = append(commands,
			engine.Command{ID: "mulligan-a", Kind: engine.CommandMulligan, Seat: engine.SeatA, ExpectedRevision: 2},
			engine.Command{ID: "mulligan-b", Kind: engine.CommandMulligan, Seat: engine.SeatB, ExpectedRevision: 3},
		)
	}
	for _, command := range commands {
		if err := session.Submit(ctx, command); err != nil {
			cancel()
			t.Fatalf("setup command %s: %v", command.ID, err)
		}
	}
	return session, clock, cancel
}
