package application

import (
	"context"
	"errors"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

// MatchSession is the only application-level writer for one deterministic
// match. Transport handlers submit commands here instead of mutating engine
// state directly.
type MatchSession struct {
	owner *MatchOwner
	match *engine.Match
}

func NewMatchSession(match *engine.Match, mailboxCapacity int) (*MatchSession, error) {
	if match == nil {
		return nil, errors.New("match is required")
	}
	return &MatchSession{owner: NewMatchOwner(mailboxCapacity), match: match}, nil
}

func (session *MatchSession) Run(ctx context.Context) error {
	return session.owner.Run(ctx)
}

func (session *MatchSession) Submit(ctx context.Context, command engine.Command) error {
	return session.owner.Submit(ctx, func(context.Context) error {
		return session.match.Handle(command)
	})
}

func (session *MatchSession) Snapshot(ctx context.Context) (engine.State, error) {
	var snapshot engine.State
	err := session.owner.Submit(ctx, func(context.Context) error {
		snapshot = session.match.State()
		return nil
	})
	return snapshot, err
}
