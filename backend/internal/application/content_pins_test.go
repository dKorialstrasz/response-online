package application

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileMatchPinProviderLoadsApprovedReleaseAndAllowsMirrors(t *testing.T) {
	provider, err := NewFileMatchPinProvider(repositoryRoot(t), "content/releases/m1-local.json")
	if err != nil {
		t.Fatalf("load content pins: %v", err)
	}
	selection := SeatSelection{
		DeckID:   "order-bjorn",
		OrderIDs: []string{"baz-132", "baz-128", "baz-130", "baz-129", "baz-131"},
	}
	pins, err := provider.PinMatch(context.Background(), [2]SeatSelection{selection, selection})
	if err != nil {
		t.Fatalf("pin mirror match: %v", err)
	}
	if pins.Versions.Content != "base-cards-v1" || pins.Versions.Engine != "deterministic-core-v1" {
		t.Fatalf("unexpected pinned versions: %#v", pins.Versions)
	}
	for index, deck := range pins.Decks {
		if deck.DeckID != selection.DeckID || deck.HeroID != "baz-111" || deck.BaseInfluence != 5 {
			t.Fatalf("seat %d pinned deck = %#v", index, deck)
		}
		if len(deck.CardIDs) != 30 || len(deck.AvailableOrderIDs) != 5 {
			t.Fatalf("seat %d pinned counts = %d/%d", index, len(deck.CardIDs), len(deck.AvailableOrderIDs))
		}
		if deck.ManifestID != "content/decks/base/order-bjorn.json#sha256:947c5557ad928e5fc40b72f6d29d9fb34df34998395002fb3dce7401fe1f8dad" {
			t.Fatalf("seat %d manifest pin = %q", index, deck.ManifestID)
		}
	}
	// Returned slices must not mutate the immutable startup snapshot.
	pins.Decks[0].CardIDs[0] = "tampered"
	reloaded, err := provider.PinMatch(context.Background(), [2]SeatSelection{selection, selection})
	if err != nil || reloaded.Decks[0].CardIDs[0] == "tampered" {
		t.Fatalf("provider snapshot was mutable: %#v, %v", reloaded.Decks[0].CardIDs, err)
	}
}

func TestFileMatchPinProviderRejectsUnknownDeckAndOrder(t *testing.T) {
	provider, err := NewFileMatchPinProvider(repositoryRoot(t), "content/releases/m1-local.json")
	if err != nil {
		t.Fatalf("load content pins: %v", err)
	}
	valid := SeatSelection{
		DeckID:   "technology-dari",
		OrderIDs: []string{"baz-018", "baz-019", "baz-020", "baz-021", "baz-022"},
	}
	unknown := valid
	unknown.DeckID = "custom-deck"
	if _, err := provider.PinMatch(context.Background(), [2]SeatSelection{valid, unknown}); err == nil {
		t.Fatal("unknown deck was accepted")
	}
	invalidOrder := valid
	invalidOrder.OrderIDs = append([]string(nil), valid.OrderIDs...)
	invalidOrder.OrderIDs[0] = "baz-999"
	if _, err := provider.PinMatch(context.Background(), [2]SeatSelection{valid, invalidOrder}); err == nil {
		t.Fatal("order outside the selected manifest was accepted")
	}
}

func TestFileMatchPinProviderRejectsEscapingPath(t *testing.T) {
	if _, err := NewFileMatchPinProvider(repositoryRoot(t), "../outside.json"); err == nil {
		t.Fatal("repository-escaping release path was accepted")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
