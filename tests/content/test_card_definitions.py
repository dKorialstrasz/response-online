from __future__ import annotations

import copy
import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "validate_card_definitions", ROOT / "scripts/validate_card_definitions.py"
)
assert SPEC and SPEC.loader
VALIDATOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VALIDATOR)


class CardDefinitionValidationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.payloads = VALIDATOR.load_payloads(ROOT)

    def test_repository_artifacts_are_consistent(self) -> None:
        self.assertEqual([], VALIDATOR.validate_payloads(copy.deepcopy(self.payloads)))

    def test_owner_review_is_an_explicit_acceptance_gate(self) -> None:
        self.assertEqual(
            [],
            VALIDATOR.validate_payloads(
                copy.deepcopy(self.payloads), require_owner_review=True
            ),
        )

        payloads = copy.deepcopy(self.payloads)
        payloads["evidence"]["ownerVerification"] = {
            "status": "pending",
            "reviewer": "dKorialstrasz",
            "reviewedAt": None,
            "decision": "pending",
            "decisionSource": None,
        }
        payloads["definitions"]["reviewStatus"] = "pending-owner-review"
        for card in payloads["definitions"]["definitions"]:
            card["verificationStatus"] = "source-matched-pending-owner-review"
            for evidence in card["evidence"]:
                evidence["ownerReviewStatus"] = "pending"
        errors = VALIDATOR.validate_payloads(payloads, require_owner_review=True)
        self.assertIn("owner verification is still pending", errors)

    def test_missing_ability_scenario_is_rejected(self) -> None:
        payloads = copy.deepcopy(self.payloads)
        payloads["scenarios"]["scenarios"].pop()
        errors = VALIDATOR.validate_payloads(payloads)
        self.assertTrue(any("unknown scenario" in error for error in errors), errors)

    def test_unverified_numeric_zero_is_rejected(self) -> None:
        payloads = copy.deepcopy(self.payloads)
        payloads["definitions"]["definitions"][1]["cost"] = {
            "kind": "fixed",
            "value": 0,
            "verificationStatus": "unknown",
        }
        errors = VALIDATOR.validate_payloads(payloads)
        self.assertTrue(any("fixed value must be source-matched" in error for error in errors), errors)

    def test_missing_transitive_definition_is_rejected(self) -> None:
        payloads = copy.deepcopy(self.payloads)
        payloads["definitions"]["definitions"].pop()
        errors = VALIDATOR.validate_payloads(payloads)
        self.assertTrue(any("expected 132" in error for error in errors), errors)

    def test_unknown_image_rights_must_not_be_promoted(self) -> None:
        payloads = copy.deepcopy(self.payloads)
        payloads["licenses"]["cardImages"][0]["rightsStatus"] = "allowed"
        errors = VALIDATOR.validate_payloads(payloads)
        self.assertTrue(any("rights must remain unknown" in error for error in errors), errors)


if __name__ == "__main__":
    unittest.main()
