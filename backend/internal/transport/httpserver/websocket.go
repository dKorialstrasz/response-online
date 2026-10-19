package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/application"
	"github.com/dKorialstrasz/response-online/backend/internal/engine"
	"github.com/gorilla/websocket"
)

type wsClient struct {
	connection *websocket.Conn
	lease      application.ConnectionLease
	writeMu    sync.Mutex
}

func (client *wsClient) write(event wsEvent) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	if err := client.connection.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return client.connection.WriteJSON(event)
}

func (client *wsClient) close(code int, reason string) {
	client.writeMu.Lock()
	_ = client.connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
	_ = client.connection.Close()
	client.writeMu.Unlock()
}

func (gateway *Gateway) serveWebSocket(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.ws.query_forbidden")
		return
	}
	upgrader := websocket.Upgrader{
		HandshakeTimeout:  5 * time.Second,
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: false,
		CheckOrigin: func(request *http.Request) bool {
			return gateway.allowedOrigin(request.Header.Get("Origin"))
		},
	}
	connection, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	client := &wsClient{connection: connection}
	connection.SetReadLimit(gateway.limits.WSFrameBytes)
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	messageType, content, err := connection.ReadMessage()
	if err != nil || messageType != websocket.TextMessage {
		client.close(websocket.ClosePolicyViolation, "authentication required")
		return
	}
	var auth wsAuth
	authErr := gateway.decodeWSJSON(content, &auth)
	authCorrelationID := auth.CorrelationID
	if !validIdentifier(authCorrelationID, 128) {
		_, authCorrelationID, _ = randomToken(12)
	}
	if authErr != nil || auth.ProtocolVersion != ProtocolVersion ||
		auth.Type != "authenticate" || !validIdentifier(auth.CorrelationID, 128) ||
		!validIdentifier(auth.ConnectionID, 128) || auth.Ticket == "" || len(auth.Ticket) > 256 {
		gateway.writeWSError(client, "", 0, 0, apiError{
			Code: codeInvalidPayload, Retryable: false, CorrelationID: authCorrelationID,
			MessageKey: "error.ws.authentication_invalid",
		})
		client.close(websocket.ClosePolicyViolation, "authentication failed")
		return
	}
	claim, err := gateway.service.RedeemConnectionTicket(auth.Ticket, auth.ConnectionID)
	auth.Ticket = ""
	for index := range content {
		content[index] = 0
	}
	if err != nil {
		gateway.writeWSError(client, "", 0, 0, apiError{
			Code: codeUnauthenticated, Retryable: false, CorrelationID: auth.CorrelationID,
			MessageKey: "error.session.unavailable",
		})
		client.close(websocket.ClosePolicyViolation, "authentication failed")
		return
	}
	defer gateway.service.ReleaseConnection(claim.Lease)
	client.lease = claim.Lease
	gateway.registerWSClient(client)
	defer gateway.unregisterWSClient(client)
	_ = connection.SetReadDeadline(time.Time{})
	room, err := gateway.service.SnapshotForConnection(claim.Lease)
	if err != nil {
		client.close(websocket.ClosePolicyViolation, "authorization failed")
		return
	}
	var matchSession *application.MatchSession
	var lifecycleEvents <-chan application.LifecycleEvent
	var subscriptionID uint64
	if room.Match != nil {
		matchSession, err = gateway.service.MatchSessionForConnection(claim.Lease)
		if err == nil {
			subscriptionID, lifecycleEvents, err = matchSession.Subscribe(request.Context(), seatForLease(claim.Lease))
		}
		if err != nil {
			client.close(websocket.ClosePolicyViolation, "match unavailable")
			return
		}
		defer matchSession.Unsubscribe(gateway.ctx, subscriptionID)
		connectErr := matchSession.Connected(request.Context(), seatForLease(claim.Lease))
		if connectErr != nil && !errors.Is(connectErr, engine.ErrMatchFinished) {
			client.close(websocket.ClosePolicyViolation, "match unavailable")
			return
		}
	}
	gateway.writeWSEvent(client, wsEvent{
		ProtocolVersion: ProtocolVersion, Type: "authenticated", CorrelationID: auth.CorrelationID,
		Revision: room.Revision, Payload: map[string]any{"seatId": claim.Lease.Seat},
	})
	gateway.writeWSEvent(client, wsEvent{
		ProtocolVersion: ProtocolVersion, Type: "roomSnapshot", CorrelationID: auth.CorrelationID,
		Revision: room.Revision, Payload: room,
	})
	if room.Match != nil {
		if event, snapshotErr := gateway.matchSnapshotEvent(request.Context(), claim.Lease, auth.CorrelationID, "matchSnapshot"); snapshotErr == nil {
			gateway.writeWSEvent(client, event)
		}
	}
	done := make(chan struct{})
	defer close(done)
	if matchSession != nil {
		go gateway.writeLifecycleEvents(request.Context(), client, claim.Lease, lifecycleEvents, done)
		go gateway.writeServerHeartbeats(client, done)
	}

	limiter := newTokenBucket(gateway.limits.WSCommandsPerSecond, gateway.limits.WSCommandBurst)
	for {
		messageType, content, err = connection.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			client.close(websocket.CloseUnsupportedData, "text frames only")
			return
		}
		if !limiter.allow() {
			gateway.writeWSError(client, "", 0, 0, apiError{
				Code: codeRateLimited, Retryable: true, MessageKey: "error.rate_limited",
			})
			continue
		}
		if err := gateway.service.AuthorizeConnection(claim.Lease); err != nil {
			client.close(websocket.ClosePolicyViolation, "connection replaced")
			return
		}
		var header struct {
			ProtocolVersion string `json:"protocolVersion"`
			Type            string `json:"type"`
		}
		if err := json.Unmarshal(content, &header); err != nil {
			gateway.writeWSError(client, "", 0, 0, apiError{Code: codeInvalidPayload, MessageKey: "error.payload.invalid"})
			continue
		}
		if header.ProtocolVersion != ProtocolVersion {
			gateway.writeWSError(client, "", 0, 0, apiError{Code: codeVersion, MessageKey: "error.version.unsupported"})
			continue
		}
		switch header.Type {
		case "heartbeat":
			var heartbeat wsHeartbeat
			if err := gateway.decodeWSJSON(content, &heartbeat); err != nil || !validIdentifier(heartbeat.CorrelationID, 128) {
				gateway.writeWSError(client, "", 0, 0, apiError{Code: codeInvalidPayload, MessageKey: "error.payload.invalid"})
				continue
			}
			if matchSession != nil {
				if err := matchSession.Heartbeat(request.Context(), seatForLease(claim.Lease)); err != nil {
					client.close(websocket.ClosePolicyViolation, "heartbeat timeout")
					return
				}
			}
			gateway.writeWSEvent(client, wsEvent{
				ProtocolVersion: ProtocolVersion, Type: "heartbeat", CorrelationID: heartbeat.CorrelationID,
			})
		case "command":
			var command wsCommand
			if err := gateway.decodeWSJSON(content, &command); err != nil || !validWSCommandEnvelope(command) {
				commandID := command.CommandID
				if !validIdentifier(commandID, 128) {
					commandID = ""
				}
				gateway.writeWSError(client, commandID, 0, 0, apiError{
					Code: codeInvalidPayload, MessageKey: "error.payload.invalid",
				})
				continue
			}
			event := gateway.executeWSCommand(request.Context(), claim.Lease, command)
			gateway.writeWSEvent(client, event)
			if event.Accepted != nil && *event.Accepted && command.Name != "resync" {
				gateway.broadcastMatchSnapshot(request.Context(), claim.Lease.RoomID, command.CommandID)
			}
		default:
			gateway.writeWSError(client, "", 0, 0, apiError{Code: codeInvalidPayload, MessageKey: "error.message_type.invalid"})
		}
	}
}

func (gateway *Gateway) writeServerHeartbeats(client *wsClient, done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_, correlationID, err := randomToken(12)
			if err != nil {
				return
			}
			gateway.writeWSEvent(client, wsEvent{
				ProtocolVersion: ProtocolVersion, Type: "heartbeat", CorrelationID: correlationID,
			})
		}
	}
}

func (gateway *Gateway) writeLifecycleEvents(
	ctx context.Context,
	client *wsClient,
	lease application.ConnectionLease,
	events <-chan application.LifecycleEvent,
	done <-chan struct{},
) {
	for {
		select {
		case <-done:
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			payload := lifecycleNoticePayload{Message: event.Message}
			out := wsEvent{ProtocolVersion: ProtocolVersion, Type: "lifecycleNotice", Payload: payload}
			if event.SnapshotRequired {
				if snapshot, err := gateway.matchSnapshotEvent(ctx, lease, "", "matchSnapshot"); err == nil {
					view, ok := snapshot.Payload.(PlayerView)
					if ok {
						payload.View = &view
						out.Payload = payload
						out.MatchID = snapshot.MatchID
						out.Revision = snapshot.Revision
						out.EventSeq = snapshot.EventSeq
					}
				}
			}
			gateway.writeWSEvent(client, out)
			if event.MatchFinished && payload.View != nil {
				gateway.writeWSEvent(client, wsEvent{
					ProtocolVersion: ProtocolVersion, Type: "matchFinished", MatchID: out.MatchID,
					Revision: out.Revision, EventSeq: out.EventSeq, Payload: *payload.View,
				})
			}
		}
	}
}

func validWSCommandEnvelope(command wsCommand) bool {
	return command.ProtocolVersion == ProtocolVersion && command.Type == "command" &&
		validIdentifier(command.CommandID, 128) && validIdentifier(command.Name, 80) &&
		validIdentifier(command.MatchID, 128) && len(command.Payload) > 0
}

func (gateway *Gateway) executeWSCommand(
	ctx context.Context,
	lease application.ConnectionLease,
	command wsCommand,
) wsEvent {
	fingerprint, err := commandFingerprint(command)
	if err != nil {
		return rejectedEvent(command, 0, 0, codeInvalidPayload, "error.payload.invalid", false, nil)
	}
	cacheKey := fmt.Sprintf("%s\x00%s\x00%s\x00%s", lease.RoomID, lease.Seat, command.MatchID, command.CommandID)
	gateway.cacheMu.Lock()
	defer gateway.cacheMu.Unlock()
	gateway.pruneCommandCacheLocked(time.Now().UTC())
	if cached, exists := gateway.commandCache[cacheKey]; exists {
		if cached.fingerprint == fingerprint {
			return cached.event
		}
		return rejectedEvent(command, cached.event.Revision, cached.event.EventSeq, codeCommandIDReused, "error.command_id.reused", false, nil)
	}

	session, err := gateway.service.MatchSessionForConnection(lease)
	if err != nil {
		return rejectedEvent(command, 0, 0, codeRoomUnavailable, "error.room.unavailable", false, nil)
	}
	state, err := session.Snapshot(ctx)
	if err != nil {
		return rejectedEvent(command, 0, 0, codeServer, "error.server.unavailable", true, nil)
	}
	if state.MatchID != command.MatchID {
		return rejectedEvent(command, state.Revision, eventSequence(state), codeForbidden, "error.access.forbidden", false, nil)
	}
	if command.Name == "resync" {
		var payload resyncPayload
		if err := gateway.decodeWSJSON(command.Payload, &payload); err != nil {
			return rejectedEvent(command, state.Revision, eventSequence(state), codeInvalidPayload, "error.payload.invalid", false, nil)
		}
		accepted := true
		event := wsEvent{
			ProtocolVersion: ProtocolVersion, Type: "matchSnapshot", CommandID: command.CommandID,
			MatchID: state.MatchID, Revision: state.Revision, EventSeq: eventSequence(state),
			Accepted: &accepted, Payload: projectPlayerView(state, seatForLease(lease), gateway.references),
		}
		gateway.commandCache[cacheKey] = cachedCommandResult{fingerprint: fingerprint, event: event, matchID: state.MatchID}
		return event
	}
	if command.Name == "requestRematch" {
		var payload rematchPayload
		if err := gateway.decodeWSJSON(command.Payload, &payload); err != nil {
			return rejectedEvent(command, state.Revision, eventSequence(state), codeInvalidPayload, "error.payload.invalid", false, nil)
		}
		if command.ExpectedRevision != state.Revision {
			view := projectPlayerView(state, seatForLease(lease), gateway.references)
			return rejectedEvent(command, state.Revision, eventSequence(state), codeStaleRevision, "error.revision.stale", false, &view)
		}
		room, started, err := gateway.service.RequestRematchForConnection(ctx, lease, command.MatchID, payload.Ready)
		if err != nil {
			code, key := engineError(err)
			return rejectedEvent(command, state.Revision, eventSequence(state), code, key, false, nil)
		}
		if started != nil {
			gateway.markMatchRunning(started.MatchID)
		}
		accepted := true
		event := wsEvent{
			ProtocolVersion: ProtocolVersion, Type: "commandResult", CommandID: command.CommandID,
			MatchID: command.MatchID, Revision: state.Revision, EventSeq: eventSequence(state),
			Accepted: &accepted, Payload: room,
		}
		gateway.commandCache[cacheKey] = cachedCommandResult{fingerprint: fingerprint, event: event, matchID: command.MatchID}
		return event
	}
	engineCommand, parseErr := gateway.engineCommand(state, lease, command)
	if parseErr != nil {
		event := rejectedEvent(command, state.Revision, eventSequence(state), parseErr.code, parseErr.messageKey, false, nil)
		gateway.commandCache[cacheKey] = cachedCommandResult{fingerprint: fingerprint, event: event, matchID: state.MatchID}
		return event
	}
	if err := gateway.service.SubmitForConnection(ctx, lease, engineCommand); err != nil {
		fresh, snapshotErr := session.Snapshot(ctx)
		if snapshotErr == nil {
			state = fresh
		}
		code, key := engineError(err)
		var view *PlayerView
		resync := false
		if code == codeStaleRevision {
			projected := projectPlayerView(state, seatForLease(lease), gateway.references)
			view = &projected
			resync = true
		}
		event := rejectedEvent(command, state.Revision, eventSequence(state), code, key, false, view)
		if event.Error != nil {
			event.Error.ResyncRequired = resync
		}
		entry := cachedCommandResult{fingerprint: fingerprint, event: event, matchID: state.MatchID}
		if state.Status == engine.StatusFinished {
			entry.finishedAt = time.Now().UTC()
		}
		gateway.commandCache[cacheKey] = entry
		gateway.markFinishedCacheLocked(state.MatchID, entry.finishedAt)
		return event
	}
	state, err = session.Snapshot(ctx)
	if err != nil {
		return rejectedEvent(command, 0, 0, codeServer, "error.server.unavailable", true, nil)
	}
	accepted := true
	event := wsEvent{
		ProtocolVersion: ProtocolVersion, Type: "commandResult", CommandID: command.CommandID,
		MatchID: state.MatchID, Revision: state.Revision, EventSeq: eventSequence(state), Accepted: &accepted,
	}
	entry := cachedCommandResult{fingerprint: fingerprint, event: event, matchID: state.MatchID}
	if state.Status == engine.StatusFinished {
		entry.finishedAt = time.Now().UTC()
	}
	gateway.commandCache[cacheKey] = entry
	gateway.markFinishedCacheLocked(state.MatchID, entry.finishedAt)
	return event
}

type commandParseError struct {
	code       errorCode
	messageKey string
}

func (gateway *Gateway) engineCommand(
	state engine.State,
	lease application.ConnectionLease,
	command wsCommand,
) (engine.Command, *commandParseError) {
	result := engine.Command{
		ID: command.CommandID, Seat: seatForLease(lease), ExpectedRevision: command.ExpectedRevision,
	}
	invalid := func(code errorCode, key string) (engine.Command, *commandParseError) {
		return engine.Command{}, &commandParseError{code: code, messageKey: key}
	}
	switch command.Name {
	case "submitMulligan":
		var payload mulliganPayload
		if err := gateway.decodeWSJSON(command.Payload, &payload); err != nil || len(payload.ObjectRefs) > engine.StartingHandSize || !uniqueObjectRefs(payload.ObjectRefs) {
			return invalid(codeInvalidPayload, "error.payload.invalid")
		}
		result.Kind = engine.CommandMulligan
		for _, ref := range payload.ObjectRefs {
			decoded, err := gateway.references.decode(state, ref)
			if err != nil {
				return invalid(codeInvalidTarget, "error.target.invalid")
			}
			result.ObjectRefs = append(result.ObjectRefs, decoded)
		}
	case "activateAbility":
		var payload abilityPayload
		if err := gateway.decodeWSJSON(command.Payload, &payload); err != nil || !validIdentifier(payload.AbilityID, 128) ||
			!validIdentifierList(payload.ModeIDs, 8, 128) || len(payload.TargetRefs) > 16 || len(payload.PaymentRefs) > 16 {
			return invalid(codeInvalidPayload, "error.payload.invalid")
		}
		result.Kind = engine.CommandActivateAbility
		result.AbilityID = payload.AbilityID
		result.ModeIDs = append([]string(nil), payload.ModeIDs...)
		if payload.SourceRef != nil {
			decoded, err := gateway.references.decode(state, *payload.SourceRef)
			if err != nil {
				return invalid(codeInvalidTarget, "error.target.invalid")
			}
			result.SourceRef = &decoded
		}
		for _, ref := range payload.TargetRefs {
			decoded, err := gateway.references.decode(state, ref)
			if err != nil {
				return invalid(codeInvalidTarget, "error.target.invalid")
			}
			result.TargetRefs = append(result.TargetRefs, decoded)
		}
		for _, ref := range payload.PaymentRefs {
			decoded, err := gateway.references.decode(state, ref)
			if err != nil {
				return invalid(codeInvalidTarget, "error.payment.invalid")
			}
			result.PaymentRefs = append(result.PaymentRefs, decoded)
		}
	case "passAction":
		var payload struct{}
		if err := gateway.decodeWSJSON(command.Payload, &payload); err != nil {
			return invalid(codeInvalidPayload, "error.payload.invalid")
		}
		result.Kind = engine.CommandPass
	case "submitChoice":
		var payload choicePayload
		if err := gateway.decodeWSJSON(command.Payload, &payload); err != nil || !validIdentifier(payload.ChoiceID, 128) ||
			!validIdentifierList(payload.OptionIDs, 32, 128) {
			return invalid(codeInvalidPayload, "error.payload.invalid")
		}
		result.Kind = engine.CommandSubmitChoice
		result.ChoiceID = payload.ChoiceID
		result.OptionIDs = append([]string(nil), payload.OptionIDs...)
	case "surrender":
		var payload struct{}
		if err := gateway.decodeWSJSON(command.Payload, &payload); err != nil {
			return invalid(codeInvalidPayload, "error.payload.invalid")
		}
		result.Kind = engine.CommandSurrender
	case "playCard":
		var payload playCardPayload
		if err := gateway.decodeWSJSON(command.Payload, &payload); err != nil || !validIdentifier(payload.SourceObjectID, 128) || !validIdentifier(payload.DeclaredType, 64) {
			return invalid(codeInvalidPayload, "error.payload.invalid")
		}
		return invalid(codeIllegalAction, "error.action.not_available")
	case "selectDeck", "setOrderArrangement", "setReady", "leaveRoom":
		return invalid(codeIllegalAction, "error.command.transport")
	default:
		return invalid(codeInvalidPayload, "error.command.unknown")
	}
	return result, nil
}

func (gateway *Gateway) matchSnapshotEvent(
	ctx context.Context,
	lease application.ConnectionLease,
	correlationID string,
	typeName string,
) (wsEvent, error) {
	session, err := gateway.service.MatchSessionForConnection(lease)
	if err != nil {
		return wsEvent{}, err
	}
	state, err := session.Snapshot(ctx)
	if err != nil {
		return wsEvent{}, err
	}
	return wsEvent{
		ProtocolVersion: ProtocolVersion, Type: typeName, CorrelationID: correlationID,
		MatchID: state.MatchID, Revision: state.Revision, EventSeq: eventSequence(state),
		Payload: projectPlayerView(state, seatForLease(lease), gateway.references),
	}, nil
}

func seatForLease(lease application.ConnectionLease) engine.Seat {
	if lease.Seat == application.SeatTwo {
		return engine.SeatB
	}
	return engine.SeatA
}

func eventSequence(state engine.State) uint64 {
	if len(state.Events) == 0 {
		return 0
	}
	return state.Events[len(state.Events)-1].Sequence
}

func engineError(err error) (errorCode, string) {
	switch {
	case errors.Is(err, application.ErrSessionLost):
		return codeUnauthenticated, "error.session.unavailable"
	case errors.Is(err, application.ErrConnectionReplaced), errors.Is(err, application.ErrForbidden):
		return codeForbidden, "error.access.forbidden"
	case errors.Is(err, application.ErrRoomUnavailable), errors.Is(err, application.ErrRoomExpired):
		return codeRoomUnavailable, "error.room.unavailable"
	case errors.Is(err, engine.ErrStaleRevision):
		return codeStaleRevision, "error.revision.stale"
	case errors.Is(err, engine.ErrMatchFinished):
		return codeMatchFinished, "error.match.finished"
	case strings.Contains(err.Error(), "acting seat"), strings.Contains(err.Error(), "pending choice"):
		return codeNotYourDecision, "error.decision.not_yours"
	default:
		return codeIllegalAction, "error.action.illegal"
	}
}

func rejectedEvent(
	command wsCommand,
	revision, eventSeq uint64,
	code errorCode,
	messageKey string,
	retryable bool,
	view *PlayerView,
) wsEvent {
	accepted := false
	return wsEvent{
		ProtocolVersion: ProtocolVersion, Type: "error", CommandID: command.CommandID,
		MatchID: command.MatchID, Revision: revision, EventSeq: eventSeq, Accepted: &accepted,
		Error: &apiError{Code: code, Retryable: retryable, CorrelationID: command.CommandID, MessageKey: messageKey, View: view},
	}
}

func commandFingerprint(command wsCommand) ([32]byte, error) {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(command.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return [32]byte{}, err
	}
	canonicalPayload, err := json.Marshal(payload)
	if err != nil {
		return [32]byte{}, err
	}
	value := struct {
		MatchID          string          `json:"matchId"`
		ExpectedRevision uint64          `json:"expectedRevision"`
		Name             string          `json:"name"`
		Payload          json.RawMessage `json:"payload"`
	}{command.MatchID, command.ExpectedRevision, command.Name, canonicalPayload}
	canonical, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(canonical), nil
}

func (gateway *Gateway) markFinishedCacheLocked(matchID string, finishedAt time.Time) {
	if finishedAt.IsZero() {
		return
	}
	for key, entry := range gateway.commandCache {
		if entry.matchID == matchID && entry.finishedAt.IsZero() {
			entry.finishedAt = finishedAt
			gateway.commandCache[key] = entry
		}
	}
}

func (gateway *Gateway) pruneCommandCacheLocked(now time.Time) {
	for key, entry := range gateway.commandCache {
		if !entry.finishedAt.IsZero() && now.Sub(entry.finishedAt) >= 15*time.Minute {
			delete(gateway.commandCache, key)
		}
	}
}

func (gateway *Gateway) registerWSClient(client *wsClient) {
	gateway.clientsMu.Lock()
	seats := gateway.clients[client.lease.RoomID]
	if seats == nil {
		seats = make(map[application.SeatID]*wsClient)
		gateway.clients[client.lease.RoomID] = seats
	}
	previous := seats[client.lease.Seat]
	seats[client.lease.Seat] = client
	gateway.clientsMu.Unlock()
	if previous != nil && previous != client {
		previous.close(websocket.ClosePolicyViolation, "connection replaced")
	}
}

func (gateway *Gateway) unregisterWSClient(client *wsClient) {
	gateway.clientsMu.Lock()
	defer gateway.clientsMu.Unlock()
	seats := gateway.clients[client.lease.RoomID]
	if seats == nil || seats[client.lease.Seat] != client {
		return
	}
	delete(seats, client.lease.Seat)
	if len(seats) == 0 {
		delete(gateway.clients, client.lease.RoomID)
	}
}

func (gateway *Gateway) clientsForRoom(roomID string) []*wsClient {
	gateway.clientsMu.Lock()
	defer gateway.clientsMu.Unlock()
	seats := gateway.clients[roomID]
	result := make([]*wsClient, 0, len(seats))
	for _, client := range seats {
		result = append(result, client)
	}
	return result
}

func (gateway *Gateway) broadcastMatchSnapshot(ctx context.Context, roomID, correlationID string) {
	clients := gateway.clientsForRoom(roomID)
	if len(clients) == 0 {
		return
	}
	session, err := gateway.service.MatchSessionForConnection(clients[0].lease)
	if err != nil {
		return
	}
	state, err := session.Snapshot(ctx)
	if err != nil {
		return
	}
	var writes sync.WaitGroup
	for _, client := range clients {
		view := projectPlayerView(state, seatForLease(client.lease), gateway.references)
		typeName := "stateUpdate"
		if state.Status == engine.StatusFinished {
			typeName = "matchFinished"
		} else if len(view.Choices) > 0 {
			typeName = "choiceRequested"
		}
		event := wsEvent{
			ProtocolVersion: ProtocolVersion, Type: typeName, CorrelationID: correlationID,
			MatchID: state.MatchID, Revision: state.Revision, EventSeq: eventSequence(state), Payload: view,
		}
		writes.Add(1)
		go func(client *wsClient, event wsEvent) {
			defer writes.Done()
			gateway.writeWSEvent(client, event)
		}(client, event)
	}
	writes.Wait()
}

func (gateway *Gateway) writeWSEvent(client *wsClient, event wsEvent) {
	if err := client.write(event); err != nil {
		client.close(websocket.CloseGoingAway, "write failed")
	}
}

func (gateway *Gateway) writeWSError(
	client *wsClient,
	commandID string,
	revision, eventSeq uint64,
	err apiError,
) {
	if !validIdentifier(err.CorrelationID, 128) {
		if validIdentifier(commandID, 128) {
			err.CorrelationID = commandID
		} else {
			_, err.CorrelationID, _ = randomToken(12)
		}
	}
	accepted := false
	gateway.writeWSEvent(client, wsEvent{
		ProtocolVersion: ProtocolVersion, Type: "error", CommandID: commandID,
		Revision: revision, EventSeq: eventSeq, Accepted: &accepted, Error: &err,
	})
}
