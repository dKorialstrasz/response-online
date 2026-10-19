// Package httpserver implements the versioned HTTP and WebSocket security
// boundary for the local M1 service.
package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/application"
	"github.com/dKorialstrasz/response-online/backend/internal/buildinfo"
	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

type Config struct {
	Context        context.Context
	Service        *application.RoomService
	AllowedOrigins []string
	SecureCookies  bool
	CSRFKey        []byte
	ReferenceKey   []byte
	Limits         SecurityLimits
}

type Gateway struct {
	ctx            context.Context
	service        *application.RoomService
	allowedOrigins map[string]struct{}
	secureCookies  bool
	csrfKey        []byte
	references     referenceCodec
	httpLimits     *httpRateLimiter
	limits         SecurityLimits

	startedMu      sync.Mutex
	startedMatches map[string]struct{}
	cacheMu        sync.Mutex
	commandCache   map[string]cachedCommandResult
	clientsMu      sync.Mutex
	clients        map[string]map[application.SeatID]*wsClient
}

// NewHandler preserves the diagnostic constructor used by tests and creates an
// isolated in-memory application service. Runtime wiring should use
// NewHandlerWithConfig so content pins and process cancellation are explicit.
func NewHandler() http.Handler {
	service, err := application.NewRoomService(application.RoomServiceConfig{})
	if err != nil {
		panic(err)
	}
	handler, err := NewHandlerWithConfig(Config{Service: service})
	if err != nil {
		panic(err)
	}
	return handler
}

func NewHandlerWithConfig(config Config) (http.Handler, error) {
	if config.Service == nil {
		return nil, fmt.Errorf("room service is required")
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	if len(config.AllowedOrigins) == 0 {
		config.AllowedOrigins = []string{
			"http://127.0.0.1:5173", "http://localhost:5173",
			"http://127.0.0.1:8080", "http://localhost:8080",
		}
	}
	csrfKey := append([]byte(nil), config.CSRFKey...)
	if len(csrfKey) == 0 {
		csrfKey = make([]byte, 32)
		if _, err := rand.Read(csrfKey); err != nil {
			return nil, fmt.Errorf("generate CSRF key: %w", err)
		}
	}
	referenceKey := append([]byte(nil), config.ReferenceKey...)
	if len(referenceKey) == 0 {
		referenceKey = make([]byte, 32)
		if _, err := rand.Read(referenceKey); err != nil {
			return nil, fmt.Errorf("generate reference key: %w", err)
		}
	}
	limits, err := normalizeSecurityLimits(config.Limits)
	if err != nil {
		return nil, err
	}
	gateway := &Gateway{
		ctx: config.Context, service: config.Service, secureCookies: config.SecureCookies,
		allowedOrigins: make(map[string]struct{}, len(config.AllowedOrigins)), csrfKey: csrfKey,
		references: newReferenceCodec(referenceKey), httpLimits: newHTTPRateLimiter(), limits: limits,
		startedMatches: make(map[string]struct{}), commandCache: make(map[string]cachedCommandResult),
		clients: make(map[string]map[application.SeatID]*wsClient),
	}
	for _, origin := range config.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
			parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
			return nil, fmt.Errorf("allowed origins must be exact scheme/host/port values")
		}
		gateway.allowedOrigins[origin] = struct{}{}
	}
	go gateway.runCommandCacheCleanup(config.Context)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", jsonHandler(http.StatusOK, map[string]string{"status": "ok"}))
	mux.HandleFunc("GET /readyz", jsonHandler(http.StatusOK, map[string]string{"status": "ready"}))
	mux.HandleFunc("GET /health/live", jsonHandler(http.StatusOK, map[string]string{"status": "ok"}))
	mux.HandleFunc("GET /health/ready", jsonHandler(http.StatusOK, map[string]string{"status": "ready"}))
	mux.HandleFunc("GET /api/v1/build", jsonHandler(http.StatusOK, buildinfo.Current()))
	mux.HandleFunc("POST /api/v1/sessions", gateway.createSession)
	mux.HandleFunc("POST /api/v1/rooms", gateway.createRoom)
	mux.HandleFunc("POST /api/v1/rooms/join", gateway.joinRoom)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}", gateway.getRoom)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/selection", gateway.setSelection)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/ready", gateway.setReady)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/leave", gateway.leaveRoom)
	mux.HandleFunc("POST /api/v1/ws-tickets", gateway.issueTicket)
	mux.HandleFunc("GET /api/v1/ws", gateway.serveWebSocket)
	return gateway.originMiddleware(mux), nil
}

func (gateway *Gateway) runCommandCacheCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			gateway.cacheMu.Lock()
			gateway.pruneCommandCacheLocked(now.UTC())
			gateway.cacheMu.Unlock()
		}
	}
}

func (gateway *Gateway) createSession(writer http.ResponseWriter, request *http.Request) {
	var body struct{}
	if err := gateway.decodeHTTPJSON(writer, request, &body); err != nil {
		gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.payload.invalid")
		return
	}
	session, err := gateway.service.CreateSession()
	if err != nil {
		gateway.writeError(writer, request, http.StatusServiceUnavailable, codeServer, true, "error.server.unavailable")
		return
	}
	gateway.setSessionCookie(writer, session.SessionID, session.Secret, session.ExpiresAt)
	gateway.writeJSON(writer, http.StatusCreated, sessionResponse{
		SessionID: session.SessionID, ExpiresAt: session.ExpiresAt,
		CSRFToken: csrfToken(session.Secret, gateway.csrfKey),
	})
}

func (gateway *Gateway) createRoom(writer http.ResponseWriter, request *http.Request) {
	credentials, sessionID, ok := gateway.authenticateHTTP(writer, request, true)
	if !ok {
		return
	}
	var body struct{}
	if err := gateway.decodeHTTPJSON(writer, request, &body); err != nil {
		gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.payload.invalid")
		return
	}
	if !gateway.httpLimits.allow("create:session:"+sessionID, gateway.limits.CreateRoomsPerSession, time.Minute) ||
		!gateway.httpLimits.allow("create:ip:"+clientIP(request), gateway.limits.CreateRoomsPerIP, time.Minute) {
		gateway.writeError(writer, request, http.StatusTooManyRequests, codeRateLimited, true, "error.rate_limited")
		return
	}
	result, err := gateway.service.CreateRoom(credentials, request.Header.Get("Idempotency-Key"))
	if err != nil {
		gateway.writeApplicationError(writer, request, err)
		return
	}
	gateway.writeJSON(writer, http.StatusCreated, roomResponse{Room: result.Room})
}

func (gateway *Gateway) joinRoom(writer http.ResponseWriter, request *http.Request) {
	credentials, sessionID, ok := gateway.authenticateHTTP(writer, request, true)
	if !ok {
		return
	}
	if !gateway.httpLimits.allow("join:session:"+sessionID, gateway.limits.JoinsPerSession, time.Minute) ||
		!gateway.httpLimits.allow("join:ip:"+clientIP(request), gateway.limits.JoinsPerIP, time.Minute) {
		gateway.writeError(writer, request, http.StatusTooManyRequests, codeRateLimited, true, "error.rate_limited")
		return
	}
	var body joinRoomRequest
	if err := gateway.decodeHTTPJSON(writer, request, &body); err != nil || !validIdentifier(body.Code, 32) {
		gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.payload.invalid")
		return
	}
	result, err := gateway.service.JoinRoom(credentials, body.Code, request.Header.Get("Idempotency-Key"))
	if err != nil {
		gateway.writeApplicationError(writer, request, err)
		return
	}
	gateway.writeJSON(writer, http.StatusOK, roomResponse{Room: result.Room})
}

func (gateway *Gateway) getRoom(writer http.ResponseWriter, request *http.Request) {
	credentials, _, ok := gateway.authenticateHTTP(writer, request, false)
	if !ok {
		return
	}
	room, err := gateway.service.Snapshot(credentials, request.PathValue("roomID"))
	if err != nil {
		gateway.writeApplicationError(writer, request, err)
		return
	}
	gateway.writeJSON(writer, http.StatusOK, roomResponse{Room: room})
}

func (gateway *Gateway) setSelection(writer http.ResponseWriter, request *http.Request) {
	credentials, _, ok := gateway.authenticateHTTP(writer, request, true)
	if !ok {
		return
	}
	var body selectionRequest
	if err := gateway.decodeHTTPJSON(writer, request, &body); err != nil || !validIdentifier(body.DeckID, 128) || len(body.OrderIDs) != engine.OrdersPerPlayer {
		gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.payload.invalid")
		return
	}
	for _, orderID := range body.OrderIDs {
		if !validIdentifier(orderID, 128) {
			gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.payload.invalid")
			return
		}
	}
	room, err := gateway.service.SetSelection(credentials, request.PathValue("roomID"), application.SeatSelection{
		DeckID: body.DeckID, OrderIDs: body.OrderIDs,
	})
	if err != nil {
		gateway.writeApplicationError(writer, request, err)
		return
	}
	gateway.writeJSON(writer, http.StatusOK, roomResponse{Room: room})
}

func (gateway *Gateway) setReady(writer http.ResponseWriter, request *http.Request) {
	credentials, _, ok := gateway.authenticateHTTP(writer, request, true)
	if !ok {
		return
	}
	var body readyRequest
	if err := gateway.decodeHTTPJSON(writer, request, &body); err != nil {
		gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.payload.invalid")
		return
	}
	room, started, err := gateway.service.SetReady(gateway.ctx, credentials, request.PathValue("roomID"), body.Ready)
	if err != nil {
		gateway.writeApplicationError(writer, request, err)
		return
	}
	if started != nil {
		gateway.ensureMatchRunning(credentials, request.PathValue("roomID"), started.MatchID)
	}
	gateway.writeJSON(writer, http.StatusOK, roomResponse{Room: room})
}

func (gateway *Gateway) leaveRoom(writer http.ResponseWriter, request *http.Request) {
	credentials, _, ok := gateway.authenticateHTTP(writer, request, true)
	if !ok {
		return
	}
	var body struct{}
	if err := gateway.decodeHTTPJSON(writer, request, &body); err != nil {
		gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.payload.invalid")
		return
	}
	room, err := gateway.service.LeaveRoom(credentials, request.PathValue("roomID"))
	if err != nil {
		gateway.writeApplicationError(writer, request, err)
		return
	}
	gateway.writeJSON(writer, http.StatusOK, roomResponse{Room: room})
}

func (gateway *Gateway) issueTicket(writer http.ResponseWriter, request *http.Request) {
	credentials, _, ok := gateway.authenticateHTTP(writer, request, true)
	if !ok {
		return
	}
	var body ticketRequest
	if err := gateway.decodeHTTPJSON(writer, request, &body); err != nil || !validIdentifier(body.RoomID, 128) {
		gateway.writeError(writer, request, http.StatusBadRequest, codeInvalidPayload, false, "error.payload.invalid")
		return
	}
	ticket, err := gateway.service.IssueConnectionTicket(credentials, body.RoomID)
	if err != nil {
		gateway.writeApplicationError(writer, request, err)
		return
	}
	gateway.writeJSON(writer, http.StatusCreated, ticketResponse{Ticket: ticket.Token, ExpiresAt: ticket.ExpiresAt})
}

func (gateway *Gateway) authenticateHTTP(
	writer http.ResponseWriter,
	request *http.Request,
	mutating bool,
) (application.SessionCredentials, string, bool) {
	sessionID, secret, ok := gateway.credentials(request)
	if !ok {
		gateway.writeError(writer, request, http.StatusUnauthorized, codeUnauthenticated, false, "error.session.unavailable")
		return application.SessionCredentials{}, "", false
	}
	if mutating && !gateway.verifyCSRF(request, secret) {
		gateway.writeError(writer, request, http.StatusForbidden, codeForbidden, false, "error.csrf.invalid")
		return application.SessionCredentials{}, "", false
	}
	credentials := application.SessionCredentials{SessionID: sessionID, Secret: secret}
	expiresAt, err := gateway.service.RefreshSession(credentials)
	if err != nil {
		gateway.writeApplicationError(writer, request, err)
		return application.SessionCredentials{}, "", false
	}
	gateway.setSessionCookie(writer, sessionID, secret, expiresAt)
	return credentials, sessionID, true
}

func (gateway *Gateway) setSessionCookie(writer http.ResponseWriter, sessionID, secret string, expiresAt time.Time) {
	http.SetCookie(writer, &http.Cookie{
		Name: sessionCookieName, Value: encodeSessionCookie(sessionID, secret),
		Path: "/", Expires: expiresAt, MaxAge: max(1, int(time.Until(expiresAt).Seconds())),
		HttpOnly: true, Secure: gateway.secureCookies, SameSite: http.SameSiteStrictMode,
	})
}

func (gateway *Gateway) ensureMatchRunning(credentials application.SessionCredentials, roomID, matchID string) {
	gateway.startedMu.Lock()
	defer gateway.startedMu.Unlock()
	if _, exists := gateway.startedMatches[matchID]; exists {
		return
	}
	session, err := gateway.service.MatchSession(credentials, roomID)
	if err != nil {
		return
	}
	gateway.startedMatches[matchID] = struct{}{}
	go func() { _ = session.Run(gateway.ctx) }()
}

func (gateway *Gateway) markMatchRunning(matchID string) {
	gateway.startedMu.Lock()
	gateway.startedMatches[matchID] = struct{}{}
	gateway.startedMu.Unlock()
}

func (gateway *Gateway) writeApplicationError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrSessionLost):
		gateway.writeError(writer, request, http.StatusUnauthorized, codeUnauthenticated, false, "error.session.unavailable")
	case errors.Is(err, application.ErrForbidden), errors.Is(err, application.ErrConnectionReplaced):
		gateway.writeError(writer, request, http.StatusForbidden, codeForbidden, false, "error.access.forbidden")
	case errors.Is(err, application.ErrRoomFull):
		gateway.writeError(writer, request, http.StatusConflict, codeRoomFull, false, "error.room.full")
	case errors.Is(err, application.ErrInvalidRoomCode), errors.Is(err, application.ErrRoomExpired), errors.Is(err, application.ErrRoomUnavailable):
		gateway.writeError(writer, request, http.StatusNotFound, codeRoomUnavailable, false, "error.room.unavailable")
	case errors.Is(err, application.ErrIdempotencyConflict):
		gateway.writeError(writer, request, http.StatusConflict, codeCommandIDReused, false, "error.idempotency.reused")
	case errors.Is(err, application.ErrInvalidInput):
		gateway.writeError(writer, request, http.StatusUnprocessableEntity, codeInvalidPayload, false, "error.payload.invalid")
	default:
		gateway.writeError(writer, request, http.StatusServiceUnavailable, codeServer, true, "error.server.unavailable")
	}
}

func (gateway *Gateway) writeError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code errorCode,
	retryable bool,
	messageKey string,
) {
	correlationID := request.Header.Get("X-Correlation-Id")
	if !validIdentifier(correlationID, 128) {
		_, correlationID, _ = randomToken(12)
	}
	gateway.writeJSON(writer, status, apiError{
		Code: code, Retryable: retryable, CorrelationID: correlationID, MessageKey: messageKey,
	})
}

func (gateway *Gateway) writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

func jsonHandler(status int, body any) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.WriteHeader(status)
		_ = json.NewEncoder(writer).Encode(body)
	}
}
