package application

import (
	"context"
	"errors"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

const (
	DefaultSessionTTL     = 24 * time.Hour
	DefaultEmptyRoomTTL   = 15 * time.Minute
	DefaultLobbyTTL       = 30 * time.Minute
	DefaultRoomTombstone  = 24 * time.Hour
	DefaultTicketTTL      = 30 * time.Second
	DefaultRoomCodeLength = 13
)

var (
	ErrInvalidInput          = errors.New("invalid input")
	ErrSessionLost           = errors.New("anonymous session is unavailable")
	ErrInvalidRoomCode       = errors.New("room code is invalid")
	ErrRoomExpired           = errors.New("room has expired")
	ErrRoomFull              = errors.New("room is full")
	ErrRoomUnavailable       = errors.New("room is unavailable")
	ErrForbidden             = errors.New("session does not control a room seat")
	ErrIdempotencyConflict   = errors.New("idempotency key was reused for another operation")
	ErrConnectionReplaced    = errors.New("controlling connection was replaced")
	ErrConnectionTicket      = errors.New("connection ticket is invalid or expired")
	ErrMatchSetupUnavailable = errors.New("match setup is unavailable")
)

type Clock interface {
	Now() time.Time
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

type SessionCredentials struct {
	SessionID string
	Secret    string
}

// AnonymousSession contains the only copy of the recovery secret returned by
// the application. The service stores only a digest of Secret.
type AnonymousSession struct {
	SessionID string
	Secret    string
	ExpiresAt time.Time
}

type SeatID string

const (
	SeatOne SeatID = "seat-1"
	SeatTwo SeatID = "seat-2"
)

func (seat SeatID) engineSeat() engine.Seat {
	if seat == SeatTwo {
		return engine.SeatB
	}
	return engine.SeatA
}

type RoomStatus string

const (
	RoomWaiting RoomStatus = "waiting"
	RoomActive  RoomStatus = "active"
)

type SeatSelection struct {
	DeckID   string
	OrderIDs []string
}

type SeatSnapshot struct {
	SeatID   SeatID
	Occupied bool
	Ready    bool
}

// RoomSnapshot is deliberately allowlisted. It contains no session IDs,
// recovery secrets, connection IDs, opponent deck choice, or engine state.
type RoomSnapshot struct {
	RoomID       string
	Code         string
	Status       RoomStatus
	Revision     uint64
	OwnSeat      SeatID
	OwnSelection SeatSelection
	Seats        [2]SeatSnapshot
	Match        *StartedMatch
}

type RoomAdmission struct {
	Room RoomSnapshot
	Seat SeatID
}

// PinnedDeck is a validated immutable input copied into a new engine match.
type PinnedDeck struct {
	DeckID            string
	ManifestID        string
	HeroID            string
	CardIDs           []string
	AvailableOrderIDs []string
	BaseInfluence     int
}

type MatchPins struct {
	Versions  engine.Versions
	Decks     [2]PinnedDeck
	Cards     []engine.CardDefinition
	Abilities []engine.AbilityDefinition
}

// MatchPinProvider validates both selected manifests against one content
// snapshot and returns the exact versions and data that must be pinned.
type MatchPinProvider interface {
	PinMatch(context.Context, [2]SeatSelection) (MatchPins, error)
}

type StartedMatch struct {
	MatchID         string
	StartedAt       time.Time
	Versions        engine.Versions
	DeckManifestIDs [2]string
	InitialRevision uint64
}

type ConnectionLease struct {
	RoomID       string
	Seat         SeatID
	ConnectionID string
	Generation   uint64
}

type ConnectionClaim struct {
	Lease                ConnectionLease
	ReplacedConnectionID string
}

type ConnectionTicket struct {
	Token     string
	ExpiresAt time.Time
}

type CleanupReport struct {
	Sessions int
	Rooms    int
	Tickets  int
}
