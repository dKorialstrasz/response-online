#!/usr/bin/env python3
"""Validate the versioned M1 content release without third-party packages."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
RELEASE_PATH = "content/releases/m1-local.json"
SCHEMA_BINDINGS = {
    "content/decks/base/*.json": "content/schemas/deck-manifest.schema.json",
    "content/cards/base/card-definitions.json": "content/schemas/card-definitions.schema.json",
    "content/evidence/base-gallery-evidence.json": "content/schemas/gallery-evidence.schema.json",
    "content/abilities/base-abilities.json": "content/schemas/abilities.schema.json",
    "content/conformance/base-ability-scenarios.json": "content/schemas/conformance-scenarios.schema.json",
    "content/licenses/registry.json": "content/schemas/license-registry.schema.json",
    RELEASE_PATH: "content/schemas/release-manifest.schema.json",
}
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def canonical_json(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8")


def canonical_sha256(value: Any) -> str:
    return hashlib.sha256(canonical_json(value)).hexdigest()


def resolve_pointer(root_schema: dict[str, Any], reference: str) -> dict[str, Any]:
    if not reference.startswith("#/"):
        raise ValueError(f"only local JSON pointers are supported: {reference}")
    current: Any = root_schema
    for token in reference[2:].split("/"):
        token = token.replace("~1", "/").replace("~0", "~")
        current = current[token]
    if not isinstance(current, dict):
        raise ValueError(f"schema reference is not an object: {reference}")
    return current


def type_matches(value: Any, expected: str) -> bool:
    return {
        "object": isinstance(value, dict),
        "array": isinstance(value, list),
        "string": isinstance(value, str),
        "integer": isinstance(value, int) and not isinstance(value, bool),
        "number": isinstance(value, (int, float)) and not isinstance(value, bool),
        "boolean": isinstance(value, bool),
        "null": value is None,
    }.get(expected, False)


def validate_schema(
    value: Any,
    schema: dict[str, Any],
    path: str,
    root_schema: dict[str, Any],
) -> list[str]:
    errors: list[str] = []
    if "$ref" in schema:
        return validate_schema(value, resolve_pointer(root_schema, schema["$ref"]), path, root_schema)

    if "oneOf" in schema:
        results = [validate_schema(value, option, path, root_schema) for option in schema["oneOf"]]
        matches = [result for result in results if not result]
        if len(matches) != 1:
            errors.append(f"{path}: expected exactly one schema in oneOf to match")
        return errors

    expected_type = schema.get("type")
    if isinstance(expected_type, list):
        if not any(type_matches(value, item) for item in expected_type):
            return [f"{path}: expected one of types {expected_type}"]
    elif isinstance(expected_type, str) and not type_matches(value, expected_type):
        return [f"{path}: expected {expected_type}"]

    if "const" in schema and value != schema["const"]:
        errors.append(f"{path}: expected constant {schema['const']!r}")
    if "enum" in schema and value not in schema["enum"]:
        errors.append(f"{path}: expected one of {schema['enum']!r}")

    if isinstance(value, dict):
        properties = schema.get("properties", {})
        for required in schema.get("required", []):
            if required not in value:
                errors.append(f"{path}: missing required field {required!r}")
        for key, child in value.items():
            if key in properties:
                errors.extend(validate_schema(child, properties[key], f"{path}.{key}", root_schema))
            elif schema.get("additionalProperties") is False:
                errors.append(f"{path}: unexpected field {key!r}")

    if isinstance(value, list):
        if len(value) < schema.get("minItems", 0):
            errors.append(f"{path}: expected at least {schema['minItems']} items")
        if "maxItems" in schema and len(value) > schema["maxItems"]:
            errors.append(f"{path}: expected at most {schema['maxItems']} items")
        if schema.get("uniqueItems"):
            encoded = [json.dumps(item, ensure_ascii=False, sort_keys=True) for item in value]
            if len(encoded) != len(set(encoded)):
                errors.append(f"{path}: array items must be unique")
        if isinstance(schema.get("items"), dict):
            for index, item in enumerate(value):
                errors.extend(validate_schema(item, schema["items"], f"{path}[{index}]", root_schema))

    if isinstance(value, str):
        if len(value) < schema.get("minLength", 0):
            errors.append(f"{path}: string is shorter than {schema['minLength']}")
        pattern = schema.get("pattern")
        if pattern and re.search(pattern, value) is None:
            errors.append(f"{path}: does not match {pattern!r}")

    if isinstance(value, (int, float)) and not isinstance(value, bool):
        if "minimum" in schema and value < schema["minimum"]:
            errors.append(f"{path}: expected value >= {schema['minimum']}")
        if "maximum" in schema and value > schema["maximum"]:
            errors.append(f"{path}: expected value <= {schema['maximum']}")

    return errors


def repository_documents(root: Path) -> dict[str, Any]:
    paths: list[Path] = []
    for pattern in SCHEMA_BINDINGS:
        paths.extend(sorted(root.glob(pattern)))
    return {path.relative_to(root).as_posix(): read_json(path) for path in paths}


def schema_for(path: str) -> str:
    for pattern, schema_path in SCHEMA_BINDINGS.items():
        if Path(path).match(pattern):
            return schema_path
    raise KeyError(path)


def card_ids_from_manifest(manifest: dict[str, Any]) -> list[str]:
    cards = [manifest.get("hero"), *(manifest.get("orders") or []), *(manifest.get("main") or [])]
    return [card.get("definitionId") for card in cards if isinstance(card, dict)]


def validate_semantics(documents: dict[str, Any], require_gameplay_ready: bool) -> list[str]:
    errors: list[str] = []
    release = documents[RELEASE_PATH]
    definitions_doc = documents["content/cards/base/card-definitions.json"]
    abilities_doc = documents["content/abilities/base-abilities.json"]
    licenses = documents["content/licenses/registry.json"]
    deck_paths = sorted(path for path in documents if path.startswith("content/decks/base/"))

    if len(deck_paths) != 6:
        errors.append(f"release: expected 6 deck manifests, found {len(deck_paths)}")

    definitions = definitions_doc.get("definitions") or []
    definition_ids = [item.get("cardId") for item in definitions if isinstance(item, dict)]
    definition_set = set(definition_ids)
    if len(definition_ids) != 132 or len(definition_set) != 132:
        errors.append("release: expected 132 unique CardDefinition IDs")

    referenced_ids: list[str] = []
    for path in deck_paths:
        manifest = documents[path]
        main_count = sum(item.get("count", 0) for item in manifest.get("main") or [])
        order_count = sum(item.get("count", 0) for item in manifest.get("orders") or [])
        hero_count = (manifest.get("hero") or {}).get("count", 0)
        if (main_count, order_count, hero_count) != (30, 5, 1):
            errors.append(f"{path}: expected deck arithmetic 30/5/1")
        referenced_ids.extend(card_ids_from_manifest(manifest))
    if set(referenced_ids) != definition_set:
        errors.append("release: deck references do not close over all CardDefinition IDs")

    ability_ids = {
        item.get("abilityId") for item in abilities_doc.get("abilities") or [] if isinstance(item, dict)
    }
    for definition in definitions:
        if not isinstance(definition, dict):
            continue
        if definition.get("verificationStatus") != "source-matched-owner-approved":
            errors.append(f"definitions[{definition.get('cardId')}]: required fields are not owner-approved")
        for face in definition.get("faces") or []:
            for ability_id in face.get("abilityIds") or []:
                if ability_id not in ability_ids:
                    errors.append(f"definitions[{definition.get('cardId')}]: unknown ability {ability_id}")

    license_ids = {item.get("cardId") for item in licenses.get("cardImages") or []}
    if not definition_set.issubset(license_ids):
        errors.append("release: every CardDefinition must have a license-registry entry")
    if licenses.get("publicReleaseGate") != "blocked-rights-unknown":
        errors.append("release: unknown rights must block public release")
    if any(item.get("rightsStatus") != "unknown" or item.get("bundled") is not False for item in licenses.get("cardImages") or []):
        errors.append("release: card image rights must remain unknown and images unbundled")

    versions = release.get("versions") or {}
    if versions.get("content") != definitions_doc.get("contentVersion"):
        errors.append("release.versions.content: does not match CardDefinition contentVersion")
    if versions.get("effectSpec") != abilities_doc.get("effectSpecVersion"):
        errors.append("release.versions.effectSpec: does not match ability effectSpecVersion")
    for path in deck_paths:
        if documents[path].get("contentVersion") != versions.get("deckManifests"):
            errors.append(f"{path}: contentVersion does not match release deckManifests version")

    expected_artifacts = {
        path for path in documents if path != RELEASE_PATH and not path.startswith("content/schemas/")
    }
    declared_artifacts = {item.get("path") for item in release.get("artifacts") or []}
    if declared_artifacts != expected_artifacts:
        errors.append("release.artifacts: exact runtime content set is not pinned")
    for artifact in release.get("artifacts") or []:
        path = artifact.get("path")
        expected_hash = artifact.get("canonicalSha256")
        if path not in documents:
            errors.append(f"release.artifacts: missing document {path!r}")
        elif artifact.get("schema") != schema_for(path):
            errors.append(f"release.artifacts[{path}]: schema binding mismatch")
        elif not SHA256_RE.fullmatch(str(expected_hash or "")):
            errors.append(f"release.artifacts[{path}]: invalid SHA-256")
        elif canonical_sha256(documents[path]) != expected_hash:
            errors.append(f"release.artifacts[{path}]: canonical SHA-256 mismatch")

    deferred = [
        item.get("abilityId")
        for item in abilities_doc.get("abilities") or []
        if (item.get("effectSpec") or {}).get("status") != "implemented"
    ]
    if require_gameplay_ready:
        if deferred:
            errors.append(f"M1 release blocked: {len(deferred)} effect specs are not implemented")
        for field in ("rules", "protocol", "engine"):
            if versions.get(field) in {None, "unverified", "scaffold-v1"}:
                errors.append(f"M1 release blocked: {field} version is not release-ready")

    return errors


def validate_documents(
    root: Path,
    documents: dict[str, Any],
    require_gameplay_ready: bool = False,
) -> list[str]:
    errors: list[str] = []
    for path, document in documents.items():
        schema_path = schema_for(path)
        schema = read_json(root / schema_path)
        errors.extend(validate_schema(document, schema, path, schema))
    errors.extend(validate_semantics(documents, require_gameplay_ready))
    return errors


def validate_repository(root: Path, require_gameplay_ready: bool = False) -> list[str]:
    return validate_documents(root, repository_documents(root), require_gameplay_ready)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--require-gameplay-ready",
        action="store_true",
        help="fail until effect specs and runtime versions are ready for M1 acceptance",
    )
    args = parser.parse_args()
    errors = validate_repository(ROOT, args.require_gameplay_ready)
    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print("Content release validation passed: schemas, references, hashes, versions, and local-only rights gate.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
