package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

func TestAnonymousSessionsAndRoomCodesUseSeparateSecrets(t *testing.T) {
	service, _, _ := newRoomTestService(t)
	session := mustCreateSession(t, service)
	credentials := credentialsFor(session)
	admission, err := service.CreateRoom(credentials, "create-one")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	if len(session.Secret) < 22 {
		t.Fatalf("session secret is too short: %d", len(session.Secret))
	}
	if session.Secret == admission.Room.Code || strings.Contains(session.Secret, admission.Room.Code) {
		t.Fatal("room code must not be the recovery secret")
	}
	if len(admission.Room.Code) < 10 {
		t.Fatalf("room code is too short: %q", admission.Room.Code)
	}
	if _, valid := normalizeRoomCode(admission.Room.Code, len(admission.Room.Code)); !valid {
		t.Fatalf("room code is not alphabet-32: %q", admission.Room.Code)
	}

	payload, err := json.Marshal(admission.Room)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if strings.Contains(string(payload), session.Secret) || strings.Contains(string(payload), session.SessionID) {
		t.Fatalf("room snapshot leaked session material: %s", payload)
	}

	wrongSecret := credentials
	wrongSecret.Secret = "not-the-secret"
	if _, err := service.Snapshot(wrongSecret, admission.Room.RoomID); !errors.Is(err, ErrSessionLost) {
		t.Fatalf("wrong secret error = %v, want session loss", err)
	}
	if strings.Contains(fmt.Sprint(service.sessions[session.SessionID].secretDigest), session.Secret) {
		t.Fatal("service retained the cleartext recovery secret")
	}
}

func TestSuccessfulAuthenticationExtendsSessionTTL(t *testing.T) {
	service, clock, _ := newRoomTestService(t)
	session := mustCreateSession(t, service)
	room, err := service.CreateRoom(credentialsFor(session), "create-sliding-session")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	clock.Advance(23 * time.Hour)
	if _, err := service.Snapshot(credentialsFor(session), room.Room.RoomID); err != nil {
		t.Fatalf("refresh active session: %v", err)
	}
	clock.Advance(23 * time.Hour)
	if _, err := service.Snapshot(credentialsFor(session), room.Room.RoomID); err != nil {
		t.Fatalf("sliding TTL was not extended: %v", err)
	}
	clock.Advance(DefaultSessionTTL)
	if _, err := service.Snapshot(credentialsFor(session), room.Room.RoomID); !errors.Is(err, ErrSessionLost) {
		t.Fatalf("inactive session error = %v", err)
	}
}

func TestCreateAndJoinAreIdempotentAndAtomic(t *testing.T) {
	service, _, _ := newRoomTestService(t)
	creator := mustCreateSession(t, service)
	first, err := service.CreateRoom(credentialsFor(creator), "same-create")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	second, err := service.CreateRoom(credentialsFor(creator), "same-create")
	if err != nil {
		t.Fatalf("repeat create room: %v", err)
	}
	if first.Room.RoomID != second.Room.RoomID || first.Seat != second.Seat {
		t.Fatalf("create was not idempotent: %#v != %#v", first, second)
	}
	if _, err := service.JoinRoom(credentialsFor(creator), first.Room.Code, "same-create"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("cross-operation key reuse error = %v", err)
	}

	const contenders = 24
	type result struct {
		session AnonymousSession
		seat    SeatID
		err     error
	}
	results := make(chan result, contenders)
	var wait sync.WaitGroup
	for index := 0; index < contenders; index++ {
		session := mustCreateSession(t, service)
		wait.Add(1)
		go func(index int, session AnonymousSession) {
			defer wait.Done()
			admission, err := service.JoinRoom(
				credentialsFor(session), first.Room.Code, fmt.Sprintf("join-%d", index),
			)
			results <- result{session: session, seat: admission.Seat, err: err}
		}(index, session)
	}
	wait.Wait()
	close(results)

	joined := 0
	var winner result
	for candidate := range results {
		if candidate.err == nil {
			joined++
			winner = candidate
			continue
		}
		if !errors.Is(candidate.err, ErrRoomFull) {
			t.Fatalf("losing join error = %v, want room full", candidate.err)
		}
	}
	if joined != 1 || winner.seat != SeatTwo {
		t.Fatalf("atomic admission winners = %d, winner seat = %q", joined, winner.seat)
	}
	repeated, err := service.JoinRoom(credentialsFor(winner.session), first.Room.Code, "winner-repeat")
	if err != nil || repeated.Seat != SeatTwo {
		t.Fatalf("existing participant rejoin = %#v, %v", repeated, err)
	}
	idempotent, err := service.JoinRoom(
		credentialsFor(winner.session), strings.ToLower(first.Room.Code), "winner-repeat",
	)
	if err != nil || idempotent.Room.RoomID != first.Room.RoomID {
		t.Fatalf("idempotent join = %#v, %v", idempotent, err)
	}
}

func TestRoomErrorsRemainDistinctWithoutLeakingSecrets(t *testing.T) {
	service, clock, _ := newRoomTestService(t)
	creator := mustCreateSession(t, service)
	room, err := service.CreateRoom(credentialsFor(creator), "create-errors")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	second := mustCreateSession(t, service)
	if _, err := service.JoinRoom(credentialsFor(second), room.Room.Code, "join-second"); err != nil {
		t.Fatalf("join second: %v", err)
	}
	third := mustCreateSession(t, service)
	if _, err := service.JoinRoom(credentialsFor(third), room.Room.Code, "join-full"); !errors.Is(err, ErrRoomFull) {
		t.Fatalf("full room error = %v", err)
	}
	if _, err := service.JoinRoom(credentialsFor(third), "AAAAAAAAAAAAA", "join-invalid"); !errors.Is(err, ErrInvalidRoomCode) {
		t.Fatalf("invalid code error = %v", err)
	}

	emptyCreator := mustCreateSession(t, service)
	empty, err := service.CreateRoom(credentialsFor(emptyCreator), "create-expired")
	if err != nil {
		t.Fatalf("create expiring room: %v", err)
	}
	if _, err := service.LeaveRoom(credentialsFor(emptyCreator), empty.Room.RoomID); err != nil {
		t.Fatalf("leave expiring room: %v", err)
	}
	clock.Advance(DefaultEmptyRoomTTL)
	service.CleanupExpired()
	if _, err := service.JoinRoom(credentialsFor(third), empty.Room.Code, "join-expired"); !errors.Is(err, ErrRoomExpired) {
		t.Fatalf("expired code error = %v", err)
	}

	clock.Advance(DefaultSessionTTL)
	if _, err := service.Snapshot(credentialsFor(creator), room.Room.RoomID); !errors.Is(err, ErrSessionLost) {
		t.Fatalf("expired session error = %v", err)
	}
	for _, err := range []error{ErrRoomFull, ErrInvalidRoomCode, ErrRoomExpired, ErrSessionLost} {
		if strings.Contains(err.Error(), creator.Secret) || strings.Contains(err.Error(), room.Room.Code) {
			t.Fatalf("safe error leaked secret/code: %v", err)
		}
	}
}

func TestSelectionResetsReadyAndStartPinsOneAtomicMatch(t *testing.T) {
	service, _, pins := newRoomTestService(t)
	first := mustCreateSession(t, service)
	second := mustCreateSession(t, service)
	room, err := service.CreateRoom(credentialsFor(first), "create-start")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if _, err := service.JoinRoom(credentialsFor(second), room.Room.Code, "join-start"); err != nil {
		t.Fatalf("join room: %v", err)
	}
	selection := testSelection("deck-mirror")
	if _, err := service.SetSelection(credentialsFor(first), room.Room.RoomID, selection); err != nil {
		t.Fatalf("select first deck: %v", err)
	}
	if _, err := service.SetSelection(credentialsFor(second), room.Room.RoomID, selection); err != nil {
		t.Fatalf("select mirror deck: %v", err)
	}
	ready, started, err := service.SetReady(context.Background(), credentialsFor(first), room.Room.RoomID, true)
	if err != nil || started != nil || !ready.Seats[0].Ready {
		t.Fatalf("ready first = %#v, %#v, %v", ready, started, err)
	}
	changed := testSelection("deck-mirror")
	changed.OrderIDs[0], changed.OrderIDs[1] = changed.OrderIDs[1], changed.OrderIDs[0]
	changedSnapshot, err := service.SetSelection(credentialsFor(first), room.Room.RoomID, changed)
	if err != nil {
		t.Fatalf("change selection: %v", err)
	}
	if changedSnapshot.Seats[0].Ready {
		t.Fatal("selection change did not reset ready")
	}
	if _, _, err := service.SetReady(context.Background(), credentialsFor(first), room.Room.RoomID, true); err != nil {
		t.Fatalf("ready first again: %v", err)
	}

	const racers = 12
	var wait sync.WaitGroup
	results := make(chan *StartedMatch, racers)
	errorsSeen := make(chan error, racers)
	for range racers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, match, err := service.SetReady(context.Background(), credentialsFor(second), room.Room.RoomID, true)
			if err != nil {
				errorsSeen <- err
				return
			}
			results <- match
		}()
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatalf("concurrent ready failed: %v", err)
	}
	var matchID string
	for match := range results {
		if match == nil {
			t.Fatal("concurrent ready returned no started match")
		}
		if matchID == "" {
			matchID = match.MatchID
		}
		if match.MatchID != matchID || match.InitialRevision != 2 {
			t.Fatalf("start was not stable: %#v", match)
		}
		if match.DeckManifestIDs != [2]string{"manifest-deck-mirror", "manifest-deck-mirror"} {
			t.Fatalf("mirror manifests were not pinned: %#v", match.DeckManifestIDs)
		}
	}
	if calls := pins.Calls(); calls != 1 {
		t.Fatalf("pin provider calls = %d, want 1", calls)
	}

	matchSession, err := service.MatchSession(credentialsFor(first), room.Room.RoomID)
	if err != nil || matchSession == nil {
		t.Fatalf("match session = %v, %v", matchSession, err)
	}
	if snapshot, err := service.Snapshot(credentialsFor(first), room.Room.RoomID); err != nil || snapshot.Status != RoomActive {
		t.Fatalf("active room snapshot = %#v, %v", snapshot, err)
	}
}

func TestFailedStartCommitsNoPartialReadyOrMatch(t *testing.T) {
	service, _, pins := newRoomTestService(t)
	pins.fail = errors.New("catalog snapshot unavailable")
	first := mustCreateSession(t, service)
	second := mustCreateSession(t, service)
	room, _ := service.CreateRoom(credentialsFor(first), "create-failed-start")
	_, _ = service.JoinRoom(credentialsFor(second), room.Room.Code, "join-failed-start")
	selection := testSelection("deck-one")
	_, _ = service.SetSelection(credentialsFor(first), room.Room.RoomID, selection)
	_, _ = service.SetSelection(credentialsFor(second), room.Room.RoomID, selection)
	_, _, _ = service.SetReady(context.Background(), credentialsFor(first), room.Room.RoomID, true)
	before, _ := service.Snapshot(credentialsFor(second), room.Room.RoomID)
	if _, _, err := service.SetReady(context.Background(), credentialsFor(second), room.Room.RoomID, true); !errors.Is(err, ErrMatchSetupUnavailable) {
		t.Fatalf("failed start error = %v", err)
	}
	after, _ := service.Snapshot(credentialsFor(second), room.Room.RoomID)
	if after.Status != RoomWaiting || after.Match != nil || after.Seats[1].Ready || after.Revision != before.Revision {
		t.Fatalf("failed start partially mutated room: before=%#v after=%#v", before, after)
	}
}

func TestRoomTTLsAndCleanupWakePendingRequests(t *testing.T) {
	service, clock, _ := newRoomTestService(t)
	creator := mustCreateSession(t, service)
	room, _ := service.CreateRoom(credentialsFor(creator), "create-wait")
	if _, err := service.LeaveRoom(credentialsFor(creator), room.Room.RoomID); err != nil {
		t.Fatalf("leave: %v", err)
	}

	waitResult := make(chan error, 1)
	// A participant must still control a seat to wait, so create a one-seat lobby
	// for the pending cleanup scenario.
	waiter := mustCreateSession(t, service)
	waitingRoom, _ := service.CreateRoom(credentialsFor(waiter), "create-pending")
	go func() {
		_, err := service.WaitForRoomChange(
			context.Background(), credentialsFor(waiter), waitingRoom.Room.RoomID, waitingRoom.Room.Revision,
		)
		waitResult <- err
	}()

	clock.Advance(DefaultEmptyRoomTTL)
	report := service.CleanupExpired()
	if report.Rooms != 1 {
		t.Fatalf("empty-room cleanup = %#v", report)
	}
	clock.Advance(DefaultLobbyTTL - DefaultEmptyRoomTTL)
	report = service.CleanupExpired()
	if report.Rooms != 1 {
		t.Fatalf("lobby cleanup = %#v", report)
	}
	select {
	case err := <-waitResult:
		if !errors.Is(err, ErrRoomExpired) {
			t.Fatalf("pending wait error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("room cleanup did not complete the pending request")
	}
}

func TestSessionExpiryWakesPendingRequestWithSessionLoss(t *testing.T) {
	clock := &fakeRoomClock{now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	service, err := NewRoomService(RoomServiceConfig{
		Clock: clock, Entropy: &incrementingReader{}, Pins: &fakePins{},
		SessionTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("new room service: %v", err)
	}
	player := mustCreateSession(t, service)
	room, err := service.CreateRoom(credentialsFor(player), "create-session-wait")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := service.WaitForRoomChange(
			context.Background(), credentialsFor(player), room.Room.RoomID, room.Room.Revision,
		)
		result <- err
	}()
	clock.Advance(time.Minute)
	service.CleanupExpired()
	select {
	case err := <-result:
		if !errors.Is(err, ErrSessionLost) {
			t.Fatalf("pending session error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session cleanup did not wake the pending request")
	}
}

func TestActiveMatchIsNotDeletedByLobbyTTL(t *testing.T) {
	service, clock, _ := newRoomTestService(t)
	first := mustCreateSession(t, service)
	second := mustCreateSession(t, service)
	room, _ := service.CreateRoom(credentialsFor(first), "create-active")
	_, _ = service.JoinRoom(credentialsFor(second), room.Room.Code, "join-active")
	selection := testSelection("deck-one")
	_, _ = service.SetSelection(credentialsFor(first), room.Room.RoomID, selection)
	_, _ = service.SetSelection(credentialsFor(second), room.Room.RoomID, selection)
	_, _, _ = service.SetReady(context.Background(), credentialsFor(first), room.Room.RoomID, true)
	_, _, _ = service.SetReady(context.Background(), credentialsFor(second), room.Room.RoomID, true)
	clock.Advance(48 * time.Hour)
	if report := service.CleanupExpired(); report.Rooms != 0 {
		t.Fatalf("active match was cleaned: %#v", report)
	}
}

func TestOneTimeTicketsAndConnectionReplacement(t *testing.T) {
	service, clock, _ := newRoomTestService(t)
	player := mustCreateSession(t, service)
	room, _ := service.CreateRoom(credentialsFor(player), "create-ticket")
	ticket, err := service.IssueConnectionTicket(credentialsFor(player), room.Room.RoomID)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	first, err := service.RedeemConnectionTicket(ticket.Token, "connection-one")
	if err != nil {
		t.Fatalf("redeem ticket: %v", err)
	}
	if err := service.AuthorizeConnection(first.Lease); err != nil {
		t.Fatalf("authorize first connection: %v", err)
	}
	if _, err := service.RedeemConnectionTicket(ticket.Token, "connection-replay"); !errors.Is(err, ErrConnectionTicket) {
		t.Fatalf("ticket replay error = %v", err)
	}
	second, err := service.ReplaceConnection(credentialsFor(player), room.Room.RoomID, "connection-two")
	if err != nil {
		t.Fatalf("replace connection: %v", err)
	}
	if second.ReplacedConnectionID != "connection-one" {
		t.Fatalf("replaced connection = %q", second.ReplacedConnectionID)
	}
	if err := service.AuthorizeConnection(first.Lease); !errors.Is(err, ErrConnectionReplaced) {
		t.Fatalf("old connection error = %v", err)
	}
	if err := service.AuthorizeConnection(second.Lease); err != nil {
		t.Fatalf("new connection error = %v", err)
	}

	expiring, err := service.IssueConnectionTicket(credentialsFor(player), room.Room.RoomID)
	if err != nil {
		t.Fatalf("issue expiring ticket: %v", err)
	}
	clock.Advance(DefaultTicketTTL)
	if _, err := service.RedeemConnectionTicket(expiring.Token, "connection-expired"); !errors.Is(err, ErrConnectionTicket) {
		t.Fatalf("expired ticket error = %v", err)
	}
	current, err := service.Snapshot(credentialsFor(player), room.Room.RoomID)
	if err != nil {
		t.Fatalf("snapshot before revoke: %v", err)
	}
	waitResult := make(chan error, 1)
	go func() {
		_, err := service.WaitForRoomChange(
			context.Background(), credentialsFor(player), room.Room.RoomID, current.Revision,
		)
		waitResult <- err
	}()
	if err := service.RevokeSession(credentialsFor(player)); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	select {
	case err := <-waitResult:
		if !errors.Is(err, ErrSessionLost) {
			t.Fatalf("revoked pending request error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session revocation did not terminate the pending request")
	}
	if err := service.AuthorizeConnection(second.Lease); !errors.Is(err, ErrSessionLost) {
		t.Fatalf("revoked session connection error = %v", err)
	}
}

type fakeRoomClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *fakeRoomClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *fakeRoomClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(duration)
}

type incrementingReader struct {
	mu   sync.Mutex
	next uint64
}

func (reader *incrementingReader) Read(target []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	written := 0
	for written < len(target) {
		reader.next++
		block := sha256.Sum256([]byte(fmt.Sprintf("test-entropy-%d", reader.next)))
		written += copy(target[written:], block[:])
	}
	return len(target), nil
}

type fakePins struct {
	mu    sync.Mutex
	calls int
	fail  error
}

func (pins *fakePins) PinMatch(_ context.Context, selections [2]SeatSelection) (MatchPins, error) {
	pins.mu.Lock()
	defer pins.mu.Unlock()
	pins.calls++
	if pins.fail != nil {
		return MatchPins{}, pins.fail
	}
	result := MatchPins{Versions: engine.Versions{
		Content: "content-v1", DeckManifests: "decks-v1", EffectSpec: "effects-v1",
		Rules: "rules-v1", Protocol: "protocol-v1", Engine: "engine-v1",
	}}
	for index, selection := range selections {
		result.Decks[index] = PinnedDeck{
			DeckID: selection.DeckID, ManifestID: "manifest-" + selection.DeckID,
			HeroID: "hero-" + selection.DeckID, CardIDs: testCards(selection.DeckID),
			AvailableOrderIDs: []string{"order-1", "order-2", "order-3", "order-4", "order-5"},
			BaseInfluence:     5,
		}
	}
	return result, nil
}

func (pins *fakePins) Calls() int {
	pins.mu.Lock()
	defer pins.mu.Unlock()
	return pins.calls
}

func newRoomTestService(t *testing.T) (*RoomService, *fakeRoomClock, *fakePins) {
	t.Helper()
	clock := &fakeRoomClock{now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	pins := &fakePins{}
	service, err := NewRoomService(RoomServiceConfig{
		Clock: clock, Entropy: &incrementingReader{}, Pins: pins,
	})
	if err != nil {
		t.Fatalf("new room service: %v", err)
	}
	return service, clock, pins
}

func mustCreateSession(t *testing.T, service *RoomService) AnonymousSession {
	t.Helper()
	session, err := service.CreateSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}

func credentialsFor(session AnonymousSession) SessionCredentials {
	return SessionCredentials{SessionID: session.SessionID, Secret: session.Secret}
}

func testSelection(deckID string) SeatSelection {
	return SeatSelection{DeckID: deckID, OrderIDs: []string{
		"order-1", "order-2", "order-3", "order-4", "order-5",
	}}
}

func testCards(prefix string) []string {
	cards := make([]string, 30)
	for index := range cards {
		cards[index] = fmt.Sprintf("%s-card-%02d", prefix, index+1)
	}
	return cards
}
