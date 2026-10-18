package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

const maxIdentifierAttempts = 32

type RoomServiceConfig struct {
	Clock            Clock
	Entropy          io.Reader
	Pins             MatchPinProvider
	SessionTTL       time.Duration
	EmptyRoomTTL     time.Duration
	LobbyTTL         time.Duration
	RoomTombstoneTTL time.Duration
	TicketTTL        time.Duration
	RoomCodeLength   int
	MailboxCapacity  int
}

type sessionRecord struct {
	secretDigest [sha256.Size]byte
	expiresAt    time.Time
}

type seatRecord struct {
	sessionID            string
	selection            SeatSelection
	ready                bool
	connectionID         string
	connectionGeneration uint64
}

type roomRecord struct {
	id           string
	code         string
	status       RoomStatus
	revision     uint64
	createdAt    time.Time
	lastActivity time.Time
	seats        [2]seatRecord
	match        *matchRecord
	changed      chan struct{}
}

type matchRecord struct {
	started StartedMatch
	session *MatchSession
}

type idempotencyRecord struct {
	kind        string
	fingerprint [sha256.Size]byte
	roomID      string
	seat        SeatID
}

type roomTombstone struct {
	expiresAt time.Time
}

type ticketRecord struct {
	sessionID string
	roomID    string
	seat      SeatID
	expiresAt time.Time
}

// RoomService owns all mutable anonymous-session and lobby state. Its mutex is
// the transaction boundary for admission, ready/start, cleanup, and connection
// replacement; engine matches retain their own single-writer mailbox.
type RoomService struct {
	mu sync.Mutex

	clock            Clock
	entropy          io.Reader
	pins             MatchPinProvider
	sessionTTL       time.Duration
	emptyRoomTTL     time.Duration
	lobbyTTL         time.Duration
	roomTombstoneTTL time.Duration
	ticketTTL        time.Duration
	roomCodeLength   int
	mailboxCapacity  int

	sessions       map[string]*sessionRecord
	rooms          map[string]*roomRecord
	roomByCode     map[string]string
	expiredRoomIDs map[string]roomTombstone
	expiredCodes   map[string]roomTombstone
	idempotency    map[string]map[string]idempotencyRecord
	tickets        map[[sha256.Size]byte]ticketRecord
}

func NewRoomService(config RoomServiceConfig) (*RoomService, error) {
	if config.Clock == nil {
		config.Clock = wallClock{}
	}
	if config.Entropy == nil {
		config.Entropy = rand.Reader
	}
	if config.SessionTTL == 0 {
		config.SessionTTL = DefaultSessionTTL
	}
	if config.EmptyRoomTTL == 0 {
		config.EmptyRoomTTL = DefaultEmptyRoomTTL
	}
	if config.LobbyTTL == 0 {
		config.LobbyTTL = DefaultLobbyTTL
	}
	if config.RoomTombstoneTTL == 0 {
		config.RoomTombstoneTTL = DefaultRoomTombstone
	}
	if config.TicketTTL == 0 {
		config.TicketTTL = DefaultTicketTTL
	}
	if config.RoomCodeLength == 0 {
		config.RoomCodeLength = DefaultRoomCodeLength
	}
	if config.MailboxCapacity <= 0 {
		config.MailboxCapacity = 64
	}
	if config.SessionTTL <= 0 || config.EmptyRoomTTL <= 0 || config.LobbyTTL <= 0 ||
		config.RoomTombstoneTTL <= 0 || config.TicketTTL <= 0 || config.RoomCodeLength < 10 {
		return nil, fmt.Errorf("%w: positive TTLs and room code length >= 10 are required", ErrInvalidInput)
	}
	return &RoomService{
		clock:            config.Clock,
		entropy:          config.Entropy,
		pins:             config.Pins,
		sessionTTL:       config.SessionTTL,
		emptyRoomTTL:     config.EmptyRoomTTL,
		lobbyTTL:         config.LobbyTTL,
		roomTombstoneTTL: config.RoomTombstoneTTL,
		ticketTTL:        config.TicketTTL,
		roomCodeLength:   config.RoomCodeLength,
		mailboxCapacity:  config.MailboxCapacity,
		sessions:         make(map[string]*sessionRecord),
		rooms:            make(map[string]*roomRecord),
		roomByCode:       make(map[string]string),
		expiredRoomIDs:   make(map[string]roomTombstone),
		expiredCodes:     make(map[string]roomTombstone),
		idempotency:      make(map[string]map[string]idempotencyRecord),
		tickets:          make(map[[sha256.Size]byte]ticketRecord),
	}, nil
}

func (service *RoomService) CreateSession() (AnonymousSession, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	now := service.clock.Now().UTC()
	for range maxIdentifierAttempts {
		sessionID, err := service.randomOpaque("session_", 16)
		if err != nil {
			return AnonymousSession{}, err
		}
		if _, exists := service.sessions[sessionID]; exists {
			continue
		}
		secret, err := service.randomOpaque("", 32)
		if err != nil {
			return AnonymousSession{}, err
		}
		digest := sha256.Sum256([]byte(secret))
		expiresAt := now.Add(service.sessionTTL)
		service.sessions[sessionID] = &sessionRecord{secretDigest: digest, expiresAt: expiresAt}
		return AnonymousSession{SessionID: sessionID, Secret: secret, ExpiresAt: expiresAt}, nil
	}
	return AnonymousSession{}, fmt.Errorf("session identifier collision budget exhausted")
}

func (service *RoomService) CreateRoom(
	credentials SessionCredentials,
	idempotencyKey string,
) (RoomAdmission, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return RoomAdmission{}, err
	}
	key, err := validateIdempotencyKey(idempotencyKey)
	if err != nil {
		return RoomAdmission{}, err
	}
	if record, exists := service.idempotencyRecordLocked(credentials.SessionID, key); exists {
		if record.kind != "create" {
			return RoomAdmission{}, ErrIdempotencyConflict
		}
		return service.admissionForRecordLocked(credentials.SessionID, record)
	}

	roomID, err := service.uniqueRoomIDLocked()
	if err != nil {
		return RoomAdmission{}, err
	}
	code, err := service.uniqueRoomCodeLocked()
	if err != nil {
		return RoomAdmission{}, err
	}
	room := &roomRecord{
		id: roomID, code: code, status: RoomWaiting, revision: 1,
		createdAt: now, lastActivity: now, changed: make(chan struct{}),
	}
	room.seats[0].sessionID = credentials.SessionID
	service.rooms[roomID] = room
	service.roomByCode[code] = roomID
	service.putIdempotencyLocked(credentials.SessionID, key, idempotencyRecord{
		kind: "create", roomID: roomID, seat: SeatOne,
	})
	return RoomAdmission{Room: service.snapshotLocked(room, SeatOne), Seat: SeatOne}, nil
}

func (service *RoomService) JoinRoom(
	credentials SessionCredentials,
	code string,
	idempotencyKey string,
) (RoomAdmission, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return RoomAdmission{}, err
	}
	key, err := validateIdempotencyKey(idempotencyKey)
	if err != nil {
		return RoomAdmission{}, err
	}
	normalizedCode, valid := normalizeRoomCode(code, service.roomCodeLength)
	if !valid {
		return RoomAdmission{}, ErrInvalidRoomCode
	}
	fingerprint := sha256.Sum256([]byte(normalizedCode))
	if record, exists := service.idempotencyRecordLocked(credentials.SessionID, key); exists {
		if record.kind != "join" || subtle.ConstantTimeCompare(record.fingerprint[:], fingerprint[:]) != 1 {
			return RoomAdmission{}, ErrIdempotencyConflict
		}
		return service.admissionForRecordLocked(credentials.SessionID, record)
	}

	roomID, exists := service.roomByCode[normalizedCode]
	if !exists {
		if service.tombstoneActiveLocked(service.expiredCodes, normalizedCode, now) {
			return RoomAdmission{}, ErrRoomExpired
		}
		return RoomAdmission{}, ErrInvalidRoomCode
	}
	room := service.rooms[roomID]
	if room == nil || room.status != RoomWaiting {
		return RoomAdmission{}, ErrRoomUnavailable
	}
	if seat, ok := roomSeatForSession(room, credentials.SessionID); ok {
		record := idempotencyRecord{kind: "join", fingerprint: fingerprint, roomID: roomID, seat: seat}
		service.putIdempotencyLocked(credentials.SessionID, key, record)
		room.lastActivity = now
		return RoomAdmission{Room: service.snapshotLocked(room, seat), Seat: seat}, nil
	}

	seatIndex := -1
	for index := range room.seats {
		if room.seats[index].sessionID == "" {
			seatIndex = index
			break
		}
	}
	if seatIndex < 0 {
		return RoomAdmission{}, ErrRoomFull
	}
	seat := seatIDForIndex(seatIndex)
	room.seats[seatIndex].sessionID = credentials.SessionID
	room.lastActivity = now
	room.revision++
	service.signalRoomLocked(room)
	record := idempotencyRecord{kind: "join", fingerprint: fingerprint, roomID: roomID, seat: seat}
	service.putIdempotencyLocked(credentials.SessionID, key, record)
	return RoomAdmission{Room: service.snapshotLocked(room, seat), Seat: seat}, nil
}

func (service *RoomService) Snapshot(
	credentials SessionCredentials,
	roomID string,
) (RoomSnapshot, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return RoomSnapshot{}, err
	}
	room, seat, err := service.authorizedRoomLocked(credentials.SessionID, roomID, now)
	if err != nil {
		return RoomSnapshot{}, err
	}
	if room.status == RoomWaiting {
		room.lastActivity = now
	}
	return service.snapshotLocked(room, seat), nil
}

func (service *RoomService) SetSelection(
	credentials SessionCredentials,
	roomID string,
	selection SeatSelection,
) (RoomSnapshot, error) {
	selection, err := normalizeSelection(selection)
	if err != nil {
		return RoomSnapshot{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return RoomSnapshot{}, err
	}
	room, seat, err := service.authorizedRoomLocked(credentials.SessionID, roomID, now)
	if err != nil {
		return RoomSnapshot{}, err
	}
	if room.status != RoomWaiting {
		return RoomSnapshot{}, ErrRoomUnavailable
	}
	seatRecord := &room.seats[seatIndex(seat)]
	if selectionsEqual(seatRecord.selection, selection) {
		room.lastActivity = now
		return service.snapshotLocked(room, seat), nil
	}
	seatRecord.selection = cloneSelection(selection)
	seatRecord.ready = false
	room.lastActivity = now
	room.revision++
	service.signalRoomLocked(room)
	return service.snapshotLocked(room, seat), nil
}

func (service *RoomService) SetReady(
	ctx context.Context,
	credentials SessionCredentials,
	roomID string,
	ready bool,
) (RoomSnapshot, *StartedMatch, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return RoomSnapshot{}, nil, err
	}
	room, seat, err := service.authorizedRoomLocked(credentials.SessionID, roomID, now)
	if err != nil {
		return RoomSnapshot{}, nil, err
	}
	if room.status == RoomActive && ready && room.match != nil {
		started := cloneStartedMatch(room.match.started)
		return service.snapshotLocked(room, seat), &started, nil
	}
	if room.status != RoomWaiting {
		return RoomSnapshot{}, nil, ErrRoomUnavailable
	}
	staged := cloneRoom(room)
	stagedSeat := &staged.seats[seatIndex(seat)]
	if ready && stagedSeat.selection.DeckID == "" {
		return RoomSnapshot{}, nil, fmt.Errorf("%w: select a deck and five orders first", ErrInvalidInput)
	}
	if stagedSeat.ready != ready {
		stagedSeat.ready = ready
		staged.revision++
	}
	staged.lastActivity = now

	var started *StartedMatch
	if staged.seats[0].ready && staged.seats[1].ready {
		match, err := service.prepareMatchLocked(ctx, staged, now)
		if err != nil {
			return RoomSnapshot{}, nil, err
		}
		staged.status = RoomActive
		staged.match = match
		staged.revision++
		copy := cloneStartedMatch(match.started)
		started = &copy
	}
	room.status = staged.status
	room.revision = staged.revision
	room.lastActivity = staged.lastActivity
	room.seats = staged.seats
	room.match = staged.match
	service.signalRoomLocked(room)
	return service.snapshotLocked(room, seat), started, nil
}

func (service *RoomService) LeaveRoom(
	credentials SessionCredentials,
	roomID string,
) (RoomSnapshot, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return RoomSnapshot{}, err
	}
	room, seat, err := service.authorizedRoomLocked(credentials.SessionID, roomID, now)
	if err != nil {
		return RoomSnapshot{}, err
	}
	if room.status != RoomWaiting {
		return RoomSnapshot{}, ErrRoomUnavailable
	}
	room.seats[seatIndex(seat)] = seatRecord{}
	for index := range room.seats {
		room.seats[index].ready = false
	}
	room.lastActivity = now
	room.revision++
	service.signalRoomLocked(room)
	return service.snapshotLocked(room, ""), nil
}

func (service *RoomService) WaitForRoomChange(
	ctx context.Context,
	credentials SessionCredentials,
	roomID string,
	afterRevision uint64,
) (RoomSnapshot, error) {
	service.mu.Lock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		service.mu.Unlock()
		return RoomSnapshot{}, err
	}
	for {
		session := service.sessions[credentials.SessionID]
		if session == nil || !service.clock.Now().UTC().Before(session.expiresAt) {
			service.mu.Unlock()
			return RoomSnapshot{}, ErrSessionLost
		}
		room, seat, err := service.authorizedRoomLocked(credentials.SessionID, roomID, service.clock.Now().UTC())
		if err != nil {
			service.mu.Unlock()
			return RoomSnapshot{}, err
		}
		if room.revision > afterRevision {
			snapshot := service.snapshotLocked(room, seat)
			service.mu.Unlock()
			return snapshot, nil
		}
		changed := room.changed
		service.mu.Unlock()
		select {
		case <-ctx.Done():
			return RoomSnapshot{}, ctx.Err()
		case <-changed:
		}
		service.mu.Lock()
	}
}

func (service *RoomService) IssueConnectionTicket(
	credentials SessionCredentials,
	roomID string,
) (ConnectionTicket, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return ConnectionTicket{}, err
	}
	_, seat, err := service.authorizedRoomLocked(credentials.SessionID, roomID, now)
	if err != nil {
		return ConnectionTicket{}, err
	}
	for range maxIdentifierAttempts {
		token, err := service.randomOpaque("", 32)
		if err != nil {
			return ConnectionTicket{}, err
		}
		digest := sha256.Sum256([]byte(token))
		if _, exists := service.tickets[digest]; exists {
			continue
		}
		expiresAt := now.Add(service.ticketTTL)
		service.tickets[digest] = ticketRecord{
			sessionID: credentials.SessionID, roomID: roomID, seat: seat, expiresAt: expiresAt,
		}
		return ConnectionTicket{Token: token, ExpiresAt: expiresAt}, nil
	}
	return ConnectionTicket{}, fmt.Errorf("connection ticket collision budget exhausted")
}

func (service *RoomService) RedeemConnectionTicket(
	token string,
	connectionID string,
) (ConnectionClaim, error) {
	if token == "" || !validConnectionID(connectionID) {
		return ConnectionClaim{}, ErrConnectionTicket
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	digest := sha256.Sum256([]byte(token))
	record, exists := service.tickets[digest]
	if !exists {
		return ConnectionClaim{}, ErrConnectionTicket
	}
	delete(service.tickets, digest)
	if !now.Before(record.expiresAt) {
		return ConnectionClaim{}, ErrConnectionTicket
	}
	session := service.sessions[record.sessionID]
	if session == nil || !now.Before(session.expiresAt) {
		if session != nil {
			delete(service.sessions, record.sessionID)
		}
		return ConnectionClaim{}, ErrSessionLost
	}
	session.expiresAt = now.Add(service.sessionTTL)
	return service.claimConnectionLocked(record.sessionID, record.roomID, record.seat, connectionID, now)
}

func (service *RoomService) ReplaceConnection(
	credentials SessionCredentials,
	roomID string,
	connectionID string,
) (ConnectionClaim, error) {
	if !validConnectionID(connectionID) {
		return ConnectionClaim{}, fmt.Errorf("%w: connection id is required", ErrInvalidInput)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return ConnectionClaim{}, err
	}
	_, seat, err := service.authorizedRoomLocked(credentials.SessionID, roomID, now)
	if err != nil {
		return ConnectionClaim{}, err
	}
	return service.claimConnectionLocked(credentials.SessionID, roomID, seat, connectionID, now)
}

func (service *RoomService) AuthorizeConnection(lease ConnectionLease) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	room := service.rooms[lease.RoomID]
	if room == nil {
		if service.tombstoneActiveLocked(service.expiredRoomIDs, lease.RoomID, now) {
			return ErrRoomExpired
		}
		return ErrRoomUnavailable
	}
	index := seatIndex(lease.Seat)
	if index < 0 {
		return ErrConnectionReplaced
	}
	seat := &room.seats[index]
	session := service.sessions[seat.sessionID]
	if session == nil || !now.Before(session.expiresAt) {
		return ErrSessionLost
	}
	if seat.connectionID != lease.ConnectionID || seat.connectionGeneration != lease.Generation {
		return ErrConnectionReplaced
	}
	session.expiresAt = now.Add(service.sessionTTL)
	if room.status == RoomWaiting {
		room.lastActivity = now
	}
	return nil
}

func (service *RoomService) ReleaseConnection(lease ConnectionLease) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	room := service.rooms[lease.RoomID]
	index := seatIndex(lease.Seat)
	if room == nil || index < 0 {
		return ErrConnectionReplaced
	}
	seat := &room.seats[index]
	if seat.connectionID != lease.ConnectionID || seat.connectionGeneration != lease.Generation {
		return ErrConnectionReplaced
	}
	seat.connectionID = ""
	seat.connectionGeneration++
	return nil
}

func (service *RoomService) RevokeSession(credentials SessionCredentials) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return err
	}
	delete(service.sessions, credentials.SessionID)
	delete(service.idempotency, credentials.SessionID)
	for digest, ticket := range service.tickets {
		if ticket.sessionID == credentials.SessionID {
			delete(service.tickets, digest)
		}
	}
	for _, room := range service.rooms {
		revokedInRoom := false
		for index := range room.seats {
			if room.seats[index].sessionID == credentials.SessionID {
				room.seats[index].connectionID = ""
				room.seats[index].connectionGeneration++
				revokedInRoom = true
			}
		}
		if revokedInRoom {
			service.signalRoomLocked(room)
		}
	}
	return nil
}

func (service *RoomService) MatchSession(
	credentials SessionCredentials,
	roomID string,
) (*MatchSession, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	if _, err := service.authenticateLocked(credentials, now); err != nil {
		return nil, err
	}
	room, _, err := service.authorizedRoomLocked(credentials.SessionID, roomID, now)
	if err != nil {
		return nil, err
	}
	if room.match == nil {
		return nil, ErrRoomUnavailable
	}
	return room.match.session, nil
}

func (service *RoomService) CleanupExpired() CleanupReport {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.clock.Now().UTC()
	report := CleanupReport{}
	for sessionID, session := range service.sessions {
		if !now.Before(session.expiresAt) {
			delete(service.sessions, sessionID)
			delete(service.idempotency, sessionID)
			for _, room := range service.rooms {
				if _, controlsSeat := roomSeatForSession(room, sessionID); controlsSeat {
					service.signalRoomLocked(room)
				}
			}
			report.Sessions++
		}
	}
	for digest, ticket := range service.tickets {
		if !now.Before(ticket.expiresAt) {
			delete(service.tickets, digest)
			report.Tickets++
		}
	}
	for roomID, room := range service.rooms {
		if room.status == RoomActive || room.match != nil {
			continue
		}
		ttl := service.lobbyTTL
		if occupiedSeats(room) == 0 {
			ttl = service.emptyRoomTTL
		}
		if now.Sub(room.lastActivity) >= ttl {
			service.expireRoomLocked(room, now)
			delete(service.rooms, roomID)
			report.Rooms++
		}
	}
	service.pruneTombstonesLocked(now)
	return report
}

// RunCleanup executes TTL cleanup until the owning process is stopped. The
// interval is operational configuration; all expiry decisions still use the
// injected service clock.
func (service *RoomService) RunCleanup(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("%w: cleanup interval must be positive", ErrInvalidInput)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			service.CleanupExpired()
		}
	}
}

func (service *RoomService) prepareMatchLocked(
	ctx context.Context,
	room *roomRecord,
	now time.Time,
) (*matchRecord, error) {
	if service.pins == nil {
		return nil, ErrMatchSetupUnavailable
	}
	var selections [2]SeatSelection
	for index := range room.seats {
		if room.seats[index].sessionID == "" || room.seats[index].selection.DeckID == "" {
			return nil, fmt.Errorf("%w: both occupied seats need a valid selection", ErrMatchSetupUnavailable)
		}
		session := service.sessions[room.seats[index].sessionID]
		if session == nil || !now.Before(session.expiresAt) {
			return nil, ErrSessionLost
		}
		selections[index] = cloneSelection(room.seats[index].selection)
	}
	pins, err := service.pins.PinMatch(ctx, selections)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMatchSetupUnavailable, err)
	}
	matchID, err := service.uniqueMatchIDLocked()
	if err != nil {
		return nil, err
	}
	seedBytes := make([]byte, 8)
	if _, err := io.ReadFull(service.entropy, seedBytes); err != nil {
		return nil, fmt.Errorf("generate match seed: %w", err)
	}
	config := engine.Config{
		MatchID: matchID, Versions: pins.Versions, Seed: binary.BigEndian.Uint64(seedBytes), StartedAt: now,
		Cards:     append([]engine.CardDefinition(nil), pins.Cards...),
		Abilities: append([]engine.AbilityDefinition(nil), pins.Abilities...),
	}
	var manifestIDs [2]string
	for index := range pins.Decks {
		deck := pins.Decks[index]
		if deck.DeckID != selections[index].DeckID || deck.ManifestID == "" {
			return nil, fmt.Errorf("%w: pin provider returned a mismatched deck", ErrMatchSetupUnavailable)
		}
		manifestIDs[index] = deck.ManifestID
		config.Players[index] = engine.PlayerConfig{
			Seat: engine.Seat(index), HeroID: deck.HeroID,
			DeckCardIDs:   append([]string(nil), deck.CardIDs...),
			AvailableIDs:  append([]string(nil), deck.AvailableOrderIDs...),
			BaseInfluence: deck.BaseInfluence,
		}
	}
	match, err := engine.NewMatch(config)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid pinned match: %v", ErrMatchSetupUnavailable, err)
	}
	for index := range selections {
		command := engine.Command{
			ID: fmt.Sprintf("lobby-orders-%d", index+1), Kind: engine.CommandArrangeOrders,
			Seat: engine.Seat(index), ExpectedRevision: uint64(index),
			OrderIDs: append([]string(nil), selections[index].OrderIDs...),
		}
		if err := match.Handle(command); err != nil {
			return nil, fmt.Errorf("%w: pinned order arrangement is invalid: %v", ErrMatchSetupUnavailable, err)
		}
	}
	matchSession, err := NewMatchSession(match, service.mailboxCapacity)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMatchSetupUnavailable, err)
	}
	state := match.State()
	for index := range room.seats {
		service.sessions[room.seats[index].sessionID].expiresAt = now.Add(service.sessionTTL)
	}
	return &matchRecord{
		started: StartedMatch{
			MatchID: matchID, StartedAt: now, Versions: pins.Versions,
			DeckManifestIDs: manifestIDs, InitialRevision: state.Revision,
		},
		session: matchSession,
	}, nil
}

func (service *RoomService) authenticateLocked(
	credentials SessionCredentials,
	now time.Time,
) (*sessionRecord, error) {
	if credentials.SessionID == "" || credentials.Secret == "" {
		return nil, ErrSessionLost
	}
	session := service.sessions[credentials.SessionID]
	if session == nil || !now.Before(session.expiresAt) {
		if session != nil {
			delete(service.sessions, credentials.SessionID)
			delete(service.idempotency, credentials.SessionID)
		}
		return nil, ErrSessionLost
	}
	digest := sha256.Sum256([]byte(credentials.Secret))
	if subtle.ConstantTimeCompare(digest[:], session.secretDigest[:]) != 1 {
		return nil, ErrSessionLost
	}
	session.expiresAt = now.Add(service.sessionTTL)
	return session, nil
}

func (service *RoomService) authorizedRoomLocked(
	sessionID string,
	roomID string,
	now time.Time,
) (*roomRecord, SeatID, error) {
	room := service.rooms[roomID]
	if room == nil {
		if service.tombstoneActiveLocked(service.expiredRoomIDs, roomID, now) {
			return nil, "", ErrRoomExpired
		}
		return nil, "", ErrRoomUnavailable
	}
	seat, ok := roomSeatForSession(room, sessionID)
	if !ok {
		return nil, "", ErrForbidden
	}
	return room, seat, nil
}

func (service *RoomService) claimConnectionLocked(
	sessionID string,
	roomID string,
	seat SeatID,
	connectionID string,
	now time.Time,
) (ConnectionClaim, error) {
	room := service.rooms[roomID]
	index := seatIndex(seat)
	if room == nil || index < 0 || room.seats[index].sessionID != sessionID {
		return ConnectionClaim{}, ErrForbidden
	}
	current := &room.seats[index]
	if current.connectionID == connectionID {
		return ConnectionClaim{Lease: ConnectionLease{
			RoomID: roomID, Seat: seat, ConnectionID: connectionID, Generation: current.connectionGeneration,
		}}, nil
	}
	replaced := current.connectionID
	current.connectionGeneration++
	current.connectionID = connectionID
	if room.status == RoomWaiting {
		room.lastActivity = now
	}
	return ConnectionClaim{
		Lease: ConnectionLease{
			RoomID: roomID, Seat: seat, ConnectionID: connectionID, Generation: current.connectionGeneration,
		},
		ReplacedConnectionID: replaced,
	}, nil
}

func (service *RoomService) admissionForRecordLocked(
	sessionID string,
	record idempotencyRecord,
) (RoomAdmission, error) {
	now := service.clock.Now().UTC()
	room := service.rooms[record.roomID]
	if room == nil {
		if service.tombstoneActiveLocked(service.expiredRoomIDs, record.roomID, now) {
			return RoomAdmission{}, ErrRoomExpired
		}
		return RoomAdmission{}, ErrRoomUnavailable
	}
	index := seatIndex(record.seat)
	if index < 0 || room.seats[index].sessionID != sessionID {
		return RoomAdmission{}, ErrRoomUnavailable
	}
	room.lastActivity = now
	return RoomAdmission{Room: service.snapshotLocked(room, record.seat), Seat: record.seat}, nil
}

func (service *RoomService) snapshotLocked(room *roomRecord, ownSeat SeatID) RoomSnapshot {
	snapshot := RoomSnapshot{
		RoomID: room.id, Code: room.code, Status: room.status,
		Revision: room.revision, OwnSeat: ownSeat,
		Seats: [2]SeatSnapshot{
			{SeatID: SeatOne, Occupied: room.seats[0].sessionID != "", Ready: room.seats[0].ready},
			{SeatID: SeatTwo, Occupied: room.seats[1].sessionID != "", Ready: room.seats[1].ready},
		},
	}
	if index := seatIndex(ownSeat); index >= 0 {
		snapshot.OwnSelection = cloneSelection(room.seats[index].selection)
	}
	if room.match != nil {
		match := cloneStartedMatch(room.match.started)
		snapshot.Match = &match
	}
	return snapshot
}

func (service *RoomService) uniqueRoomIDLocked() (string, error) {
	for range maxIdentifierAttempts {
		roomID, err := service.randomOpaque("room_", 16)
		if err != nil {
			return "", err
		}
		if _, exists := service.rooms[roomID]; exists {
			continue
		}
		if _, exists := service.expiredRoomIDs[roomID]; !exists {
			return roomID, nil
		}
	}
	return "", fmt.Errorf("room identifier collision budget exhausted")
}

func (service *RoomService) uniqueRoomCodeLocked() (string, error) {
	bytesNeeded := (service.roomCodeLength*5 + 7) / 8
	for range maxIdentifierAttempts {
		randomBytes := make([]byte, bytesNeeded)
		if _, err := io.ReadFull(service.entropy, randomBytes); err != nil {
			return "", fmt.Errorf("generate room code: %w", err)
		}
		code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes)
		code = code[:service.roomCodeLength]
		if _, exists := service.roomByCode[code]; exists {
			continue
		}
		if _, exists := service.expiredCodes[code]; !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("room code collision budget exhausted")
}

func (service *RoomService) uniqueMatchIDLocked() (string, error) {
	for range maxIdentifierAttempts {
		matchID, err := service.randomOpaque("match_", 16)
		if err != nil {
			return "", err
		}
		collision := false
		for _, room := range service.rooms {
			if room.match != nil && room.match.started.MatchID == matchID {
				collision = true
				break
			}
		}
		if !collision {
			return matchID, nil
		}
	}
	return "", fmt.Errorf("match identifier collision budget exhausted")
}

func (service *RoomService) randomOpaque(prefix string, byteCount int) (string, error) {
	randomBytes := make([]byte, byteCount)
	if _, err := io.ReadFull(service.entropy, randomBytes); err != nil {
		return "", fmt.Errorf("generate opaque identifier: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func (service *RoomService) expireRoomLocked(room *roomRecord, now time.Time) {
	delete(service.roomByCode, room.code)
	tombstone := roomTombstone{expiresAt: now.Add(service.roomTombstoneTTL)}
	service.expiredRoomIDs[room.id] = tombstone
	service.expiredCodes[room.code] = tombstone
	service.signalRoomLocked(room)
}

func (service *RoomService) pruneTombstonesLocked(now time.Time) {
	for key, tombstone := range service.expiredRoomIDs {
		if !now.Before(tombstone.expiresAt) {
			delete(service.expiredRoomIDs, key)
		}
	}
	for key, tombstone := range service.expiredCodes {
		if !now.Before(tombstone.expiresAt) {
			delete(service.expiredCodes, key)
		}
	}
}

func (service *RoomService) tombstoneActiveLocked(
	tombstones map[string]roomTombstone,
	key string,
	now time.Time,
) bool {
	tombstone, exists := tombstones[key]
	return exists && now.Before(tombstone.expiresAt)
}

func (service *RoomService) signalRoomLocked(room *roomRecord) {
	close(room.changed)
	room.changed = make(chan struct{})
}

func (service *RoomService) idempotencyRecordLocked(sessionID, key string) (idempotencyRecord, bool) {
	records := service.idempotency[sessionID]
	if records == nil {
		return idempotencyRecord{}, false
	}
	record, exists := records[key]
	return record, exists
}

func (service *RoomService) putIdempotencyLocked(sessionID, key string, record idempotencyRecord) {
	if service.idempotency[sessionID] == nil {
		service.idempotency[sessionID] = make(map[string]idempotencyRecord)
	}
	service.idempotency[sessionID][key] = record
}

func normalizeRoomCode(code string, expectedLength int) (string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != expectedLength {
		return "", false
	}
	for _, character := range code {
		if (character < 'A' || character > 'Z') && (character < '2' || character > '7') {
			return "", false
		}
	}
	return code, true
}

func normalizeSelection(selection SeatSelection) (SeatSelection, error) {
	selection.DeckID = strings.TrimSpace(selection.DeckID)
	if selection.DeckID == "" || len(selection.DeckID) > 128 || len(selection.OrderIDs) != engine.OrdersPerPlayer {
		return SeatSelection{}, fmt.Errorf("%w: deck id and exactly five orders are required", ErrInvalidInput)
	}
	seen := make(map[string]struct{}, len(selection.OrderIDs))
	orders := make([]string, len(selection.OrderIDs))
	for index, orderID := range selection.OrderIDs {
		orderID = strings.TrimSpace(orderID)
		if orderID == "" || len(orderID) > 128 {
			return SeatSelection{}, fmt.Errorf("%w: order ids must be non-empty", ErrInvalidInput)
		}
		if _, exists := seen[orderID]; exists {
			return SeatSelection{}, fmt.Errorf("%w: order ids must be unique", ErrInvalidInput)
		}
		seen[orderID] = struct{}{}
		orders[index] = orderID
	}
	selection.OrderIDs = orders
	return selection, nil
}

func validateIdempotencyKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 128 {
		return "", fmt.Errorf("%w: idempotency key must contain 1..128 characters", ErrInvalidInput)
	}
	return key, nil
}

func validConnectionID(connectionID string) bool {
	connectionID = strings.TrimSpace(connectionID)
	return connectionID != "" && len(connectionID) <= 128
}

func roomSeatForSession(room *roomRecord, sessionID string) (SeatID, bool) {
	for index := range room.seats {
		if room.seats[index].sessionID == sessionID {
			return seatIDForIndex(index), true
		}
	}
	return "", false
}

func occupiedSeats(room *roomRecord) int {
	count := 0
	for index := range room.seats {
		if room.seats[index].sessionID != "" {
			count++
		}
	}
	return count
}

func seatIDForIndex(index int) SeatID {
	if index == 1 {
		return SeatTwo
	}
	return SeatOne
}

func seatIndex(seat SeatID) int {
	switch seat {
	case SeatOne:
		return 0
	case SeatTwo:
		return 1
	default:
		return -1
	}
}

func selectionsEqual(left, right SeatSelection) bool {
	if left.DeckID != right.DeckID || len(left.OrderIDs) != len(right.OrderIDs) {
		return false
	}
	for index := range left.OrderIDs {
		if left.OrderIDs[index] != right.OrderIDs[index] {
			return false
		}
	}
	return true
}

func cloneSelection(selection SeatSelection) SeatSelection {
	selection.OrderIDs = append([]string(nil), selection.OrderIDs...)
	return selection
}

func cloneStartedMatch(match StartedMatch) StartedMatch { return match }

func cloneRoom(room *roomRecord) *roomRecord {
	clone := *room
	for index := range clone.seats {
		clone.seats[index].selection = cloneSelection(room.seats[index].selection)
	}
	return &clone
}
