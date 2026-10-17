package engine

import (
	"encoding/binary"
	"errors"
)

func SeedFromEntropy(entropy Entropy) (uint64, error) {
	if entropy == nil {
		return 0, errors.New("entropy is required")
	}
	var raw [8]byte
	n, err := entropy.Read(raw[:])
	if err != nil {
		return 0, err
	}
	if n != len(raw) {
		return 0, errors.New("entropy returned a short seed")
	}
	seed := binary.LittleEndian.Uint64(raw[:])
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return seed, nil
}

func newRandom(seed uint64) RandomState {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return RandomState{Seed: seed, State: seed}
}

func (random *RandomState) next() uint64 {
	// xorshift64* is small, deterministic, and fully captured by RandomState.
	value := random.State
	value ^= value >> 12
	value ^= value << 25
	value ^= value >> 27
	random.State = value
	random.Cursor++
	return value * 2685821657736338717
}

func (random *RandomState) intn(limit int) int {
	if limit <= 1 {
		return 0
	}
	bound := uint64(limit)
	cutoff := ^uint64(0) - (^uint64(0) % bound)
	for {
		value := random.next()
		if value < cutoff {
			return int(value % bound)
		}
	}
}
