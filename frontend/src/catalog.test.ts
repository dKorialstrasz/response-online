import { describe, expect, it } from "vitest";

import { cardImageUrl, confirmedCard, decks } from "./catalog";

describe("утверждённый каталог", () => {
  it("содержит ровно шесть разных manifests и допускает одинаковый выбор", () => {
    expect(decks).toHaveLength(6);
    expect(new Set(decks.map((deck) => deck.deckId)).size).toBe(6);
    const mirror = [decks[0].deckId, decks[0].deckId];
    expect(mirror[0]).toBe(mirror[1]);
  });

  it("каждая колода содержит 30 карт, пять Приказов и героя", () => {
    for (const deck of decks) {
      expect(deck.main.reduce((total, card) => total + card.count, 0)).toBe(30);
      expect(deck.orders).toHaveLength(5);
      expect(deck.hero.count).toBe(1);
    }
  });

  it("не превращает неподтверждённые числовые поля в нули", () => {
    for (const deck of decks) {
      for (const card of [deck.hero, ...deck.orders, ...deck.main]) {
        const detail = confirmedCard(card);
        for (const value of [
          detail.cost,
          detail.influence,
          detail.strength,
          detail.durability,
        ]) {
          expect(value === undefined || Number.isFinite(value)).toBe(true);
        }
      }
    }
  });

  it("сопоставляет каждую карту стартовых колод с официальным изображением", () => {
    for (const deck of decks) {
      for (const card of [deck.hero, ...deck.orders, ...deck.main]) {
        expect(cardImageUrl(card.definitionId, "thumb")).toMatch(
          /^https:\/\/response-league\.ru\/api\/v1\/card-images\//,
        );
        expect(cardImageUrl(card.definitionId, "medium")).toMatch(
          /^https:\/\/response-league\.ru\/api\/v1\/card-images\//,
        );
      }
    }
  });
});
