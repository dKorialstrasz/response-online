import cardDefinitions from "../../content/cards/base/card-definitions.json";
import chaos from "../../content/decks/base/chaos-enitokl.json";
import death from "../../content/decks/base/death-kassiy.json";
import life from "../../content/decks/base/life-katarina.json";
import nature from "../../content/decks/base/nature-medana.json";
import order from "../../content/decks/base/order-bjorn.json";
import technology from "../../content/decks/base/technology-dari.json";

export type CatalogCard = {
  definitionId: string;
  name: string;
  category: string;
  count: number;
};

export type ConfirmedCard = CatalogCard & {
  types: string[];
  subtypes: string[];
  faction: string;
  rarity: string;
  cost?: number;
  influence?: number;
  strength?: number;
  durability?: number;
  abilityText: string[];
};

export type Deck = {
  deckId: string;
  displayName: string;
  factionName: string;
  hero: CatalogCard;
  orders: CatalogCard[];
  main: CatalogCard[];
};

type RawManifest = {
  reviewStatus: string;
  deckId: string;
  displayName: string;
  factionName: string;
  hero: CatalogCard;
  orders: CatalogCard[];
  main: CatalogCard[];
};

type ConfirmedNumber = {
  kind: string;
  value?: number;
  verificationStatus: string;
};

type RawDefinition = {
  cardId: string;
  name: string;
  verificationStatus: string;
  typeLine: { types: string[]; subtypes: string[] };
  factions: string[];
  rarity: string;
  cost: ConfirmedNumber;
  stats: {
    influence: ConfirmedNumber;
    strength: ConfirmedNumber;
    durability: ConfirmedNumber;
  };
  faces: Array<{ exactAbilityText?: string }>;
};

const approvedManifests = [
  technology,
  death,
  chaos,
  nature,
  life,
  order,
] as RawManifest[];

if (
  approvedManifests.length !== 6 ||
  approvedManifests.some((manifest) => manifest.reviewStatus !== "approved")
) {
  throw new Error("Каталог стартовых колод не прошёл проверку");
}

export const decks: Deck[] = approvedManifests.map((manifest) => ({
  deckId: manifest.deckId,
  displayName: manifest.displayName,
  factionName: manifest.factionName,
  hero: manifest.hero,
  orders: manifest.orders,
  main: manifest.main,
}));

const definitions = new Map(
  (cardDefinitions.definitions as RawDefinition[])
    .filter(
      (definition) =>
        definition.verificationStatus === "source-matched-owner-approved",
    )
    .map((definition) => [definition.cardId, definition]),
);

function fixedValue(field: ConfirmedNumber): number | undefined {
  return field.kind === "fixed" &&
    field.verificationStatus === "source-matched" &&
    typeof field.value === "number"
    ? field.value
    : undefined;
}

export function findDeck(deckId: string): Deck | undefined {
  return decks.find((deck) => deck.deckId === deckId);
}

export function confirmedCard(card: CatalogCard): ConfirmedCard {
  const definition = definitions.get(card.definitionId);
  if (!definition) {
    return {
      ...card,
      types: [],
      subtypes: [],
      faction: "",
      rarity: "",
      abilityText: [],
    };
  }
  return {
    ...card,
    name: definition.name,
    types: definition.typeLine.types,
    subtypes: definition.typeLine.subtypes,
    faction: definition.factions.join(", "),
    rarity: definition.rarity,
    cost: fixedValue(definition.cost),
    influence: fixedValue(definition.stats.influence),
    strength: fixedValue(definition.stats.strength),
    durability: fixedValue(definition.stats.durability),
    abilityText: definition.faces
      .map((face) => face.exactAbilityText)
      .filter((text): text is string => Boolean(text)),
  };
}

export function cardName(definitionId: string): string {
  return definitions.get(definitionId)?.name ?? "Неизвестная карта";
}
