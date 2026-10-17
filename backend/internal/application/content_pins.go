package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dKorialstrasz/response-online/backend/internal/engine"
)

type releaseDocument struct {
	Versions     engine.Versions   `json:"versions"`
	Artifacts    []releaseArtifact `json:"artifacts"`
	MatchPinning struct {
		Required bool `json:"required"`
	} `json:"matchPinning"`
}

type releaseArtifact struct {
	Path            string `json:"path"`
	CanonicalSHA256 string `json:"canonicalSha256"`
}

type deckManifestDocument struct {
	ContentVersion string         `json:"contentVersion"`
	ReviewStatus   string         `json:"reviewStatus"`
	DeckID         string         `json:"deckId"`
	Hero           manifestCard   `json:"hero"`
	Orders         []manifestCard `json:"orders"`
	Main           []manifestCard `json:"main"`
}

type manifestCard struct {
	DefinitionID string `json:"definitionId"`
	Count        int    `json:"count"`
}

type cardDefinitionsDocument struct {
	ContentVersion string                   `json:"contentVersion"`
	ReviewStatus   string                   `json:"reviewStatus"`
	Definitions    []cardDefinitionDocument `json:"definitions"`
}

type cardDefinitionDocument struct {
	CardID string `json:"cardId"`
	Stats  struct {
		Influence verifiedInteger `json:"influence"`
	} `json:"stats"`
}

type verifiedInteger struct {
	Kind  string `json:"kind"`
	Value int    `json:"value"`
}

// FileMatchPinProvider loads one validated release snapshot at startup. It
// verifies every consumed artifact against the release digest before exposing
// immutable deck inputs to RoomService.
type FileMatchPinProvider struct {
	versions engine.Versions
	decks    map[string]PinnedDeck
}

func NewFileMatchPinProvider(repositoryRoot, releaseRelativePath string) (*FileMatchPinProvider, error) {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	releasePath, err := safeRepositoryPath(root, releaseRelativePath)
	if err != nil {
		return nil, err
	}
	releaseBytes, err := os.ReadFile(releasePath)
	if err != nil {
		return nil, fmt.Errorf("read content release: %w", err)
	}
	var release releaseDocument
	if err := json.Unmarshal(releaseBytes, &release); err != nil {
		return nil, fmt.Errorf("decode content release: %w", err)
	}
	if !release.MatchPinning.Required {
		return nil, fmt.Errorf("content release does not require match pinning")
	}
	if err := validatePinnedVersions(release.Versions); err != nil {
		return nil, err
	}

	artifacts := make(map[string]releaseArtifact, len(release.Artifacts))
	for _, artifact := range release.Artifacts {
		if artifact.Path == "" || len(artifact.CanonicalSHA256) != sha256.Size*2 {
			return nil, fmt.Errorf("content release has an invalid artifact record")
		}
		if _, duplicate := artifacts[artifact.Path]; duplicate {
			return nil, fmt.Errorf("content release repeats artifact %q", artifact.Path)
		}
		artifacts[artifact.Path] = artifact
	}

	definitionsPath := "content/cards/base/card-definitions.json"
	definitionsBytes, err := verifiedArtifact(root, artifacts, definitionsPath)
	if err != nil {
		return nil, err
	}
	var definitions cardDefinitionsDocument
	if err := json.Unmarshal(definitionsBytes, &definitions); err != nil {
		return nil, fmt.Errorf("decode card definitions: %w", err)
	}
	if definitions.ContentVersion != release.Versions.Content || definitions.ReviewStatus != "approved" {
		return nil, fmt.Errorf("card definitions do not match the approved release")
	}
	heroInfluence := make(map[string]int)
	for _, definition := range definitions.Definitions {
		if definition.Stats.Influence.Kind == "fixed" && definition.Stats.Influence.Value > 0 {
			heroInfluence[definition.CardID] = definition.Stats.Influence.Value
		}
	}

	deckPaths := make([]string, 0, 6)
	for path := range artifacts {
		if strings.HasPrefix(path, "content/decks/base/") && strings.HasSuffix(path, ".json") {
			deckPaths = append(deckPaths, path)
		}
	}
	sort.Strings(deckPaths)
	if len(deckPaths) != 6 {
		return nil, fmt.Errorf("content release must contain exactly six fixed deck manifests")
	}
	decks := make(map[string]PinnedDeck, len(deckPaths))
	for _, path := range deckPaths {
		artifact := artifacts[path]
		manifestBytes, err := verifiedArtifact(root, artifacts, path)
		if err != nil {
			return nil, err
		}
		var manifest deckManifestDocument
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return nil, fmt.Errorf("decode deck manifest %q: %w", path, err)
		}
		if manifest.DeckID == "" || manifest.ContentVersion != release.Versions.DeckManifests ||
			manifest.ReviewStatus != "approved" || len(manifest.Orders) != engine.OrdersPerPlayer {
			return nil, fmt.Errorf("deck manifest %q does not match the approved release", path)
		}
		if _, duplicate := decks[manifest.DeckID]; duplicate {
			return nil, fmt.Errorf("duplicate deck id %q", manifest.DeckID)
		}
		influence := heroInfluence[manifest.Hero.DefinitionID]
		if influence <= 0 {
			return nil, fmt.Errorf("deck %q has no verified hero influence", manifest.DeckID)
		}
		deck := PinnedDeck{
			DeckID:        manifest.DeckID,
			ManifestID:    path + "#sha256:" + artifact.CanonicalSHA256,
			HeroID:        manifest.Hero.DefinitionID,
			BaseInfluence: influence,
		}
		for _, card := range manifest.Main {
			if card.DefinitionID == "" || card.Count <= 0 {
				return nil, fmt.Errorf("deck %q has an invalid main card", manifest.DeckID)
			}
			for range card.Count {
				deck.CardIDs = append(deck.CardIDs, card.DefinitionID)
			}
		}
		for _, order := range manifest.Orders {
			if order.DefinitionID == "" || order.Count != 1 {
				return nil, fmt.Errorf("deck %q has an invalid order", manifest.DeckID)
			}
			deck.AvailableOrderIDs = append(deck.AvailableOrderIDs, order.DefinitionID)
		}
		if len(deck.CardIDs) != 30 {
			return nil, fmt.Errorf("deck %q does not contain 30 main cards", manifest.DeckID)
		}
		decks[manifest.DeckID] = deck
	}
	return &FileMatchPinProvider{versions: release.Versions, decks: decks}, nil
}

func (provider *FileMatchPinProvider) PinMatch(
	_ context.Context,
	selections [2]SeatSelection,
) (MatchPins, error) {
	if provider == nil {
		return MatchPins{}, fmt.Errorf("content pin provider is nil")
	}
	result := MatchPins{Versions: provider.versions}
	for index, selection := range selections {
		deck, exists := provider.decks[selection.DeckID]
		if !exists {
			return MatchPins{}, fmt.Errorf("deck is not in the fixed M1 release")
		}
		if !sameStringSet(selection.OrderIDs, deck.AvailableOrderIDs) {
			return MatchPins{}, fmt.Errorf("order arrangement is not a permutation of the selected deck")
		}
		result.Decks[index] = clonePinnedDeck(deck)
	}
	return result, nil
}

func verifiedArtifact(
	repositoryRoot string,
	artifacts map[string]releaseArtifact,
	relativePath string,
) ([]byte, error) {
	artifact, exists := artifacts[relativePath]
	if !exists {
		return nil, fmt.Errorf("content release does not pin %q", relativePath)
	}
	path, err := safeRepositoryPath(repositoryRoot, relativePath)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pinned artifact %q: %w", relativePath, err)
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != artifact.CanonicalSHA256 {
		return nil, fmt.Errorf("pinned artifact %q failed SHA-256 verification", relativePath)
	}
	return content, nil
}

func safeRepositoryPath(repositoryRoot, relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("content path must be repository-relative")
	}
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("content path escapes the repository")
	}
	path := filepath.Join(repositoryRoot, clean)
	resolvedRoot, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root links: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve content path links: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("content path escapes the repository")
	}
	return resolvedPath, nil
}

func validatePinnedVersions(versions engine.Versions) error {
	values := []string{
		versions.Content, versions.DeckManifests, versions.EffectSpec,
		versions.Rules, versions.Protocol, versions.Engine,
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("content release omits a required match version")
		}
	}
	return nil
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func clonePinnedDeck(deck PinnedDeck) PinnedDeck {
	deck.CardIDs = append([]string(nil), deck.CardIDs...)
	deck.AvailableOrderIDs = append([]string(nil), deck.AvailableOrderIDs...)
	return deck
}
