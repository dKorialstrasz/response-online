from __future__ import annotations

import copy
import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "validate_content_release", ROOT / "scripts/validate_content_release.py"
)
assert SPEC and SPEC.loader
VALIDATOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VALIDATOR)


class ContentReleaseValidationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.documents = VALIDATOR.repository_documents(ROOT)

    def validate(self, documents: dict, gameplay_ready: bool = False) -> list[str]:
        return VALIDATOR.validate_documents(ROOT, documents, gameplay_ready)

    def test_repository_release_is_structurally_valid(self) -> None:
        self.assertEqual([], self.validate(copy.deepcopy(self.documents)))

    def test_schema_rejects_incomplete_card_definition(self) -> None:
        documents = copy.deepcopy(self.documents)
        del documents["content/cards/base/card-definitions.json"]["definitions"][0]["cost"]
        errors = self.validate(documents)
        self.assertTrue(any("missing required field 'cost'" in error for error in errors), errors)

    def test_deck_arithmetic_is_release_blocking(self) -> None:
        documents = copy.deepcopy(self.documents)
        documents["content/decks/base/technology-dari.json"]["main"][0]["count"] = 1
        errors = self.validate(documents)
        self.assertTrue(any("expected deck arithmetic 30/5/1" in error for error in errors), errors)

    def test_hash_mismatch_is_release_blocking(self) -> None:
        documents = copy.deepcopy(self.documents)
        documents["content/evidence/base-gallery-evidence.json"]["scope"] += " Changed."
        errors = self.validate(documents)
        self.assertTrue(any("canonical SHA-256 mismatch" in error for error in errors), errors)

    def test_schema_binding_is_release_blocking(self) -> None:
        documents = copy.deepcopy(self.documents)
        documents["content/releases/m1-local.json"]["artifacts"][0]["schema"] = (
            "content/schemas/deck-manifest.schema.json"
        )
        errors = self.validate(documents)
        self.assertTrue(any("schema binding mismatch" in error for error in errors), errors)

    def test_unknown_rights_cannot_be_promoted(self) -> None:
        documents = copy.deepcopy(self.documents)
        documents["content/licenses/registry.json"]["cardImages"][0]["rightsStatus"] = "allowed"
        errors = self.validate(documents)
        self.assertTrue(any("rights must remain unknown" in error for error in errors), errors)

    def test_core_engine_release_is_still_blocked_by_effects_only(self) -> None:
        errors = self.validate(copy.deepcopy(self.documents), gameplay_ready=True)
        self.assertTrue(any("126 effect specs are not implemented" in error for error in errors), errors)
        self.assertFalse(any("protocol version is not release-ready" in error for error in errors), errors)
        self.assertFalse(any("rules version is not release-ready" in error for error in errors), errors)
        self.assertFalse(any("engine version is not release-ready" in error for error in errors), errors)


if __name__ == "__main__":
    unittest.main()
