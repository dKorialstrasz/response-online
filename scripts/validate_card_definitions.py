#!/usr/bin/env python3
"""Validate M1 CardDefinition evidence, ability links, and transitive closure."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


CREATED_OBJECT_MARKERS = ("создайте", "создаёт", "создает", "созданн", "жетон")
REQUIRED_EVIDENCE_PATHS = {
    "cardId",
    "sourceCardId",
    "setCode",
    "setName",
    "printedNumber",
    "locale",
    "name",
    "typeLine",
    "factions",
    "rarity",
    "keywords",
    "cost",
    "stats",
    "faces",
    "clarifications",
    "transitiveRefs",
    "imageAssetIds",
}


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def canonical_json(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8")


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256_text(value: str) -> str:
    return sha256_bytes(value.encode("utf-8"))


def load_payloads(root: Path) -> dict[str, Any]:
    paths = {
        "snapshot": "content/evidence/base-gallery-snapshot.json",
        "evidence": "content/evidence/base-gallery-evidence.json",
        "definitions": "content/cards/base/card-definitions.json",
        "abilities": "content/abilities/base-abilities.json",
        "scenarios": "content/conformance/base-ability-scenarios.json",
        "licenses": "content/licenses/registry.json",
        "closure": "content/closure/base-content-closure.json",
        "nameDiscrepancies": "content/reviews/base-name-discrepancies.json",
    }
    result = {name: read_json(root / relative) for name, relative in paths.items()}
    result["deckManifests"] = [
        read_json(path) for path in sorted((root / "content/decks/base").glob("*.json"))
    ]
    return result


def add(errors: list[str], condition: bool, message: str) -> None:
    if not condition:
        errors.append(message)


def validate_value_expression(value: Any, path: str, errors: list[str]) -> None:
    if not isinstance(value, dict):
        errors.append(f"{path}: expected value expression")
        return
    kind = value.get("kind")
    add(errors, kind in {"fixed", "notApplicable", "unknown"}, f"{path}.kind: invalid")
    status = value.get("verificationStatus")
    if kind == "fixed":
        number = value.get("value")
        add(errors, isinstance(number, int) and number >= 0, f"{path}.value: expected non-negative int")
        add(errors, status == "source-matched", f"{path}: fixed value must be source-matched")
    elif kind == "unknown":
        add(errors, value.get("value") is None, f"{path}: unknown must use null value")
        add(errors, status == "unknown", f"{path}: unknown status required")
    elif kind == "notApplicable":
        add(errors, "value" not in value, f"{path}: notApplicable must not carry a value")
        add(errors, status == "source-matched", f"{path}: absence must be source-matched")


def source_printing(source: dict[str, Any], card_id: str) -> dict[str, Any] | None:
    for printing in source.get("printings") or []:
        if printing.get("id") == card_id:
            return printing
    return None


def validate_payloads(payloads: dict[str, Any], require_owner_review: bool = False) -> list[str]:
    errors: list[str] = []
    snapshot = payloads["snapshot"]
    evidence = payloads["evidence"]
    definitions_doc = payloads["definitions"]
    abilities_doc = payloads["abilities"]
    scenarios_doc = payloads["scenarios"]
    licenses = payloads["licenses"]
    closure = payloads["closure"]
    discrepancies = payloads["nameDiscrepancies"]

    snapshot_hash = sha256_bytes(canonical_json(snapshot))
    add(errors, evidence.get("snapshotSha256") == snapshot_hash, "evidence snapshot SHA-256 mismatch")
    add(
        errors,
        snapshot.get("source", {}).get("catalogVersion") == evidence.get("source", {}).get("catalogVersion"),
        "catalog version mismatch",
    )
    add(errors, evidence.get("captureMethod") == "official-json-api-no-ocr", "OCR/manual capture is not allowed")

    owner_status = evidence.get("ownerVerification", {}).get("status")
    add(errors, owner_status in {"pending", "approved"}, "invalid owner verification status")
    if owner_status == "approved":
        add(errors, bool(evidence["ownerVerification"].get("reviewer")), "approved owner review needs reviewer")
        add(errors, bool(evidence["ownerVerification"].get("reviewedAt")), "approved owner review needs date")
        add(errors, definitions_doc.get("reviewStatus") == "approved", "definition review status must be approved")
    if require_owner_review:
        add(errors, owner_status == "approved", "owner verification is still pending")

    source_cards = {card["id"]: card for card in snapshot.get("cards") or []}
    definitions = definitions_doc.get("definitions") or []
    by_card_id = {card.get("cardId"): card for card in definitions}
    expected_ids = [f"baz-{number:03d}" for number in range(1, 133)]
    add(errors, len(definitions) == 132, f"definitions: expected 132, got {len(definitions)}")
    add(errors, len(by_card_id) == len(definitions), "definitions: duplicate cardId")
    add(errors, sorted(by_card_id) == expected_ids, "definitions: expected exact baz-001..baz-132 range")

    image_assets = {asset["assetId"]: asset for asset in licenses.get("cardImages") or []}
    add(errors, licenses.get("publicReleaseGate") == "blocked-rights-unknown", "public release must remain blocked")

    face_text_by_ability: dict[str, tuple[str, str, str]] = {}
    all_transitive_refs: set[str] = set()
    for card in definitions:
        card_id = card.get("cardId", "<missing>")
        prefix = f"definitions[{card_id}]"
        source = source_cards.get(card.get("sourceCardId"))
        add(errors, source is not None, f"{prefix}.sourceCardId: missing source card")
        if source is None:
            continue
        printing = source_printing(source, card_id)
        add(errors, printing is not None, f"{prefix}: source printing missing")

        add(errors, card.get("setCode") == "БАЗ", f"{prefix}.setCode: unexpected")
        add(errors, card.get("setName") == "Базовый", f"{prefix}.setName: unexpected")
        add(errors, card.get("locale") == "ru-RU", f"{prefix}.locale: unexpected")
        add(errors, card.get("name") == source.get("name"), f"{prefix}.name: not exact gallery spelling")
        add(errors, card.get("factions") == (source.get("factions") or []), f"{prefix}.factions: source mismatch")
        add(
            errors,
            card.get("typeLine")
            == {
                "supertypes": source.get("supertypes") or [],
                "types": source.get("types") or [],
                "subtypes": source.get("subtypes") or [],
            },
            f"{prefix}.typeLine: source mismatch",
        )

        validate_value_expression(card.get("cost"), f"{prefix}.cost", errors)
        stats = card.get("stats") or {}
        for stat in ("influence", "strength", "durability"):
            validate_value_expression(stats.get(stat), f"{prefix}.stats.{stat}", errors)

        field_paths = {
            field_path
            for item in card.get("evidence") or []
            for field_path in item.get("fieldPaths") or []
        }
        add(
            errors,
            REQUIRED_EVIDENCE_PATHS <= field_paths,
            f"{prefix}.evidence: missing field paths {sorted(REQUIRED_EVIDENCE_PATHS - field_paths)}",
        )
        for index, item in enumerate(card.get("evidence") or []):
            evidence_prefix = f"{prefix}.evidence[{index}]"
            add(errors, item.get("evidenceId") == evidence.get("evidenceId"), f"{evidence_prefix}: evidenceId mismatch")
            add(errors, item.get("snapshotSha256") == snapshot_hash, f"{evidence_prefix}: snapshot hash mismatch")
            add(errors, item.get("verificationStatus") == "source-matched", f"{evidence_prefix}: source not matched")
            add(errors, item.get("transferMethod") == "official-json-api-no-ocr", f"{evidence_prefix}: invalid transfer method")
            add(
                errors,
                item.get("ownerReviewStatus") == owner_status,
                f"{evidence_prefix}: owner review status mismatch",
            )

        expected_verification = (
            "source-matched-owner-approved"
            if owner_status == "approved"
            else "source-matched-pending-owner-review"
        )
        add(
            errors,
            card.get("verificationStatus") == expected_verification,
            f"{prefix}.verificationStatus: owner review mismatch",
        )

        faces = card.get("faces") or []
        add(errors, bool(faces), f"{prefix}.faces: at least one face required")
        for face in faces:
            face_id = face.get("faceId", "<missing>")
            exact_text = face.get("exactAbilityText")
            add(errors, isinstance(exact_text, str), f"{prefix}.faces[{face_id}].exactAbilityText: string required")
            if not isinstance(exact_text, str):
                continue
            ability_ids = face.get("abilityIds") or []
            add(errors, bool(exact_text) == bool(ability_ids), f"{prefix}.faces[{face_id}]: text/ability link mismatch")
            for ability_id in ability_ids:
                add(errors, ability_id not in face_text_by_ability, f"duplicate abilityId: {ability_id}")
                face_text_by_ability[ability_id] = (card_id, face_id, exact_text)

        expected_clarifications = [item["body"] for item in source.get("rulings") or []]
        actual_clarifications = [item.get("text") for item in card.get("clarifications") or []]
        add(errors, actual_clarifications == expected_clarifications, f"{prefix}.clarifications: source mismatch")

        for asset_id in card.get("imageAssetIds") or []:
            asset = image_assets.get(asset_id)
            add(errors, asset is not None, f"{prefix}.imageAssetIds: missing {asset_id}")
            if asset:
                add(errors, asset.get("rightsStatus") == "unknown", f"{asset_id}: rights must remain unknown")
                add(errors, asset.get("bundled") is False, f"{asset_id}: official image must not be bundled")

        text = "\n".join(face.get("exactAbilityText", "") for face in faces).lower()
        marker_hits = [marker for marker in CREATED_OBJECT_MARKERS if marker in text]
        refs = card.get("transitiveRefs") or []
        all_transitive_refs.update(refs)
        add(errors, not marker_hits or bool(refs), f"{prefix}: created-object marker without transitive reference")

    abilities = {ability["abilityId"]: ability for ability in abilities_doc.get("abilities") or []}
    scenarios = {scenario["scenarioId"]: scenario for scenario in scenarios_doc.get("scenarios") or []}
    add(errors, len(abilities) == len(abilities_doc.get("abilities") or []), "abilities: duplicate abilityId")
    add(errors, set(abilities) == set(face_text_by_ability), "abilities: registry and CardDefinition links differ")

    for ability_id, ability in abilities.items():
        prefix = f"abilities[{ability_id}]"
        card_id, face_id, exact_text = face_text_by_ability.get(ability_id, (None, None, ""))
        add(errors, ability.get("cardId") == card_id, f"{prefix}.cardId: mismatch")
        add(errors, ability.get("faceId") == face_id, f"{prefix}.faceId: mismatch")
        add(errors, ability.get("exactTextSha256") == sha256_text(exact_text), f"{prefix}: exact text hash mismatch")
        add(errors, ability.get("source", {}).get("evidenceId") == evidence.get("evidenceId"), f"{prefix}: source missing")
        effect_spec = ability.get("effectSpec") or {}
        add(errors, effect_spec.get("status") == "implemented", f"{prefix}: EffectSpec is not implemented")
        add(errors, effect_spec.get("rulesVersion") == "response-rules-2026-10-17", f"{prefix}: rules pin mismatch")
        operations = effect_spec.get("operations") or []
        add(errors, len(operations) == 1, f"{prefix}: expected exactly one closed handler")
        if len(operations) == 1:
            add(errors, operations[0].get("kind") == "closed-handler", f"{prefix}: operation is not closed")
            add(errors, operations[0].get("handler") == ability_id, f"{prefix}: handler/ability mismatch")
        ambiguity = ability.get("ambiguityReview") or {}
        add(errors, ambiguity.get("status") in {"none-recorded", "resolved", "blocked"}, f"{prefix}: ambiguity status missing")
        if ambiguity.get("identifiedAmbiguities"):
            add(errors, bool(ambiguity.get("ownerDecisionRefs")), f"{prefix}: ambiguity needs owner decision")
        scenario_ids = ability.get("conformanceScenarioIds") or []
        add(errors, bool(scenario_ids), f"{prefix}: conformance scenario missing")
        for scenario_id in scenario_ids:
            scenario = scenarios.get(scenario_id)
            add(errors, scenario is not None, f"{prefix}: unknown scenario {scenario_id}")
            if scenario:
                add(errors, scenario.get("abilityId") == ability_id, f"{scenario_id}: ability link mismatch")
                add(errors, scenario.get("sourceTextSha256") == sha256_text(exact_text), f"{scenario_id}: source hash mismatch")
                add(errors, len(scenario.get("requiredAssertions") or []) >= 4, f"{scenario_id}: incomplete assertions")
                add(errors, scenario.get("gameplayStatus") == "implemented", f"{scenario_id}: gameplay is not implemented")
                cases = scenario.get("cases") or []
                add(errors, {case.get("kind") for case in cases} == {"positive", "negative", "interaction"}, f"{scenario_id}: positive/negative/interaction cases required")
                add(errors, len({case.get("caseId") for case in cases}) == 3, f"{scenario_id}: case ids must be unique")

    deck_ids: set[str] = set()
    deck_names = {deck["deckId"] for deck in payloads.get("deckManifests") or []}
    for deck in payloads.get("deckManifests") or []:
        refs = [deck["hero"], *deck["orders"], *deck["main"]]
        deck_ids.update(item["definitionId"] for item in refs)
    add(errors, deck_ids == set(expected_ids), "deck manifests must close over exactly baz-001..baz-132")
    add(errors, not (deck_ids - set(by_card_id)), "deck manifests contain unresolved definitions")
    add(errors, set(closure.get("resolvedDefinitionIds") or []) == deck_ids, "closure report resolved IDs mismatch")
    add(errors, not closure.get("unresolvedDefinitionIds"), "closure report has unresolved definitions")
    add(errors, not closure.get("unreachableDefinitionIds"), "closure report has unreachable definitions")
    add(errors, set(closure.get("transitiveObjectIds") or []) == all_transitive_refs, "closure transitive refs mismatch")
    add(errors, closure.get("status") == "closed", "closure report is not closed")

    matchups = scenarios_doc.get("matchups") or []
    expected_matchups = {(left, right) for left in deck_names for right in deck_names}
    actual_matchups = {(item.get("deckA"), item.get("deckB")) for item in matchups}
    add(errors, len(matchups) == 36 and actual_matchups == expected_matchups, "conformance: expected all 36 ordered deck matchups")
    add(errors, sum(item.get("kind") == "mirror" for item in matchups) == 6, "conformance: expected six mirrors")

    discrepancy_cards = {item["cardId"] for item in discrepancies.get("items") or []}
    for item in discrepancies.get("items") or []:
        add(errors, item.get("ownerDecisionRef") == "OQ-07", f"{item.get('cardId')}: missing OQ-07 decision")
        add(errors, item.get("resolution") == "preserve-both-source-spellings", f"{item.get('cardId')}: silent normalization")
    expected_discrepancies = set()
    for deck in payloads.get("deckManifests") or []:
        for entry in [deck["hero"], *deck["orders"], *deck["main"]]:
            definition = by_card_id.get(entry["definitionId"])
            if definition and entry["name"] != definition["name"]:
                expected_discrepancies.add(entry["definitionId"])
    add(errors, discrepancy_cards == expected_discrepancies, "name discrepancy registry is incomplete")
    return errors


def validate_repository(root: Path, require_owner_review: bool = False) -> list[str]:
    return validate_payloads(load_payloads(root), require_owner_review=require_owner_review)


def build_report(payloads: dict[str, Any], errors: list[str], strict_errors: list[str]) -> dict[str, Any]:
    definitions = payloads["definitions"]["definitions"]
    abilities = payloads["abilities"]["abilities"]
    clarifications = sum(len(card["clarifications"]) for card in definitions)
    owner_status = payloads["evidence"]["ownerVerification"]["status"]
    return {
        "schemaVersion": 1,
        "reportId": "base-card-closure-2026-10-17",
        "validatedAt": "2026-10-17",
        "contentVersion": payloads["definitions"]["contentVersion"],
        "catalogVersion": payloads["snapshot"]["source"]["catalogVersion"],
        "status": "pass" if not errors else "fail",
        "ownerVerificationStatus": owner_status,
        "strictAcceptanceStatus": "pass" if not strict_errors else "pending-owner-review",
        "counts": {
            "definitions": len(definitions),
            "abilities": len(abilities),
            "conformanceScenarios": len(payloads["scenarios"]["scenarios"]),
            "clarifications": clarifications,
            "imageReferences": len(payloads["licenses"]["cardImages"]),
            "deckRoots": len(payloads["closure"]["roots"]),
            "transitiveObjects": len(payloads["closure"]["transitiveObjectIds"]),
            "nameDiscrepancies": len(payloads["nameDiscrepancies"]["items"]),
        },
        "errors": errors,
        "strictAcceptanceBlockers": strict_errors,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo-root", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--require-owner-reviewed", action="store_true")
    parser.add_argument("--write-report", type=Path)
    parser.add_argument("--check-report", type=Path)
    args = parser.parse_args()

    root = args.repo_root.resolve()
    payloads = load_payloads(root)
    errors = validate_payloads(payloads, require_owner_review=False)
    strict_errors = validate_payloads(payloads, require_owner_review=True)
    selected_errors = strict_errors if args.require_owner_reviewed else errors
    report = build_report(payloads, errors, strict_errors)

    if args.write_report:
        report_path = args.write_report
        if not report_path.is_absolute():
            report_path = root / report_path
        report_path.parent.mkdir(parents=True, exist_ok=True)
        report_path.write_bytes(canonical_json(report))

    if args.check_report:
        report_path = args.check_report
        if not report_path.is_absolute():
            report_path = root / report_path
        if not report_path.exists():
            selected_errors.append(f"validation report is missing: {report_path}")
        elif read_json(report_path) != report:
            selected_errors.append(f"validation report is stale: {report_path}")

    if selected_errors:
        for error in selected_errors:
            print(f"ERROR: {error}")
        print(f"Card content validation failed with {len(selected_errors)} error(s).")
        return 1

    print(
        "Card content validation passed: "
        f"{len(payloads['definitions']['definitions'])} definitions, "
        f"{len(payloads['abilities']['abilities'])} abilities, "
        f"{len(payloads['scenarios']['scenarios'])} scenarios."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
