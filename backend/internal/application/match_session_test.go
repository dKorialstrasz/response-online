package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

func TestMatchSessionHasOneTransactionalWriter(t *testing.T) {
	match, err := engine.NewMatch(sessionTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewMatchSession(match, 8)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- session.Run(ctx) }()

	setup := []engine.Command{
		{ID: "orders-a", Kind: engine.CommandArrangeOrders, Seat: engine.SeatA, ExpectedRevision: 0, OrderIDs: sessionOrders(engine.SeatA)},
		{ID: "orders-b", Kind: engine.CommandArrangeOrders, Seat: engine.SeatB, ExpectedRevision: 1, OrderIDs: sessionOrders(engine.SeatB)},
		{ID: "mulligan-a", Kind: engine.CommandMulligan, Seat: engine.SeatA, ExpectedRevision: 2},
		{ID: "mulligan-b", Kind: engine.CommandMulligan, Seat: engine.SeatB, ExpectedRevision: 3},
	}
	for _, command := range setup {
		if err := session.Submit(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	before, err := session.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	commands := []engine.Command{
		{ID: "racing-pass-1", Kind: engine.CommandPass, Seat: before.ActingSeat, ExpectedRevision: before.Revision},
		{ID: "racing-pass-2", Kind: engine.CommandPass, Seat: before.ActingSeat, ExpectedRevision: before.Revision},
	}
	var wait sync.WaitGroup
	errorsSeen := make(chan error, len(commands))
	for _, command := range commands {
		wait.Add(1)
		go func(command engine.Command) {
			defer wait.Done()
			errorsSeen <- session.Submit(ctx, command)
		}(command)
	}
	wait.Wait()
	close(errorsSeen)
	successes, stale := 0, 0
	for submitErr := range errorsSeen {
		switch {
		case submitErr == nil:
			successes++
		case errors.Is(submitErr, engine.ErrStaleRevision):
			stale++
		default:
			t.Fatalf("unexpected submit error: %v", submitErr)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("successes=%d stale=%d, want 1/1", successes, stale)
	}
	after, err := session.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision+1 {
		t.Fatalf("revision=%d, want %d", after.Revision, before.Revision+1)
	}
	cancel()
	<-finished
}

func sessionTestConfig() engine.Config {
	players := [2]engine.PlayerConfig{}
	for seat := engine.SeatA; seat <= engine.SeatB; seat++ {
		deck := make([]string, 30)
		for index := range deck {
			deck[index] = fmt.Sprintf("s%d-card-%d", seat, index)
		}
		players[int(seat)] = engine.PlayerConfig{
			Seat: seat, HeroID: fmt.Sprintf("s%d-hero", seat), DeckCardIDs: deck,
			AvailableIDs: sessionOrders(seat), BaseInfluence: 5,
		}
	}
	return engine.Config{
		MatchID: "session-test",
		Versions: engine.Versions{
			Content: "content-v1", DeckManifests: "decks-v1", EffectSpec: "effects-v1",
			Rules: "rules-v1", Protocol: "protocol-v1", Engine: "engine-v1",
		},
		Players: players, Seed: 7, StartedAt: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC),
	}
}

func sessionOrders(seat engine.Seat) []string {
	orders := make([]string, engine.OrdersPerPlayer)
	for index := range orders {
		orders[index] = fmt.Sprintf("s%d-order-%d", seat, index)
	}
	return orders
}
