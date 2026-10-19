package application

import (
	"context"
	"testing"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

func TestRematchRequiresBothPlayersAndPreservesPriorResult(t *testing.T) {
	service, _, _ := newRoomTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	one := mustCreateSession(t, service)
	two := mustCreateSession(t, service)
	created, err := service.CreateRoom(credentialsFor(one), "rematch-create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.JoinRoom(credentialsFor(two), created.Room.Code, "rematch-join"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetSelection(credentialsFor(one), created.Room.RoomID, testSelection("deck-a")); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetSelection(credentialsFor(two), created.Room.RoomID, testSelection("deck-b")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.SetReady(ctx, credentialsFor(one), created.Room.RoomID, true); err != nil {
		t.Fatal(err)
	}
	_, started, err := service.SetReady(ctx, credentialsFor(two), created.Room.RoomID, true)
	if err != nil || started == nil {
		t.Fatalf("start match: %+v %v", started, err)
	}
	session, err := service.MatchSession(credentialsFor(one), created.Room.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = session.Run(ctx) }()
	for _, command := range []engine.Command{
		{ID: "mulligan-a", Kind: engine.CommandMulligan, Seat: engine.SeatA, ExpectedRevision: 2},
		{ID: "mulligan-b", Kind: engine.CommandMulligan, Seat: engine.SeatB, ExpectedRevision: 3},
		{ID: "surrender-a", Kind: engine.CommandSurrender, Seat: engine.SeatA, ExpectedRevision: 4},
	} {
		if err := session.Submit(ctx, command); err != nil {
			t.Fatalf("%s: %v", command.ID, err)
		}
	}
	claimOne, err := service.ReplaceConnection(credentialsFor(one), created.Room.RoomID, "rematch-one")
	if err != nil {
		t.Fatal(err)
	}
	claimTwo, err := service.ReplaceConnection(credentialsFor(two), created.Room.RoomID, "rematch-two")
	if err != nil {
		t.Fatal(err)
	}

	room, next, err := service.RequestRematchForConnection(ctx, claimOne.Lease, started.MatchID, true)
	if err != nil || next != nil || !room.Seats[0].RematchReady {
		t.Fatalf("first ready: room=%+v next=%+v err=%v", room, next, err)
	}
	room, next, err = service.RequestRematchForConnection(ctx, claimTwo.Lease, started.MatchID, false)
	if err != nil || next != nil || room.Match.MatchID != started.MatchID {
		t.Fatalf("refusal created rematch: room=%+v next=%+v err=%v", room, next, err)
	}
	room, next, err = service.RequestRematchForConnection(ctx, claimTwo.Lease, started.MatchID, true)
	if err != nil || next == nil {
		t.Fatalf("second ready: room=%+v next=%+v err=%v", room, next, err)
	}
	if next.MatchID == started.MatchID || room.Match.MatchID != next.MatchID {
		t.Fatalf("rematch id was not replaced: old=%s new=%s room=%+v", started.MatchID, next.MatchID, room.Match)
	}
	if len(room.PreviousMatches) != 1 || room.PreviousMatches[0].MatchID != started.MatchID ||
		room.PreviousMatches[0].Reason != engine.ResultSurrender || room.PreviousMatches[0].WinnerSeat != SeatTwo {
		t.Fatalf("preserved result=%+v", room.PreviousMatches)
	}
	current, err := service.MatchSessionForConnection(claimOne.Lease)
	if err != nil {
		t.Fatal(err)
	}
	state, err := current.Snapshot(ctx)
	if err != nil || state.MatchID != next.MatchID || state.Result != nil {
		t.Fatalf("new match state=%+v err=%v", state.Result, err)
	}
}
