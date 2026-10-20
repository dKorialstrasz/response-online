package httpserver

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/application"
	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

const (
	ProtocolVersion = "1.1"
	maxHTTPBody     = 64 << 10
	maxWSFrame      = 32 << 10
	maxJSONDepth    = 16
)

// SecurityLimits makes the SEC-04/05 starting limits adjustable for controlled
// local tests and future deployment profiles. Zero values select the M1
// defaults; negative values are rejected during handler construction.
type SecurityLimits struct {
	CreateRoomsPerSession int
	CreateRoomsPerIP      int
	JoinsPerSession       int
	JoinsPerIP            int
	WSCommandsPerSecond   int
	WSCommandBurst        int
	HTTPBodyBytes         int64
	WSFrameBytes          int64
	JSONDepth             int
}

func normalizeSecurityLimits(limits SecurityLimits) (SecurityLimits, error) {
	values := []int{
		limits.CreateRoomsPerSession, limits.CreateRoomsPerIP, limits.JoinsPerSession,
		limits.JoinsPerIP, limits.WSCommandsPerSecond, limits.WSCommandBurst, limits.JSONDepth,
	}
	for _, value := range values {
		if value < 0 {
			return SecurityLimits{}, fmt.Errorf("security limits cannot be negative")
		}
	}
	if limits.HTTPBodyBytes < 0 || limits.WSFrameBytes < 0 {
		return SecurityLimits{}, fmt.Errorf("security limits cannot be negative")
	}
	if limits.CreateRoomsPerSession == 0 {
		limits.CreateRoomsPerSession = 5
	}
	if limits.CreateRoomsPerIP == 0 {
		limits.CreateRoomsPerIP = 20
	}
	if limits.JoinsPerSession == 0 {
		limits.JoinsPerSession = 10
	}
	if limits.JoinsPerIP == 0 {
		limits.JoinsPerIP = 30
	}
	if limits.WSCommandsPerSecond == 0 {
		limits.WSCommandsPerSecond = 20
	}
	if limits.WSCommandBurst == 0 {
		limits.WSCommandBurst = 40
	}
	if limits.HTTPBodyBytes == 0 {
		limits.HTTPBodyBytes = maxHTTPBody
	}
	if limits.WSFrameBytes == 0 {
		limits.WSFrameBytes = maxWSFrame
	}
	if limits.JSONDepth == 0 {
		limits.JSONDepth = maxJSONDepth
	}
	return limits, nil
}

type errorCode string

const (
	codeInvalidPayload   errorCode = "INVALID_PAYLOAD"
	codeUnauthenticated  errorCode = "UNAUTHENTICATED"
	codeForbidden        errorCode = "FORBIDDEN"
	codeRoomUnavailable  errorCode = "ROOM_UNAVAILABLE"
	codeRoomFull         errorCode = "ROOM_FULL"
	codeMatchFinished    errorCode = "MATCH_FINISHED"
	codeNotYourDecision  errorCode = "NOT_YOUR_DECISION"
	codeIllegalAction    errorCode = "ILLEGAL_ACTION"
	codeInvalidTarget    errorCode = "INVALID_TARGET"
	codeInsufficientCost errorCode = "INSUFFICIENT_COST"
	codeStaleRevision    errorCode = "STALE_REVISION"
	codeChoiceExpired    errorCode = "CHOICE_EXPIRED"
	codeCommandIDReused  errorCode = "COMMAND_ID_REUSED"
	codeRateLimited      errorCode = "RATE_LIMITED"
	codeVersion          errorCode = "VERSION_UNSUPPORTED"
	codeServer           errorCode = "SERVER_UNAVAILABLE"
)

type apiError struct {
	Code           errorCode   `json:"code"`
	Retryable      bool        `json:"retryable"`
	CorrelationID  string      `json:"correlationId"`
	MessageKey     string      `json:"messageKey"`
	ResyncRequired bool        `json:"resyncRequired,omitempty"`
	View           *PlayerView `json:"view,omitempty"`
}

type sessionResponse struct {
	SessionID string    `json:"sessionId"`
	ExpiresAt time.Time `json:"expiresAt"`
	CSRFToken string    `json:"csrfToken"`
}

type roomResponse struct {
	Room application.RoomSnapshot `json:"room"`
}

type joinRoomRequest struct {
	Code string `json:"code"`
}

type selectionRequest struct {
	DeckID   string   `json:"deckId"`
	OrderIDs []string `json:"orderIds"`
}

type readyRequest struct {
	Ready bool `json:"ready"`
}

type ticketRequest struct {
	RoomID string `json:"roomId"`
}

type ticketResponse struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type wsAuth struct {
	ProtocolVersion string `json:"protocolVersion"`
	Type            string `json:"type"`
	CorrelationID   string `json:"correlationId"`
	Ticket          string `json:"ticket"`
	ConnectionID    string `json:"connectionId"`
}

type wsCommand struct {
	ProtocolVersion  string          `json:"protocolVersion"`
	Type             string          `json:"type"`
	CommandID        string          `json:"commandId"`
	MatchID          string          `json:"matchId"`
	ExpectedRevision uint64          `json:"expectedRevision"`
	Name             string          `json:"name"`
	Payload          json.RawMessage `json:"payload"`
}

type wsHeartbeat struct {
	ProtocolVersion string `json:"protocolVersion"`
	Type            string `json:"type"`
	CorrelationID   string `json:"correlationId"`
}

type lifecycleNoticePayload struct {
	Message string      `json:"message"`
	View    *PlayerView `json:"view,omitempty"`
}

type wsEvent struct {
	ProtocolVersion string    `json:"protocolVersion"`
	Type            string    `json:"type"`
	CorrelationID   string    `json:"correlationId,omitempty"`
	CommandID       string    `json:"commandId,omitempty"`
	MatchID         string    `json:"matchId,omitempty"`
	Revision        uint64    `json:"revision"`
	EventSeq        uint64    `json:"eventSeq"`
	Accepted        *bool     `json:"accepted,omitempty"`
	Payload         any       `json:"payload,omitempty"`
	Error           *apiError `json:"error,omitempty"`
}

type objectRefPayload struct {
	ZoneObjectID string `json:"zoneObjectId"`
	Generation   uint64 `json:"generation"`
}

func (value objectRefPayload) engineRef() engine.ObjectRef {
	return engine.ObjectRef{ZoneObjectID: value.ZoneObjectID, Generation: value.Generation}
}

type mulliganPayload struct {
	ObjectRefs []objectRefPayload `json:"objectRefs"`
}

type abilityPayload struct {
	AbilityID         string             `json:"abilityId"`
	SourceRef         *objectRefPayload  `json:"sourceRef,omitempty"`
	ModeIDs           []string           `json:"modeIds,omitempty"`
	TargetRefs        []objectRefPayload `json:"targetRefs,omitempty"`
	PaymentRefs       []objectRefPayload `json:"paymentRefs,omitempty"`
	InfluencePayment  *int               `json:"influencePayment,omitempty"`
	ShieldPaymentRefs []objectRefPayload `json:"shieldPaymentRefs,omitempty"`
}

type choicePayload struct {
	ChoiceID  string   `json:"choiceId"`
	OptionIDs []string `json:"optionIds"`
}

type playCardPayload struct {
	SourceRef         objectRefPayload   `json:"sourceRef"`
	DeclaredType      engine.CardType    `json:"declaredType"`
	ModeIDs           []string           `json:"modeIds,omitempty"`
	TargetRefs        []objectRefPayload `json:"targetRefs,omitempty"`
	Placement         *playCardPlacement `json:"placement,omitempty"`
	PaymentChoice     *playCardPayment   `json:"paymentChoice,omitempty"`
	ShieldPaymentRefs []objectRefPayload `json:"shieldPaymentRefs,omitempty"`
}

type playCardPlacement struct {
	Lane *int             `json:"lane,omitempty"`
	Slot engine.BoardSlot `json:"slot,omitempty"`
}

type playCardPayment struct {
	ObjectRefs []objectRefPayload `json:"objectRefs,omitempty"`
}

type resyncPayload struct {
	LastEventSeq uint64 `json:"lastEventSeq,omitempty"`
}

type rematchPayload struct {
	Ready bool `json:"ready"`
}

type cachedCommandResult struct {
	fingerprint [32]byte
	event       wsEvent
	matchID     string
	finishedAt  time.Time
}
