#!/usr/bin/env python3
"""Validate the six fixed Base-set deck manifests using the standard library."""

from __future__ import annotations

import argparse
import json
import re
import sys
from datetime import date
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MANIFEST_DIR = ROOT / "content" / "decks" / "base"
CYRILLIC_RE = re.compile(r"[А-Яа-яЁё]")
DEFINITION_ID_RE = re.compile(r"baz-(\d{3})")
SHA256_RE = re.compile(r"[0-9a-f]{64}")

EXPECTED = {
    "chaos-enitokl.json": {
        "deckId": "chaos-enitokl",
        "displayName": "Энитокль",
        "factionName": "Хаос",
        "sourceUrl": "https://response-game.ru/store/tproduct/781431895752-startovaya-koloda-enitokl",
        "sha256": "a3f52e339a3fefa8e4b2f2e4545014cb8f95a5dbb95c1f6398c3694d7c2070dc",
    },
    "death-kassiy.json": {
        "deckId": "death-kassiy",
        "displayName": "Кассий",
        "factionName": "Смерть",
        "sourceUrl": "https://response-game.ru/store/tproduct/167585109542-startovaya-koloda-kassii",
        "sha256": "86572cf65d0839fc82f129a540f2efd5b409572cec4cdf2912e15407e0a1b2cf",
    },
    "life-katarina.json": {
        "deckId": "life-katarina",
        "displayName": "Катарина",
        "factionName": "Жизнь",
        "sourceUrl": "https://response-game.ru/store/tproduct/370965877392-startovaya-koloda-katarina",
        "sha256": "e8f486354d5725e5300985deb6680601ed43fe65a85eaf1a0298b92b9a5104a5",
    },
    "nature-medana.json": {
        "deckId": "nature-medana",
        "displayName": "Медана",
        "factionName": "Природа",
        "sourceUrl": "https://response-game.ru/store/tproduct/762054556062-startovaya-koloda-medana",
        "sha256": "e181c29148cbbfebb4f4cb3086b58a908f81b4b220e7a92b3060675fc816e62e",
    },
    "order-bjorn.json": {
        "deckId": "order-bjorn",
        "displayName": "Бьёрн",
        "factionName": "Порядок",
        "sourceUrl": "https://response-game.ru/store/tproduct/978528586692-startovaya-koloda-byorn",
        "sha256": "3e5e9a4a81282919f56bdca5bae1a0ac369280fa6e3a046ff96b02b0e97d9f0f",
    },
    "technology-dari.json": {
        "deckId": "technology-dari",
        "displayName": "Дари",
        "factionName": "Технологии",
        "sourceUrl": "https://response-game.ru/store/tproduct/961576141202-startovaya-koloda-dari",
        "sha256": "ff5a02fb591ec473bea2f56dd7a5467d45a1fd7d33a6e2a7ceff6f7ad9df290d",
    },
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--require-reviewed",
        action="store_true",
        help="fail unless every manifest has explicit owner approval",
    )
    return parser.parse_args()


def validate_date(value: object, field: str, errors: list[str]) -> None:
    if not isinstance(value, str):
        errors.append(f"{field}: expected ISO date string")
        return
    try:
        date.fromisoformat(value)
    except ValueError:
        errors.append(f"{field}: invalid ISO date {value!r}")


def validate_card(
    card: object,
    field: str,
    expected_category: str | set[str],
    errors: list[str],
) -> tuple[int | None, str | None]:
    if not isinstance(card, dict):
        errors.append(f"{field}: expected object")
        return None, None

    required = {"definitionId", "name", "printedNumber", "category", "count"}
    missing = required - card.keys()
    if missing:
        errors.append(f"{field}: missing fields {sorted(missing)}")
        return None, None

    number = card["printedNumber"]
    definition_id = card["definitionId"]
    name = card["name"]
    category = card["category"]
    count = card["count"]

    if not isinstance(number, int) or not 1 <= number <= 132:
        errors.append(f"{field}.printedNumber: expected integer 1..132")
    match = DEFINITION_ID_RE.fullmatch(definition_id) if isinstance(definition_id, str) else None
    if not match or not isinstance(number, int) or int(match.group(1)) != number:
        errors.append(f"{field}.definitionId: must match printedNumber as baz-NNN")
    if not isinstance(name, str) or not name.strip() or not CYRILLIC_RE.search(name):
        errors.append(f"{field}.name: expected non-empty Russian display text")

    allowed_categories = (
        {expected_category} if isinstance(expected_category, str) else expected_category
    )
    if category not in allowed_categories:
        errors.append(f"{field}.category: expected one of {sorted(allowed_categories)}")
    if not isinstance(count, int) or count <= 0:
        errors.append(f"{field}.count: expected positive integer")

    return number if isinstance(number, int) else None, definition_id if isinstance(definition_id, str) else None


def main() -> int:
    args = parse_args()
    errors: list[str] = []
    seen_numbers: set[int] = set()
    seen_ids: set[str] = set()
    pending_reviews = 0

    actual_files = {path.name for path in MANIFEST_DIR.glob("*.json")}
    expected_files = set(EXPECTED)
    if actual_files != expected_files:
        errors.append(
            f"manifest files mismatch: missing={sorted(expected_files - actual_files)}, "
            f"extra={sorted(actual_files - expected_files)}"
        )

    for filename, expected in EXPECTED.items():
        path = MANIFEST_DIR / filename
        if not path.is_file():
            continue
        try:
            manifest = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            errors.append(f"{filename}: cannot parse JSON: {exc}")
            continue

        prefix = filename
        for field, value in (
            ("schemaVersion", 1),
            ("locale", "ru-RU"),
            ("setCode", "БАЗ"),
            ("contentVersion", "base-decks-v1"),
            ("deckId", expected["deckId"]),
            ("displayName", expected["displayName"]),
            ("factionName", expected["factionName"]),
        ):
            if manifest.get(field) != value:
                errors.append(f"{prefix}.{field}: expected {value!r}")

        if not CYRILLIC_RE.search(str(manifest.get("displayName", ""))):
            errors.append(f"{prefix}.displayName: expected Russian display text")
        if not CYRILLIC_RE.search(str(manifest.get("factionName", ""))):
            errors.append(f"{prefix}.factionName: expected Russian display text")

        source = manifest.get("source")
        if not isinstance(source, dict):
            errors.append(f"{prefix}.source: expected object")
        else:
            if source.get("url") != expected["sourceUrl"]:
                errors.append(f"{prefix}.source.url: unexpected official URL")
            if source.get("sha256") != expected["sha256"]:
                errors.append(f"{prefix}.source.sha256: unexpected evidence hash")
            if not SHA256_RE.fullmatch(str(source.get("sha256", ""))):
                errors.append(f"{prefix}.source.sha256: invalid SHA-256")
            validate_date(source.get("checkedAt"), f"{prefix}.source.checkedAt", errors)

        review_status = manifest.get("reviewStatus")
        reviewed_by = manifest.get("reviewedBy")
        reviewed_at = manifest.get("reviewedAt")
        if review_status == "pending":
            pending_reviews += 1
            if reviewed_by is not None or reviewed_at is not None:
                errors.append(f"{prefix}: pending review must not name a reviewer/date")
            if args.require_reviewed:
                errors.append(f"{prefix}: owner review is still pending")
        elif review_status == "approved":
            if reviewed_by != "dKorialstrasz":
                errors.append(f"{prefix}.reviewedBy: expected 'dKorialstrasz'")
            validate_date(reviewed_at, f"{prefix}.reviewedAt", errors)
        else:
            errors.append(f"{prefix}.reviewStatus: expected 'pending' or 'approved'")

        deck_numbers: list[int] = []
        deck_ids: list[str] = []
        number, definition_id = validate_card(
            manifest.get("hero"), f"{prefix}.hero", "hero", errors
        )
        if isinstance(manifest.get("hero"), dict) and manifest["hero"].get("count") != 1:
            errors.append(f"{prefix}.hero.count: expected 1")
        if number is not None:
            deck_numbers.append(number)
        if definition_id is not None:
            deck_ids.append(definition_id)

        orders = manifest.get("orders")
        if not isinstance(orders, list) or len(orders) != 5:
            errors.append(f"{prefix}.orders: expected exactly 5 entries")
            orders = orders if isinstance(orders, list) else []
        for index, item in enumerate(orders):
            number, definition_id = validate_card(
                item, f"{prefix}.orders[{index}]", "order", errors
            )
            if isinstance(item, dict) and item.get("count") != 1:
                errors.append(f"{prefix}.orders[{index}].count: expected 1")
            if number is not None:
                deck_numbers.append(number)
            if definition_id is not None:
                deck_ids.append(definition_id)

        main_cards = manifest.get("main")
        if not isinstance(main_cards, list):
            errors.append(f"{prefix}.main: expected list")
            main_cards = []
        for index, item in enumerate(main_cards):
            number, definition_id = validate_card(
                item,
                f"{prefix}.main[{index}]",
                {"unit", "spell", "upgrade"},
                errors,
            )
            if number is not None:
                deck_numbers.append(number)
            if definition_id is not None:
                deck_ids.append(definition_id)

        main_count = sum(
            item.get("count", 0) for item in main_cards if isinstance(item, dict)
        )
        order_count = sum(
            item.get("count", 0) for item in orders if isinstance(item, dict)
        )
        if main_count != 30:
            errors.append(f"{prefix}.main: expected 30 cards, found {main_count}")
        if order_count != 5:
            errors.append(f"{prefix}.orders: expected 5 cards, found {order_count}")
        if len(deck_numbers) != 22 or len(set(deck_numbers)) != 22:
            errors.append(f"{prefix}: expected 22 unique printed entries")
        if len(deck_ids) != len(set(deck_ids)):
            errors.append(f"{prefix}: duplicate definitionId")

        duplicates = seen_numbers.intersection(deck_numbers)
        if duplicates:
            errors.append(f"{prefix}: printed numbers repeated across decks: {sorted(duplicates)}")
        duplicate_ids = seen_ids.intersection(deck_ids)
        if duplicate_ids:
            errors.append(f"{prefix}: definitionIds repeated across decks: {sorted(duplicate_ids)}")
        seen_numbers.update(deck_numbers)
        seen_ids.update(deck_ids)

    expected_numbers = set(range(1, 133))
    if seen_numbers != expected_numbers:
        errors.append(
            f"Base set coverage mismatch: missing={sorted(expected_numbers - seen_numbers)}, "
            f"extra={sorted(seen_numbers - expected_numbers)}"
        )

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print(
        f"Deck validation passed: {len(EXPECTED)} manifests, "
        f"132 unique definitions, {pending_reviews} pending owner review."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
