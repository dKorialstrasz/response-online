// Package application coordinates rooms and matches around the pure engine.
package application

import (
	"context"
	"errors"
)

var ErrOwnerStopped = errors.New("match owner stopped")

type Operation func(context.Context) error

type request struct {
	operation Operation
	result    chan error
}

// MatchOwner serializes accepted work for one match. It intentionally knows
// nothing about card rules, transport payloads, or persistence.
type MatchOwner struct {
	mailbox chan request
}

func NewMatchOwner(mailboxCapacity int) *MatchOwner {
	if mailboxCapacity < 1 {
		mailboxCapacity = 1
	}
	return &MatchOwner{mailbox: make(chan request, mailboxCapacity)}
}

func (owner *MatchOwner) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case request := <-owner.mailbox:
			request.result <- request.operation(ctx)
		}
	}
}

func (owner *MatchOwner) Submit(ctx context.Context, operation Operation) error {
	if operation == nil {
		return errors.New("nil match operation")
	}

	request := request{operation: operation, result: make(chan error, 1)}
	select {
	case <-ctx.Done():
		return ErrOwnerStopped
	case owner.mailbox <- request:
	}

	select {
	case <-ctx.Done():
		return ErrOwnerStopped
	case err := <-request.result:
		return err
	}
}
