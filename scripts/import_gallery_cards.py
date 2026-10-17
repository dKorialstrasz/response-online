#!/usr/bin/env python3
"""Build deterministic M1 card-content artifacts from official gallery API pages.

The importer performs no OCR and never downloads card images. It preserves the
official Russian gallery payload as evidence, then derives versioned local-only
content records from that snapshot.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


GALLERY_URL = "https://response-game.ru/spoiler"
API_URL = "https://response-league.ru/api/v1/cards"
SET_NAME = "Базовый"
SET_CODE = "БАЗ"
LOCALE = "ru-RU"
CONTENT_VERSION = "base-cards-v1"
EVIDENCE_ID = "base-gallery-2026-10-17"
CHECKED_AT = "2026-10-17"
CREATED_OBJECT_MARKERS = ("создайте", "создаёт", "создает", "созданн", "жетон")


def canonical_json(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8")


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256_text(value: str) -> str:
    return sha256_bytes(value.encode("utf-8"))


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(canonical_json(value))


def load_catalog(page_paths: list[Path]) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    pages = [read_json(path) for path in page_paths]
    versions = {page["meta"]["catalog_version"] for page in pages}
    if len(versions) != 1:
        raise ValueError(f"mixed catalog versions: {sorted(versions)}")

    expected_pages = pages[0]["meta"]["last_page"]
    page_numbers = sorted(page["meta"]["current_page"] for page in pages)
    if page_numbers != list(range(1, expected_pages + 1)):
        raise ValueError(f"incomplete pagination: {page_numbers}, expected 1..{expected_pages}")

    cards = [card for page in pages for card in page["data"]]
    if len(cards) != pages[0]["meta"]["total"]:
        raise ValueError("catalog total does not match downloaded page data")

    meta = {
        "catalogVersion": versions.pop(),
        "pageCount": expected_pages,
        "catalogCardCount": len(cards),
    }
    return cards, meta


def base_printing(card: dict[str, Any]) -> dict[str, Any] | None:
    matches = [
        printing
        for printing in card["printings"]
        if printing["set"] == SET_NAME and re.fullmatch(r"baz-\d{3}", printing["id"])
    ]
    if not matches:
        return None
    if len(matches) != 1:
        raise ValueError(f"{card['id']}: expected one {SET_NAME} printing")
    return matches[0]


def value_expression(value: int | None, *, applicable: bool) -> dict[str, Any]:
    if not applicable:
        return {"kind": "notApplicable", "verificationStatus": "source-matched"}
    if value is None:
        return {"kind": "unknown", "value": None, "verificationStatus": "unknown"}
    return {"kind": "fixed", "value": value, "verificationStatus": "source-matched"}


def evidence_entry(
    source_card_id: str,
    field_paths: list[str],
    catalog_version: str,
    snapshot_sha256: str,
    owner_review_status: str,
) -> dict[str, Any]:
    return {
        "evidenceId": EVIDENCE_ID,
        "sourceCardId": source_card_id,
        "fieldPaths": field_paths,
        "sourceUrl": GALLERY_URL,
        "dataEndpoint": API_URL,
        "sourceRevision": catalog_version,
        "snapshotSha256": snapshot_sha256,
        "checkedAt": CHECKED_AT,
        "transferredBy": "ChatGPT",
        "transferMethod": "official-json-api-no-ocr",
        "verificationStatus": "source-matched",
        "ownerReviewStatus": owner_review_status,
    }


def build_face(
    *,
    card_id: str,
    face_id: str,
    exact_text: str,
    level: int | None,
) -> tuple[dict[str, Any], dict[str, Any] | None, dict[str, Any] | None]:
    ability_id = None
    ability = None
    scenario = None
    if exact_text:
        ability_id = f"{card_id}.{face_id}.ability"
        scenario_id = f"{ability_id}.conformance"
        ability = {
            "abilityId": ability_id,
            "cardId": card_id,
            "faceId": face_id,
            "exactTextSha256": sha256_text(exact_text),
            "source": {
                "evidenceId": EVIDENCE_ID,
                "fieldPath": f"cards[{card_id}].faces[{face_id}].exactAbilityText",
            },
            "effectSpec": {
                "status": "deferred",
                "trackingIssue": "#12",
                "reason": "Issue #6 records exact text; executable semantics belong to M1-P10.",
            },
            "ambiguityReview": {
                "status": "none-recorded",
                "identifiedAmbiguities": [],
                "ownerDecisionRefs": [],
            },
            "conformanceScenarioIds": [scenario_id],
        }
        scenario = {
            "scenarioId": scenario_id,
            "abilityId": ability_id,
            "cardId": card_id,
            "faceId": face_id,
            "sourceTextSha256": sha256_text(exact_text),
            "sourceAssertion": "The scenario is anchored to the exact verified Russian text.",
            "gameplayStatus": "planned-in-issue-12",
            "requiredAssertions": [
                "legal timing and controller",
                "targets, choices, and payment",
                "state changes and emitted events",
                "negative and boundary cases",
            ],
        }

    face = {
        "faceId": face_id,
        "level": level,
        "exactAbilityText": exact_text,
        "abilityIds": [ability_id] if ability_id else [],
    }
    return face, ability, scenario


def build_artifacts(
    all_cards: list[dict[str, Any]],
    meta: dict[str, Any],
    deck_paths: list[Path],
    owner_reviewed_by: str | None = None,
    owner_reviewed_at: str | None = None,
) -> dict[str, Any]:
    if bool(owner_reviewed_by) != bool(owner_reviewed_at):
        raise ValueError("owner reviewer and review date must be supplied together")
    owner_review_status = "approved" if owner_reviewed_by else "pending"
    selected: list[tuple[int, dict[str, Any], dict[str, Any]]] = []
    for card in all_cards:
        printing = base_printing(card)
        if printing is not None:
            selected.append((int(printing["number"]), card, printing))
    selected.sort(key=lambda item: item[0])

    expected_numbers = list(range(1, 133))
    actual_numbers = [number for number, _, _ in selected]
    if actual_numbers != expected_numbers:
        raise ValueError("Base printing numbers are not the complete 1..132 range")

    deck_entries: dict[str, dict[str, Any]] = {}
    deck_roots: list[dict[str, Any]] = []
    order_ids: set[str] = set()
    for path in sorted(deck_paths):
        deck = read_json(path)
        refs = [deck["hero"], *deck["orders"], *deck["main"]]
        order_ids.update(item["definitionId"] for item in deck["orders"])
        deck_roots.append(
            {
                "deckId": deck["deckId"],
                "manifestPath": f"content/decks/base/{path.name}",
                "definitionIds": [item["definitionId"] for item in refs],
            }
        )
        for entry in refs:
            deck_entries.setdefault(entry["definitionId"], entry)

    snapshot = {
        "schemaVersion": 1,
        "source": {
            "galleryUrl": GALLERY_URL,
            "apiUrl": API_URL,
            "checkedAt": CHECKED_AT,
            **meta,
        },
        "selection": {
            "set": SET_NAME,
            "setCode": SET_CODE,
            "printingNumberRange": [1, 132],
        },
        "cards": [card for _, card, _ in selected],
    }
    snapshot_sha256 = sha256_bytes(canonical_json(snapshot))

    definitions: list[dict[str, Any]] = []
    abilities: list[dict[str, Any]] = []
    scenarios: list[dict[str, Any]] = []
    image_assets: list[dict[str, Any]] = []

    for number, source, printing in selected:
        card_id = printing["id"]
        card_types = source.get("types") or []
        is_hero = "Герой" in card_types
        is_unit = "Юнит" in card_types
        is_order = card_id in order_ids

        faces: list[dict[str, Any]] = []
        if source.get("levels"):
            for index, level_data in enumerate(source["levels"], start=1):
                face, ability, scenario = build_face(
                    card_id=card_id,
                    face_id=f"level-{index}",
                    exact_text=level_data.get("text") or "",
                    level=index,
                )
                faces.append(face)
                if ability:
                    abilities.append(ability)
                if scenario:
                    scenarios.append(scenario)
        else:
            face, ability, scenario = build_face(
                card_id=card_id,
                face_id="front",
                exact_text=source.get("text") or "",
                level=None,
            )
            faces.append(face)
            if ability:
                abilities.append(ability)
            if scenario:
                scenarios.append(scenario)

        asset_ids: list[str] = []
        for image in sorted(printing.get("images") or [], key=lambda item: (item["face"], item["position"])):
            asset_id = f"{card_id}-{image['face']}-{image['position']}"
            asset_ids.append(asset_id)
            image_assets.append(
                {
                    "assetId": asset_id,
                    "kind": "card-image-reference",
                    "cardId": card_id,
                    "face": image["face"],
                    "position": image["position"],
                    "sourceUrls": {
                        "thumbnail": image.get("thumb_url"),
                        "medium": image.get("medium_url"),
                    },
                    "rightsStatus": "unknown",
                    "bundled": False,
                    "usePolicy": "reference-only-not-for-public-build",
                }
            )

        influence = None
        if is_hero and source.get("levels"):
            influence = source["levels"][0].get("influence")

        identity_paths = [
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
        ]
        rules_paths = [
            "cost",
            "stats",
            "faces",
            "clarifications",
            "transitiveRefs",
        ]
        media_paths = ["imageAssetIds"]

        text_for_closure = "\n".join(face["exactAbilityText"] for face in faces).lower()
        marker_hits = [marker for marker in CREATED_OBJECT_MARKERS if marker in text_for_closure]
        if marker_hits:
            raise ValueError(f"{card_id}: created-object marker requires manual reference: {marker_hits}")

        definition = {
            "cardId": card_id,
            "sourceCardId": source["id"],
            "setCode": SET_CODE,
            "setName": SET_NAME,
            "printedNumber": number,
            "locale": LOCALE,
            "name": source["name"],
            "typeLine": {
                "supertypes": source.get("supertypes") or [],
                "types": card_types,
                "subtypes": source.get("subtypes") or [],
            },
            "factions": source.get("factions") or [],
            "rarity": printing.get("rarity"),
            "keywords": source.get("keywords") or [],
            "cost": value_expression(source.get("cost"), applicable=not is_hero and not is_order),
            "stats": {
                "influence": value_expression(influence, applicable=is_hero),
                "strength": value_expression(source.get("strength"), applicable=is_unit),
                "durability": value_expression(None, applicable=False),
            },
            "faces": faces,
            "clarifications": [
                {"clarificationId": f"gallery-{item['id']}", "text": item["body"]}
                for item in source.get("rulings") or []
            ],
            "effectSpecVersion": "effect-spec-v1",
            "imageAssetIds": asset_ids,
            "transitiveRefs": [],
            "verificationStatus": (
                "source-matched-owner-approved"
                if owner_review_status == "approved"
                else "source-matched-pending-owner-review"
            ),
            "evidence": [
                evidence_entry(
                    source["id"], identity_paths, meta["catalogVersion"], snapshot_sha256,
                    owner_review_status,
                ),
                evidence_entry(
                    source["id"], rules_paths, meta["catalogVersion"], snapshot_sha256,
                    owner_review_status,
                ),
                evidence_entry(
                    source["id"], media_paths, meta["catalogVersion"], snapshot_sha256,
                    owner_review_status,
                ),
            ],
        }
        definitions.append(definition)

    by_id = {definition["cardId"]: definition for definition in definitions}
    name_discrepancies = []
    for card_id, entry in sorted(deck_entries.items()):
        gallery_name = by_id[card_id]["name"]
        if entry["name"] != gallery_name:
            name_discrepancies.append(
                {
                    "cardId": card_id,
                    "productPageName": entry["name"],
                    "galleryName": gallery_name,
                    "resolution": "preserve-both-source-spellings",
                    "ownerDecisionRef": "OQ-07",
                    "displayCanonicalSource": GALLERY_URL,
                }
            )

    definition_ids = sorted(by_id)
    deck_definition_ids = sorted(deck_entries)
    unresolved = sorted(set(deck_definition_ids) - set(definition_ids))
    unreachable = sorted(set(definition_ids) - set(deck_definition_ids))

    return {
        "snapshot": snapshot,
        "evidence": {
            "schemaVersion": 1,
            "evidenceId": EVIDENCE_ID,
            "source": snapshot["source"],
            "snapshotPath": "content/evidence/base-gallery-snapshot.json",
            "snapshotSha256": snapshot_sha256,
            "captureMethod": "official-json-api-no-ocr",
            "scope": "All 132 Base-set definitions used by the six M1 starter decks.",
            "transferVerification": {
                "status": "source-matched",
                "performedBy": "ChatGPT",
                "performedAt": CHECKED_AT,
            },
            "ownerVerification": {
                "status": owner_review_status,
                "reviewer": owner_reviewed_by or "dKorialstrasz",
                "reviewedAt": owner_reviewed_at,
                "decision": (
                    "accepted-card-content-and-authorized-merge"
                    if owner_review_status == "approved"
                    else "pending"
                ),
                "decisionSource": (
                    "ChatGPT project conversation"
                    if owner_review_status == "approved"
                    else None
                ),
            },
        },
        "definitions": {
            "schemaVersion": 1,
            "contentVersion": CONTENT_VERSION,
            "locale": LOCALE,
            "setCode": SET_CODE,
            "evidenceId": EVIDENCE_ID,
            "reviewStatus": (
                "approved" if owner_review_status == "approved" else "pending-owner-review"
            ),
            "definitions": definitions,
        },
        "abilities": {
            "schemaVersion": 1,
            "contentVersion": CONTENT_VERSION,
            "effectSpecVersion": "effect-spec-v1",
            "abilities": abilities,
        },
        "scenarios": {
            "schemaVersion": 1,
            "contentVersion": CONTENT_VERSION,
            "scenarios": scenarios,
        },
        "licenses": {
            "schemaVersion": 1,
            "registryVersion": "m1-local-assets-v1",
            "publicReleaseGate": "blocked-rights-unknown",
            "cardImages": image_assets,
            "fonts": {"status": "no-bundled-font-assets", "assets": []},
            "sounds": {"status": "no-sound-assets", "assets": []},
            "advertising": {"status": "no-advertising-assets", "assets": []},
        },
        "closure": {
            "schemaVersion": 1,
            "contentVersion": CONTENT_VERSION,
            "roots": deck_roots,
            "resolvedDefinitionIds": definition_ids,
            "transitiveObjectIds": [],
            "unresolvedDefinitionIds": unresolved,
            "unreachableDefinitionIds": unreachable,
            "createdObjectMarkerPolicy": list(CREATED_OBJECT_MARKERS),
            "status": "closed" if not unresolved and not unreachable else "open",
        },
        "nameDiscrepancies": {
            "schemaVersion": 1,
            "decisionRef": "OQ-07",
            "items": name_discrepancies,
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("pages", nargs="+", type=Path)
    parser.add_argument("--repo-root", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--owner-reviewed-by")
    parser.add_argument("--owner-reviewed-at")
    args = parser.parse_args()

    repo_root = args.repo_root.resolve()
    cards, meta = load_catalog(args.pages)
    deck_paths = sorted((repo_root / "content/decks/base").glob("*.json"))
    artifacts = build_artifacts(
        cards,
        meta,
        deck_paths,
        owner_reviewed_by=args.owner_reviewed_by,
        owner_reviewed_at=args.owner_reviewed_at,
    )

    outputs = {
        "content/evidence/base-gallery-snapshot.json": artifacts["snapshot"],
        "content/evidence/base-gallery-evidence.json": artifacts["evidence"],
        "content/cards/base/card-definitions.json": artifacts["definitions"],
        "content/abilities/base-abilities.json": artifacts["abilities"],
        "content/conformance/base-ability-scenarios.json": artifacts["scenarios"],
        "content/licenses/registry.json": artifacts["licenses"],
        "content/closure/base-content-closure.json": artifacts["closure"],
        "content/reviews/base-name-discrepancies.json": artifacts["nameDiscrepancies"],
    }
    for relative, payload in outputs.items():
        write_json(repo_root / relative, payload)
        print(relative)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
