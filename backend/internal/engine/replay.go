package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func StateHash(state State) (string, error) {
	canonical, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func Replay(config Config, commands []Command) (State, string, error) {
	match, err := NewMatch(config)
	if err != nil {
		return State{}, "", err
	}
	for index, command := range commands {
		if err := match.Handle(command); err != nil {
			return State{}, "", fmt.Errorf("replay command %d (%s): %w", index, command.ID, err)
		}
	}
	state := match.State()
	hash, err := StateHash(state)
	if err != nil {
		return State{}, "", err
	}
	return state, hash, nil
}
