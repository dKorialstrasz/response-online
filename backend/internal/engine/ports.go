// Package engine contains deterministic game-state transitions.
//
// Issue #8 defines only dependency ports. Game rules and card behavior are
// implemented by their dedicated roadmap issues after verification.
package engine

import (
	"crypto/rand"
	"time"
)

type Clock interface {
	Now() time.Time
}

type Entropy interface {
	Read([]byte) (int, error)
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}

type CryptoEntropy struct{}

func (CryptoEntropy) Read(target []byte) (int, error) {
	return rand.Read(target)
}
