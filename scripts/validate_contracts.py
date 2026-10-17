#!/usr/bin/env python3
"""Validate contract syntax and the implemented scaffold surface."""

from __future__ import annotations

import json
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
OPENAPI = ROOT / "contracts" / "http" / "openapi.json"
WS_SCHEMA = ROOT / "contracts" / "ws" / "envelope.schema.json"


def load_object(path: Path, errors: list[str]) -> dict[str, object]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        errors.append(f"{path.relative_to(ROOT)}: {exc}")
        return {}
    if not isinstance(value, dict):
        errors.append(f"{path.relative_to(ROOT)}: expected a JSON object")
        return {}
    return value


def main() -> int:
    errors: list[str] = []
    openapi = load_object(OPENAPI, errors)
    schema = load_object(WS_SCHEMA, errors)

    if openapi.get("openapi") != "3.1.1":
        errors.append("openapi.json: expected OpenAPI 3.1.1")
    info = openapi.get("info")
    if not isinstance(info, dict) or info.get("version") != "scaffold-v1":
        errors.append("openapi.json: expected info.version scaffold-v1")
    paths = openapi.get("paths")
    expected_paths = {"/healthz", "/readyz", "/api/v1/build"}
    if not isinstance(paths, dict) or set(paths) != expected_paths:
        errors.append(f"openapi.json: expected paths {sorted(expected_paths)}")
    servers = openapi.get("servers")
    if not isinstance(servers, list) or any(
        not isinstance(server, dict)
        or not str(server.get("url", "")).startswith("http://127.0.0.1")
        for server in servers
    ):
        errors.append("openapi.json: only explicit loopback servers are allowed in M1 scaffold")

    if schema.get("$schema") != "https://json-schema.org/draft/2020-12/schema":
        errors.append("envelope.schema.json: expected JSON Schema 2020-12")
    if schema.get("additionalProperties") is not False:
        errors.append("envelope.schema.json: envelope must reject unknown fields")
    required = schema.get("required")
    expected_required = {"protocolVersion", "messageType", "correlationId", "payload"}
    if not isinstance(required, list) or set(required) != expected_required:
        errors.append("envelope.schema.json: required envelope fields changed")

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print("Contract validation passed: OpenAPI diagnostics and WS envelope scaffold.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

