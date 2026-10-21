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

func TestRematchDecisionBroadcastsRoomStateToBothSeats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service, credentials, roomID, session := readyRoom(t)
	go func() { _ = session.Run(ctx) }()
	for _, command := range []engine.Command{
		{ID: "mulligan-a", Kind: engine.CommandMulligan, Seat: engine.SeatA, ExpectedRevision: 2},
		{ID: "mulligan-b", Kind: engine.CommandMulligan, Seat: engine.SeatB, ExpectedRevision: 3},
		{ID: "surrender-a", Kind: engine.CommandSurrender, Seat: engine.SeatA, ExpectedRevision: 4},
	} {
		if err := session.Submit(ctx, command); err != nil {
			t.Fatalf("finish match with %s: %v", command.ID, err)
		}
	}

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
		ticket, issueErr := service.IssueConnectionTicket(credentials[index], roomID)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		connection, _, dialErr := dialer.Dial("ws://pipe/api/v1/ws", header)
		if dialErr != nil {
			t.Fatal(dialErr)
		}
		defer connection.Close()
		connections[index] = connection
		if err := connection.WriteJSON(wsAuth{
			ProtocolVersion: ProtocolVersion, Type: "authenticate", CorrelationID: fmt.Sprintf("rematch-auth-%d", index),
			Ticket: ticket.Token, ConnectionID: fmt.Sprintf("rematch-connection-%d", index),
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
	}

	command := wsCommand{
		ProtocolVersion: ProtocolVersion, Type: "command", CommandID: "rematch-ready-a",
		MatchID: initial[0].MatchID, ExpectedRevision: initial[0].Revision, Name: "requestRematch",
		Payload: json.RawMessage(`{"ready":true}`),
	}
	if err := connections[0].WriteJSON(command); err != nil {
		t.Fatal(err)
	}
	var result wsEvent
	if err := connections[0].ReadJSON(&result); err != nil {
		t.Fatal(err)
	}
	if result.Type != "commandResult" || result.Accepted == nil || !*result.Accepted {
		t.Fatalf("rematch command result = %+v", result)
	}
	for index, connection := range connections {
		_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
		var event wsEvent
		if err := connection.ReadJSON(&event); err != nil {
			t.Fatalf("seat %d room broadcast: %v", index, err)
		}
		if event.Type != "roomSnapshot" {
			t.Fatalf("seat %d first rematch broadcast = %+v", index, event)
		}
		encoded, _ := json.Marshal(event.Payload)
		var room application.RoomSnapshot
		if err := json.Unmarshal(encoded, &room); err != nil {
			t.Fatal(err)
		}
		if !room.Seats[0].RematchReady || room.Seats[1].RematchReady {
			t.Fatalf("seat %d rematch room = %+v", index, room.Seats)
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

func TestQueueAndJournalExposeOnlyPublicAllowlistedFacts(t *testing.T) {
	match := newProjectionMatch(t)
	state := match.State()
	state.Players[0].Objects[0].Zone = engine.ZoneBoard
	state.Players[0].Objects[0].Lane = 0
	state.Players[0].Objects[0].Slot = engine.SlotUnit
	publicRef := state.Players[0].Objects[0].Ref()
	hiddenRef := state.Players[1].Objects[0].Ref()
	state.EffectQueue = []engine.EffectQueueEntry{
		{EntryID: "public-entry", Effect: engine.ResolvedEffect{Kind: engine.EffectModifyPower, SourceRef: &publicRef}},
		{EntryID: "hidden-entry", Effect: engine.ResolvedEffect{Kind: engine.EffectRequestChoice, SourceRef: &hiddenRef}},
	}
	seat := engine.SeatA
	state.Events = append(state.Events,
		engine.Event{Sequence: uint64(len(state.Events) + 1), Kind: "card_played", Seat: &seat, Detail: "secret-card-instance"},
		engine.Event{Sequence: uint64(len(state.Events) + 2), Kind: "private_trap_armed", Detail: "secret-trap"},
		engine.Event{Sequence: uint64(len(state.Events) + 3), Kind: "choice_requested", Seat: &seat, Detail: "secret-choice-kind"},
		engine.Event{Sequence: uint64(len(state.Events) + 4), Kind: "choice_submitted", Seat: &seat, Detail: "secret-choice-answer"},
	)

	view := projectPlayerView(state, engine.SeatA, newReferenceCodec(bytes.Repeat([]byte{17}, 32)))
	if len(view.Queue) != 1 || view.Queue[0].Kind != "Изменение силы" {
		t.Fatalf("public queue projection = %+v", view.Queue)
	}
	if len(view.Journal) == 0 || view.Journal[len(view.Journal)-1].Message != "Карта разыграна" {
		t.Fatalf("public journal projection = %+v", view.Journal)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"public-entry", "hidden-entry", "secret-card-instance", "secret-trap", "private_trap_armed",
		"secret-choice-kind", "secret-choice-answer", "Сервер ожидает решение", "Решение принято", hiddenRef.ZoneObjectID,
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("queue or journal leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestVoidAndCombatPlaybackProjectionArePublicAndTyped(t *testing.T) {
	state := newProjectionMatch(t).State()
	state.Players[0].Objects[0].Zone = engine.ZoneVoid
	state.Players[0].Objects[0].ZoneIndex = 0
	state.Players[0].Objects[1].Zone = engine.ZoneBoard
	state.Players[0].Objects[1].Lane = 0
	state.Players[0].Objects[1].Slot = engine.SlotUnit
	seat := engine.SeatB
	state.Events = append(state.Events,
		engine.Event{Sequence: uint64(len(state.Events) + 1), Kind: "combat_lane_started", Detail: "lane=3"},
		engine.Event{Sequence: uint64(len(state.Events) + 2), Kind: "hero_hit", Seat: &seat, Detail: "lane=3 secret-internal-detail"},
	)
	view := projectPlayerView(state, engine.SeatA, newReferenceCodec(bytes.Repeat([]byte{21}, 32)))
	if len(view.Players[0].Void) != 1 || view.Players[0].Void[0].DefinitionID == "" {
		t.Fatalf("void projection = %+v", view.Players[0].Void)
	}
	if len(view.CombatEvents) != 2 || view.CombatEvents[0].Lane == nil || *view.CombatEvents[0].Lane != 3 || view.CombatEvents[1].Seat != string(applicationSeatTwo) {
		t.Fatalf("combat playback projection = %+v", view.CombatEvents)
	}
	encoded, err := json.Marshal(view.CombatEvents)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-internal-detail") {
		t.Fatalf("combat playback leaked engine detail: %s", encoded)
	}
	viewJSON, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(viewJSON), `"lane":0`) || strings.Contains(string(viewJSON), `"lane":-1`) {
		t.Fatalf("board lane projection does not match schema: %s", viewJSON)
	}
}

func TestLegalActionProjectionUsesSchemaArraysInsteadOfNull(t *testing.T) {
	state := newProjectionMatch(t).State()
	view := projectPlayerView(
		state,
		engine.SeatA,
		newReferenceCodec(bytes.Repeat([]byte{19}, 32)),
		[]engine.LegalAction{{Name: "passAction"}},
	)
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, field := range []string{"modeIds", "targetRefs", "paymentRefs", "influenceCosts", "placements", "shieldTargets"} {
		if strings.Contains(text, `"`+field+`":null`) || !strings.Contains(text, `"`+field+`":[]`) {
			t.Fatalf("legal action field %s is not a schema array: %s", field, text)
		}
	}
}

func TestOwnerOnlyChoiceAndLegalActionsDoNotLeakForeignHandOrContinuation(t *testing.T) {
	match := newProjectionMatch(t)
	state := match.State()
	owner := engine.SeatB
	hidden := state.Players[1].Objects[0].Ref()
	state.PendingChoices = []engine.PendingChoice{{
		ChoiceID: "choice-secret", Seat: owner, Kind: "opponent_discard_or_sacrifice", Mandatory: true,
		Visibility: engine.ChoiceOwnerOnly, MinSelections: 1, MaxSelections: 1,
		QueuePosition: engine.QueueBeforeExisting, Revision: state.Revision,
		Options:      []engine.ChoiceOption{{ID: "option-secret", Objects: []engine.ObjectRef{hidden}}},
		Continuation: &engine.ChoiceContinuation{Handler: "baz-036.front.ability:discard-or-sacrifice", Controller: engine.SeatA},
	}}
	codec := newReferenceCodec(bytes.Repeat([]byte{11}, 32))
	opponentView := projectPlayerView(state, engine.SeatA, codec, match.LegalActions(engine.SeatA))
	ownerView := projectPlayerView(state, owner, codec, match.LegalActions(owner))
	encoded, err := json.Marshal(opponentView)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"choice-secret", "option-secret", "opponent_discard_or_sacrifice", "baz-036.front.ability", hidden.ZoneObjectID} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("opponent projection leaked %q: %s", forbidden, encoded)
		}
	}
	if len(opponentView.Choices) != 1 || !opponentView.Choices[0].Redacted || opponentView.Choices[0].ChoiceID != "" || len(opponentView.Choices[0].Options) != 0 {
		t.Fatalf("opponent choice projection = %+v", opponentView.Choices)
	}
	if len(ownerView.Choices) != 1 || ownerView.Choices[0].ChoiceID != "choice-secret" || len(ownerView.Choices[0].Options) != 1 {
		t.Fatalf("owner choice projection = %+v", ownerView.Choices)
	}
	for _, action := range opponentView.LegalActions {
		for _, ref := range append(append([]objectRefPayload(nil), action.TargetRefs...), action.PaymentRefs...) {
			if ref == codec.encode(hidden) {
				t.Fatal("foreign hand ref leaked through legal action allowlist")
			}
		}
	}
}

func TestVisibleOptionalChoiceSerializesExplicitFalseAndZeroBounds(t *testing.T) {
	match := newProjectionMatch(t)
	state := match.State()
	state.PendingChoices = []engine.PendingChoice{{
		ChoiceID: "choice-optional", Seat: engine.SeatA, Kind: "optional_draw_two", Mandatory: false,
		Visibility: engine.ChoicePublic, MinSelections: 0, MaxSelections: 1,
		QueuePosition: engine.QueueBeforeExisting, Revision: state.Revision,
		Options: []engine.ChoiceOption{{ID: "activate"}},
	}}
	codec := newReferenceCodec(bytes.Repeat([]byte{13}, 32))
	owner := projectPlayerView(state, engine.SeatA, codec)
	observer := projectPlayerView(state, engine.SeatB, codec)
	for name, view := range map[string]PlayerView{"owner": owner, "observer": observer} {
		encoded, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		for _, required := range []string{`"mandatory":false`, `"minSelections":0`, `"maxSelections":1`, `"orderMatters":false`} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s optional projection omitted %s: %s", name, required, text)
			}
		}
	}
}

func TestPlayCardPayloadRequiresCurrentGenerationRef(t *testing.T) {
	state := engine.State{Players: [2]engine.PlayerState{{Seat: engine.SeatA}, {Seat: engine.SeatB}}}
	state.Players[0].Objects = []engine.CardObject{{
		InstanceID: "card", DefinitionID: "baz-121", OwnerSeat: engine.SeatA, ControllerSeat: engine.SeatA,
		Zone: engine.ZoneHand, ZoneObjectID: "card:g2", Generation: 2, Lane: -1,
	}}
	codec := newReferenceCodec(bytes.Repeat([]byte{12}, 32))
	limits, _ := normalizeSecurityLimits(SecurityLimits{})
	gateway := &Gateway{references: codec, limits: limits}
	encoded := codec.encode(state.Players[0].Objects[0].Ref())
	lane := 2
	payload, _ := json.Marshal(playCardPayload{SourceRef: encoded, DeclaredType: engine.CardSpell, Placement: nil})
	command, parseErr := gateway.engineCommand(state, application.ConnectionLease{Seat: application.SeatOne}, wsCommand{Payload: payload, Name: "playCard", CommandID: "play"})
	if parseErr != nil || command.Kind != engine.CommandPlayCard || command.SourceRef == nil || command.SourceRef.Generation != 2 || command.DeclaredType != engine.CardSpell {
		t.Fatalf("decoded command=%+v err=%+v", command, parseErr)
	}
	stale := encoded
	stale.Generation = 1
	payload, _ = json.Marshal(playCardPayload{SourceRef: stale, DeclaredType: engine.CardUnit, Placement: &playCardPlacement{Lane: &lane, Slot: engine.SlotUnit}})
	if _, parseErr := gateway.engineCommand(state, application.ConnectionLease{Seat: application.SeatOne}, wsCommand{Payload: payload, Name: "playCard", CommandID: "stale"}); parseErr == nil || parseErr.code != codeInvalidTarget {
		t.Fatalf("stale generation parse error = %+v", parseErr)
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
