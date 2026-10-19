package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/application"
	"github.com/dKorialstrasz/response-online/backend/internal/engine"
	"github.com/gorilla/websocket"
)

const testOrigin = "https://client.example.test"

func TestSessionCookieCSRFAndOriginBoundary(t *testing.T) {
	service, err := application.NewRoomService(application.RoomServiceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandlerWithConfig(Config{
		Service: service, AllowedOrigins: []string{testOrigin}, SecureCookies: true,
		CSRFKey: bytes.Repeat([]byte{1}, 32), ReferenceKey: bytes.Repeat([]byte{2}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{}`))
	request.Header.Set("Origin", testOrigin)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, body = %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Domain != "" {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
	var session sessionResponse
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.SessionID == "" || session.CSRFToken == "" || strings.Contains(response.Body.String(), cookies[0].Value) {
		t.Fatal("session response did not keep the recovery secret cookie-only")
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	request.Header.Set("Origin", testOrigin)
	request.Header.Set("Idempotency-Key", "create-1")
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"unexpected":true}`))
	request.Header.Set("Origin", testOrigin)
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	request.Header.Set("Idempotency-Key", "create-1")
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("create room unknown-field status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	request.Header.Set("Origin", testOrigin)
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	request.Header.Set("Idempotency-Key", "create-1")
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("authenticated create status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "session_") || strings.Contains(response.Body.String(), cookies[0].Value) {
		t.Fatalf("room response leaked credentials: %s", response.Body.String())
	}
	refreshedCookies := response.Result().Cookies()
	if len(refreshedCookies) != 1 || refreshedCookies[0].MaxAge <= 0 || refreshedCookies[0].Value != cookies[0].Value {
		t.Fatalf("active session cookie was not renewed: %#v", refreshedCookies)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{}`))
	request.Header.Set("Origin", "https://attacker.example.test")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unknown Origin status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/ws?ticket=secret", nil)
	request.Header.Set("Origin", testOrigin)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("ticket-in-URL response = %d %s", response.Code, response.Body.String())
	}
}

func TestConfiguredOriginsMustBeExactHTTPOrigins(t *testing.T) {
	service, _ := application.NewRoomService(application.RoomServiceConfig{})
	for _, origin := range []string{"https://example.test/path", "https://user@example.test", "javascript:alert(1)", "https://example.test?query=1"} {
		if _, err := NewHandlerWithConfig(Config{Service: service, AllowedOrigins: []string{origin}}); err == nil {
			t.Fatalf("invalid origin %q was accepted", origin)
		}
	}
	if _, err := NewHandlerWithConfig(Config{
		Service: service, AllowedOrigins: []string{testOrigin},
		Limits: SecurityLimits{WSCommandBurst: -1},
	}); err == nil {
		t.Fatal("negative security limit was accepted")
	}
}

func TestHTTPBodyAndUnknownFieldsAreRejected(t *testing.T) {
	service, _ := application.NewRoomService(application.RoomServiceConfig{})
	handler, _ := NewHandlerWithConfig(Config{
		Service: service, AllowedOrigins: []string{testOrigin},
		CSRFKey: bytes.Repeat([]byte{1}, 32), ReferenceKey: bytes.Repeat([]byte{2}, 32),
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{"unexpected":true}`))
	request.Header.Set("Origin", testOrigin)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown-field status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{"padding":"`+strings.Repeat("x", maxHTTPBody)+`"}`))
	request.Header.Set("Origin", testOrigin)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized-body status = %d", response.Code)
	}
}

func TestConfiguredCreateRoomRateLimit(t *testing.T) {
	service, _ := application.NewRoomService(application.RoomServiceConfig{})
	handler, err := NewHandlerWithConfig(Config{
		Service: service, AllowedOrigins: []string{testOrigin},
		CSRFKey: bytes.Repeat([]byte{1}, 32), ReferenceKey: bytes.Repeat([]byte{2}, 32),
		Limits: SecurityLimits{CreateRoomsPerSession: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{}`))
	request.Header.Set("Origin", testOrigin)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var session sessionResponse
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	cookie := response.Result().Cookies()[0]
	for attempt, expected := range []int{http.StatusCreated, http.StatusTooManyRequests} {
		request = httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
		request.Header.Set("Origin", testOrigin)
		request.Header.Set("X-CSRF-Token", session.CSRFToken)
		request.Header.Set("Idempotency-Key", fmt.Sprintf("create-%d", attempt))
		request.AddCookie(cookie)
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != expected {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, expected)
		}
	}
}

func TestWebSocketAuthenticatesWithOneTimeFirstMessageTicket(t *testing.T) {
	service, err := application.NewRoomService(application.RoomServiceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	session, _ := service.CreateSession()
	credentials := application.SessionCredentials{SessionID: session.SessionID, Secret: session.Secret}
	room, err := service.CreateRoom(credentials, "room-create")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := service.IssueConnectionTicket(credentials, room.Room.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandlerWithConfig(Config{
		Service: service, AllowedOrigins: []string{testOrigin},
		CSRFKey: bytes.Repeat([]byte{1}, 32), ReferenceKey: bytes.Repeat([]byte{2}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	listener := newPipeListener()
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	dialer := websocket.Dialer{NetDialContext: listener.dialContext}
	wsURL := "ws://pipe/api/v1/ws"
	header := http.Header{"Origin": []string{testOrigin}}
	connection, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.WriteJSON(wsAuth{
		ProtocolVersion: ProtocolVersion, Type: "authenticate", CorrelationID: "auth-1",
		Ticket: ticket.Token, ConnectionID: "connection-1",
	}); err != nil {
		t.Fatal(err)
	}
	var authenticated wsEvent
	if err := connection.ReadJSON(&authenticated); err != nil {
		t.Fatal(err)
	}
	if authenticated.Type != "authenticated" || authenticated.Revision != room.Room.Revision {
		t.Fatalf("unexpected authenticated event: %#v", authenticated)
	}
	var snapshot wsEvent
	if err := connection.ReadJSON(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Type != "roomSnapshot" {
		t.Fatalf("unexpected room event: %#v", snapshot)
	}

	second, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.WriteJSON(wsAuth{
		ProtocolVersion: ProtocolVersion, Type: "authenticate", CorrelationID: "auth-2",
		Ticket: ticket.Token, ConnectionID: "connection-2",
	}); err != nil {
		t.Fatal(err)
	}
	var rejected wsEvent
	if err := second.ReadJSON(&rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Error == nil || rejected.Error.Code != codeUnauthenticated {
		t.Fatalf("reused ticket result: %#v", rejected)
	}
}

func TestAcceptedCommandBroadcastsPersonalizedStateToBothSeats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service, credentials, roomID, session := readyRoom(t)
	go func() { _ = session.Run(ctx) }()
	handler, err := NewHandlerWithConfig(Config{
		Context: ctx, Service: service, AllowedOrigins: []string{testOrigin},
		CSRFKey: bytes.Repeat([]byte{1}, 32), ReferenceKey: bytes.Repeat([]byte{2}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	listener := newPipeListener()
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	dialer := websocket.Dialer{NetDialContext: listener.dialContext}
	header := http.Header{"Origin": []string{testOrigin}}
	connections := make([]*websocket.Conn, 2)
	initial := make([]wsEvent, 2)
	for index := range connections {
		ticket, err := service.IssueConnectionTicket(credentials[index], roomID)
		if err != nil {
			t.Fatal(err)
		}
		connection, _, err := dialer.Dial("ws://pipe/api/v1/ws", header)
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		connections[index] = connection
		if err := connection.WriteJSON(wsAuth{
			ProtocolVersion: ProtocolVersion, Type: "authenticate", CorrelationID: fmt.Sprintf("auth-%d", index),
			Ticket: ticket.Token, ConnectionID: fmt.Sprintf("connection-%d", index),
		}); err != nil {
			t.Fatal(err)
		}
		for eventIndex := 0; eventIndex < 3; eventIndex++ {
			var event wsEvent
			if err := connection.ReadJSON(&event); err != nil {
				t.Fatal(err)
			}
			if event.Type == "matchSnapshot" {
				initial[index] = event
			}
		}
		if initial[index].MatchID == "" {
			t.Fatalf("seat %d did not receive initial match snapshot", index)
		}
	}
	command := wsCommand{
		ProtocolVersion: ProtocolVersion, Type: "command", CommandID: "broadcast-mulligan",
		MatchID: initial[0].MatchID, ExpectedRevision: initial[0].Revision, Name: "submitMulligan",
		Payload: json.RawMessage(`{"objectRefs":[]}`),
	}
	if err := connections[0].WriteJSON(command); err != nil {
		t.Fatal(err)
	}
	_ = connections[0].SetReadDeadline(time.Now().Add(2 * time.Second))
	_ = connections[1].SetReadDeadline(time.Now().Add(2 * time.Second))
	var result wsEvent
	if err := connections[0].ReadJSON(&result); err != nil {
		t.Fatal(err)
	}
	if result.Type != "commandResult" || result.Accepted == nil || !*result.Accepted {
		t.Fatalf("unexpected command result: %#v", result)
	}
	for index, connection := range connections {
		var update wsEvent
		if err := connection.ReadJSON(&update); err != nil {
			t.Fatalf("seat %d did not receive broadcast: %v", index, err)
		}
		if update.Type != "stateUpdate" || update.Revision <= initial[index].Revision {
			t.Fatalf("seat %d broadcast = %#v", index, update)
		}
		encoded, _ := json.Marshal(update.Payload)
		if strings.Contains(string(encoded), "instanceId") || strings.Contains(string(encoded), "seed") {
			t.Fatalf("seat %d broadcast leaked hidden state: %s", index, encoded)
		}
	}
}

func TestPlayerViewRedactsHiddenStateAndUsesOpaqueReferences(t *testing.T) {
	match := newProjectionMatch(t)
	state := match.State()
	codec := newReferenceCodec(bytes.Repeat([]byte{9}, 32))
	view := projectPlayerView(state, engine.SeatA, codec)
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"instanceId", "seed", "cursor", "diagnosticTrace", "commands", "match-test:s1:c"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("PlayerView leaked %q: %s", forbidden, text)
		}
	}
	if len(view.Players[0].Hand) != engine.StartingHandSize || len(view.Players[1].Hand) != 0 || view.Players[1].HandCount != engine.StartingHandSize {
		t.Fatalf("unexpected hand projection: %#v", view.Players)
	}
	ref := view.Players[0].Hand[0].ObjectRef
	if !strings.HasPrefix(ref.ZoneObjectID, "obj_") || strings.Contains(ref.ZoneObjectID, "match-test") {
		t.Fatalf("non-opaque object ref: %#v", ref)
	}
	decoded, err := codec.decode(state, ref)
	if err != nil || decoded.ZoneObjectID == "" {
		t.Fatalf("decode projected ref: %#v, %v", decoded, err)
	}
	if _, err := codec.decode(state, objectRefPayload{ZoneObjectID: state.Players[0].Objects[0].ZoneObjectID, Generation: 1}); err == nil {
		t.Fatal("internal object reference was accepted at the transport boundary")
	}
}

func TestCommandReplayAndInvalidReferenceDoNotMutateState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service, credentials, roomID, lease, session := activeRoom(t)
	go func() { _ = session.Run(ctx) }()
	limits, _ := normalizeSecurityLimits(SecurityLimits{})
	gateway := &Gateway{
		ctx: ctx, service: service, references: newReferenceCodec(bytes.Repeat([]byte{7}, 32)),
		commandCache: make(map[string]cachedCommandResult), limits: limits,
	}
	state, err := session.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	command := wsCommand{
		ProtocolVersion: ProtocolVersion, Type: "command", CommandID: "mulligan-1",
		MatchID: state.MatchID, ExpectedRevision: state.Revision, Name: "submitMulligan",
		Payload: json.RawMessage(`{"objectRefs":[]}`),
	}
	first := gateway.executeWSCommand(ctx, lease, command)
	if first.Accepted == nil || !*first.Accepted {
		t.Fatalf("first command rejected: %#v", first)
	}
	replayed := gateway.executeWSCommand(ctx, lease, command)
	if replayed.Accepted == nil || !*replayed.Accepted || replayed.Revision != first.Revision {
		t.Fatalf("idempotent replay changed result: first=%#v replay=%#v", first, replayed)
	}
	changed := command
	changed.ExpectedRevision++
	conflict := gateway.executeWSCommand(ctx, lease, changed)
	if conflict.Error == nil || conflict.Error.Code != codeCommandIDReused {
		t.Fatalf("command id reuse result: %#v", conflict)
	}
	beforeStale, _ := session.Snapshot(ctx)
	stale := wsCommand{
		ProtocolVersion: ProtocolVersion, Type: "command", CommandID: "stale-command",
		MatchID: beforeStale.MatchID, ExpectedRevision: command.ExpectedRevision, Name: "surrender",
		Payload: json.RawMessage(`{}`),
	}
	staleResult := gateway.executeWSCommand(ctx, lease, stale)
	afterStale, _ := session.Snapshot(ctx)
	if staleResult.Error == nil || staleResult.Error.Code != codeStaleRevision ||
		!staleResult.Error.ResyncRequired || staleResult.Error.View == nil || afterStale.Revision != beforeStale.Revision {
		t.Fatalf("stale command result=%#v before=%d after=%d", staleResult, beforeStale.Revision, afterStale.Revision)
	}

	before, _ := session.Snapshot(ctx)
	invalid := wsCommand{
		ProtocolVersion: ProtocolVersion, Type: "command", CommandID: "mulligan-invalid",
		MatchID: before.MatchID, ExpectedRevision: before.Revision, Name: "submitMulligan",
		Payload: json.RawMessage(`{"objectRefs":[{"zoneObjectId":"` + before.Players[0].Objects[0].ZoneObjectID + `","generation":1}]}`),
	}
	result := gateway.executeWSCommand(ctx, lease, invalid)
	after, _ := session.Snapshot(ctx)
	if result.Error == nil || result.Error.Code != codeInvalidTarget || after.Revision != before.Revision {
		t.Fatalf("invalid reference mutated state: result=%#v before=%d after=%d", result, before.Revision, after.Revision)
	}

	room, err := service.Snapshot(credentials, roomID)
	if err != nil {
		t.Fatal(err)
	}
	encodedRoom, _ := json.Marshal(room)
	if strings.Contains(string(encodedRoom), "manifest-deck-b") || !strings.Contains(string(encodedRoom), "manifest-deck-a") {
		t.Fatalf("room snapshot leaked opponent manifest: %s", encodedRoom)
	}

	beforeReplacement, _ := session.Snapshot(ctx)
	if _, err := service.ReplaceConnection(credentials, roomID, "replacement-connection"); err != nil {
		t.Fatal(err)
	}
	err = service.SubmitForConnection(ctx, lease, engine.Command{
		ID: "replaced-command", Kind: engine.CommandPass, Seat: engine.SeatA,
		ExpectedRevision: beforeReplacement.Revision,
	})
	afterReplacement, _ := session.Snapshot(ctx)
	if !errors.Is(err, application.ErrConnectionReplaced) || afterReplacement.Revision != beforeReplacement.Revision {
		t.Fatalf("replaced connection mutated state: err=%v before=%d after=%d", err, beforeReplacement.Revision, afterReplacement.Revision)
	}
}

func TestResumeReturnsFreshPersonalViewAndReplayDoesNotRepeatEffect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service, credentials, roomID, lease, session := activeRoom(t)
	go func() { _ = session.Run(ctx) }()
	if err := session.Connected(ctx, engine.SeatA); err != nil {
		t.Fatal(err)
	}
	limits, _ := normalizeSecurityLimits(SecurityLimits{})
	gateway := &Gateway{
		ctx: ctx, service: service, references: newReferenceCodec(bytes.Repeat([]byte{7}, 32)),
		commandCache: make(map[string]cachedCommandResult), limits: limits,
	}
	before, err := session.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	command := wsCommand{
		ProtocolVersion: ProtocolVersion, Type: "command", CommandID: "resume-replay",
		MatchID: before.MatchID, ExpectedRevision: before.Revision, Name: "submitMulligan",
		Payload: json.RawMessage(`{"objectRefs":[]}`),
	}
	accepted := gateway.executeWSCommand(ctx, lease, command)
	if accepted.Accepted == nil || !*accepted.Accepted {
		t.Fatalf("initial command=%+v", accepted)
	}
	after, _ := session.Snapshot(ctx)
	claim, err := service.ReplaceConnection(credentials, roomID, "resumed-connection")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Connected(ctx, engine.SeatA); err != nil {
		t.Fatal(err)
	}
	event, err := gateway.matchSnapshotEvent(ctx, claim.Lease, "resume", "matchSnapshot")
	if err != nil {
		t.Fatal(err)
	}
	view, ok := event.Payload.(PlayerView)
	if !ok || view.OwnSeat != "seat-1" || view.Revision != after.Revision || event.Revision != after.Revision {
		t.Fatalf("resume snapshot=%+v", event)
	}
	encoded, _ := json.Marshal(view)
	for _, forbidden := range []string{"instanceId", "seed", "commands", "diagnosticTrace", "match-test:s1:c"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("resumed PlayerView leaked %q: %s", forbidden, encoded)
		}
	}
	replayed := gateway.executeWSCommand(ctx, claim.Lease, command)
	final, _ := session.Snapshot(ctx)
	if replayed.Accepted == nil || !*replayed.Accepted || replayed.Revision != accepted.Revision || final.Revision != after.Revision {
		t.Fatalf("replay after resume repeated effect: accepted=%+v replayed=%+v revisions=%d/%d", accepted, replayed, after.Revision, final.Revision)
	}
}

type fixedPins struct{}

func (fixedPins) PinMatch(_ context.Context, selections [2]application.SeatSelection) (application.MatchPins, error) {
	result := application.MatchPins{Versions: engine.Versions{
		Content: "content-v1", DeckManifests: "decks-v1", EffectSpec: "effects-v1",
		Rules: "rules-v1", Protocol: ProtocolVersion, Engine: "engine-v1",
	}}
	for index := range result.Decks {
		cards := make([]string, 30)
		for cardIndex := range cards {
			cards[cardIndex] = string(rune('a'+index)) + "-card"
		}
		result.Decks[index] = application.PinnedDeck{
			DeckID: selections[index].DeckID, ManifestID: "manifest-" + selections[index].DeckID,
			HeroID: "hero", CardIDs: cards, AvailableOrderIDs: append([]string(nil), selections[index].OrderIDs...), BaseInfluence: 10,
		}
	}
	return result, nil
}

func activeRoom(t *testing.T) (*application.RoomService, application.SessionCredentials, string, application.ConnectionLease, *application.MatchSession) {
	t.Helper()
	service, credentials, roomID, session := readyRoom(t)
	ticket, err := service.IssueConnectionTicket(credentials[0], roomID)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := service.RedeemConnectionTicket(ticket.Token, "connection-test")
	if err != nil {
		t.Fatal(err)
	}
	return service, credentials[0], roomID, claim.Lease, session
}

func readyRoom(t *testing.T) (*application.RoomService, [2]application.SessionCredentials, string, *application.MatchSession) {
	t.Helper()
	service, err := application.NewRoomService(application.RoomServiceConfig{Pins: fixedPins{}})
	if err != nil {
		t.Fatal(err)
	}
	one, _ := service.CreateSession()
	two, _ := service.CreateSession()
	credentials := [2]application.SessionCredentials{
		{SessionID: one.SessionID, Secret: one.Secret},
		{SessionID: two.SessionID, Secret: two.Secret},
	}
	oneCredentials := credentials[0]
	twoCredentials := credentials[1]
	created, err := service.CreateRoom(oneCredentials, "create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.JoinRoom(twoCredentials, created.Room.Code, "join"); err != nil {
		t.Fatal(err)
	}
	orders := []string{"o1", "o2", "o3", "o4", "o5"}
	if _, err := service.SetSelection(oneCredentials, created.Room.RoomID, application.SeatSelection{DeckID: "deck-a", OrderIDs: orders}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetSelection(twoCredentials, created.Room.RoomID, application.SeatSelection{DeckID: "deck-b", OrderIDs: orders}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.SetReady(context.Background(), oneCredentials, created.Room.RoomID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.SetReady(context.Background(), twoCredentials, created.Room.RoomID, true); err != nil {
		t.Fatal(err)
	}
	session, err := service.MatchSession(oneCredentials, created.Room.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	return service, credentials, created.Room.RoomID, session
}

func newProjectionMatch(t *testing.T) *engine.Match {
	t.Helper()
	config := engine.Config{
		MatchID: "match-test", StartedAt: time.Unix(1, 0), Seed: 42,
		Versions: engine.Versions{Content: "c", DeckManifests: "d", EffectSpec: "e", Rules: "r", Protocol: ProtocolVersion, Engine: "g"},
	}
	orders := []string{"o1", "o2", "o3", "o4", "o5"}
	for index := range config.Players {
		cards := make([]string, 30)
		for cardIndex := range cards {
			cards[cardIndex] = string(rune('a'+index)) + "-card"
		}
		config.Players[index] = engine.PlayerConfig{
			Seat: engine.Seat(index), HeroID: "hero", DeckCardIDs: cards,
			AvailableIDs: orders, BaseInfluence: 10,
		}
	}
	match, err := engine.NewMatch(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := match.Handle(engine.Command{ID: "orders-a", Kind: engine.CommandArrangeOrders, Seat: engine.SeatA, ExpectedRevision: 0, OrderIDs: orders}); err != nil {
		t.Fatal(err)
	}
	if err := match.Handle(engine.Command{ID: "orders-b", Kind: engine.CommandArrangeOrders, Seat: engine.SeatB, ExpectedRevision: 1, OrderIDs: orders}); err != nil {
		t.Fatal(err)
	}
	return match
}

type pipeListener struct {
	connections chan net.Conn
	closed      chan struct{}
	once        sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{connections: make(chan net.Conn), closed: make(chan struct{})}
}

func (listener *pipeListener) Accept() (net.Conn, error) {
	select {
	case connection := <-listener.connections:
		return connection, nil
	case <-listener.closed:
		return nil, net.ErrClosed
	}
}

func (listener *pipeListener) Close() error {
	listener.once.Do(func() { close(listener.closed) })
	return nil
}

func (listener *pipeListener) Addr() net.Addr { return pipeAddress("pipe") }

func (listener *pipeListener) dialContext(_ context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case listener.connections <- server:
		return client, nil
	case <-listener.closed:
		_ = client.Close()
		_ = server.Close()
		return nil, net.ErrClosed
	}
}

type pipeAddress string

func (address pipeAddress) Network() string { return string(address) }
func (address pipeAddress) String() string  { return string(address) }
