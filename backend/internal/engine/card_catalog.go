package engine

import "fmt"

func buildCardCatalog(
	definitions []CardDefinition,
	abilities map[string]AbilityDefinition,
) (map[string]CardDefinition, error) {
	catalog := make(map[string]CardDefinition, len(definitions))
	for _, definition := range definitions {
		if definition.CardID == "" || definition.Name == "" || len(definition.Types) == 0 {
			return nil, fmt.Errorf("card definition requires id, name, and type")
		}
		if _, duplicate := catalog[definition.CardID]; duplicate {
			return nil, fmt.Errorf("card definition %q is duplicated", definition.CardID)
		}
		for _, cardType := range definition.Types {
			if !validCardType(cardType) {
				return nil, fmt.Errorf("card definition %q has invalid type %q", definition.CardID, cardType)
			}
		}
		if definition.Cost != nil && *definition.Cost < 0 {
			return nil, fmt.Errorf("card definition %q has negative cost", definition.CardID)
		}
		if definition.BasePower != nil && *definition.BasePower < 0 {
			return nil, fmt.Errorf("card definition %q has negative base power", definition.CardID)
		}
		seenAbilities := make(map[string]struct{}, len(definition.AbilityIDs))
		for _, abilityID := range definition.AbilityIDs {
			if _, exists := abilities[abilityID]; !exists {
				return nil, fmt.Errorf("card definition %q references unknown ability %q", definition.CardID, abilityID)
			}
			if _, duplicate := seenAbilities[abilityID]; duplicate {
				return nil, fmt.Errorf("card definition %q repeats ability %q", definition.CardID, abilityID)
			}
			seenAbilities[abilityID] = struct{}{}
		}
		clone := definition
		clone.Types = append([]CardType(nil), definition.Types...)
		clone.AbilityIDs = append([]string(nil), definition.AbilityIDs...)
		clone.NativeKeywords = append([]Keyword(nil), definition.NativeKeywords...)
		if clone.BaseMagicShieldCost < 0 {
			return nil, fmt.Errorf("card definition %q has negative Magic Shield cost", definition.CardID)
		}
		seenKeywords := make(map[Keyword]struct{}, len(clone.NativeKeywords))
		for _, keyword := range clone.NativeKeywords {
			if !validKeyword(keyword) {
				return nil, fmt.Errorf("card definition %q has invalid native keyword %q", definition.CardID, keyword)
			}
			if _, duplicate := seenKeywords[keyword]; duplicate {
				return nil, fmt.Errorf("card definition %q repeats native keyword %q", definition.CardID, keyword)
			}
			seenKeywords[keyword] = struct{}{}
		}
		if definition.Cost != nil {
			value := *definition.Cost
			clone.Cost = &value
		}
		if definition.BasePower != nil {
			value := *definition.BasePower
			clone.BasePower = &value
		}
		catalog[definition.CardID] = clone
	}
	return catalog, nil
}

func validKeyword(keyword Keyword) bool {
	return keyword == KeywordRider || keyword == KeywordBypass || keyword == KeywordPoison ||
		keyword == KeywordCharge || keyword == KeywordRangedAttack || keyword == KeywordResilience
}

func validCardType(cardType CardType) bool {
	return cardType == CardHero || cardType == CardOrder || cardType == CardUnit ||
		cardType == CardSpell || cardType == CardEnhancement
}

func cardHasType(definition CardDefinition, expected CardType) bool {
	for _, cardType := range definition.Types {
		if cardType == expected {
			return true
		}
	}
	return false
}
