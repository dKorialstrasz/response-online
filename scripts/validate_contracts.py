#!/usr/bin/env python3
"""Validate versioned HTTP/WS contracts, examples and generated parity lists."""

from __future__ import annotations

import json
import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
OPENAPI = ROOT / "contracts" / "http" / "openapi.json"
WS_SCHEMA = ROOT / "contracts" / "ws" / "envelope.schema.json"
WS_EXAMPLES = ROOT / "contracts" / "ws" / "examples"
TS_PROTOCOL = ROOT / "frontend" / "src" / "protocol.ts"
GO_PROTOCOL = ROOT / "backend" / "internal" / "transport" / "httpserver" / "protocol.go"

EXPECTED_PATHS = {
    "/healthz",
    "/readyz",
    "/health/live",
    "/health/ready",
    "/api/v1/build",
    "/api/v1/sessions",
    "/api/v1/rooms",
    "/api/v1/rooms/join",
    "/api/v1/rooms/{roomId}",
    "/api/v1/rooms/{roomId}/selection",
    "/api/v1/rooms/{roomId}/ready",
    "/api/v1/rooms/{roomId}/leave",
    "/api/v1/ws-tickets",
    "/api/v1/ws",
}
EXPECTED_COMMANDS = {
    "selectDeck",
    "setOrderArrangement",
    "setReady",
    "submitMulligan",
    "playCard",
    "activateAbility",
    "passAction",
    "submitChoice",
    "surrender",
    "requestRematch",
    "leaveRoom",
    "resync",
}
EXPECTED_EVENTS = {
    "authenticated",
    "roomSnapshot",
    "matchSnapshot",
    "stateUpdate",
    "choiceRequested",
    "commandResult",
    "error",
    "connectionStatus",
    "lifecycleNotice",
    "matchFinished",
    "serverDraining",
    "heartbeat",
}
FORBIDDEN_PLAYER_KEYS = {
    "seed",
    "random",
    "instanceId",
    "commands",
    "diagnosticTrace",
    "effectQueue",
    "delayedTriggers",
    "replacements",
}


def load_object(path: Path, errors: list[str]) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        errors.append(f"{path.relative_to(ROOT)}: {exc}")
        return {}
    if not isinstance(value, dict):
        errors.append(f"{path.relative_to(ROOT)}: expected a JSON object")
        return {}
    return value


def literal_array(source: str, name: str) -> set[str]:
    match = re.search(
        rf"export const {re.escape(name)} = \[(.*?)\] as const;",
        source,
        re.DOTALL,
    )
    if not match:
        return set()
    return set(re.findall(r'"([^"]+)"', match.group(1)))


def command_payload_names(schema: dict[str, Any]) -> set[str]:
    names: set[str] = set()
    command = schema.get("$defs", {}).get("clientCommand", {})
    for rule in command.get("allOf", []):
        condition = rule.get("if", {}).get("properties", {}).get("name", {})
        if "const" in condition:
            names.add(condition["const"])
        names.update(condition.get("enum", []))
    return names


def walk_keys(value: Any) -> set[str]:
    keys: set[str] = set()
    if isinstance(value, dict):
        keys.update(value)
        for child in value.values():
            keys.update(walk_keys(child))
    elif isinstance(value, list):
        for child in value:
            keys.update(walk_keys(child))
    return keys


def resolve_ref(root: dict[str, Any], reference: str) -> dict[str, Any]:
    if not reference.startswith("#/"):
        raise ValueError(f"only local schema refs are supported: {reference}")
    value: Any = root
    for segment in reference[2:].split("/"):
        segment = segment.replace("~1", "/").replace("~0", "~")
        value = value[segment]
    if not isinstance(value, dict):
        raise ValueError(f"schema ref does not resolve to an object: {reference}")
    return value


def matches_type(value: Any, expected: str) -> bool:
    return {
        "null": value is None,
        "object": isinstance(value, dict),
        "array": isinstance(value, list),
        "string": isinstance(value, str),
        "boolean": isinstance(value, bool),
        "integer": isinstance(value, int) and not isinstance(value, bool),
        "number": isinstance(value, (int, float)) and not isinstance(value, bool),
    }.get(expected, False)


def schema_errors(
    value: Any,
    schema: dict[str, Any],
    root: dict[str, Any],
    path: str = "$",
) -> list[str]:
    errors: list[str] = []
    if "$ref" in schema:
        try:
            target = resolve_ref(root, str(schema["$ref"]))
        except (KeyError, TypeError, ValueError) as exc:
            return [f"{path}: {exc}"]
        return schema_errors(value, target, root, path)

    for child in schema.get("allOf", []):
        errors.extend(schema_errors(value, child, root, path))
    choices = schema.get("oneOf")
    if isinstance(choices, list):
        matched = sum(not schema_errors(value, child, root, path) for child in choices)
        if matched != 1:
            errors.append(f"{path}: expected exactly one oneOf branch, matched {matched}")
    condition = schema.get("if")
    if isinstance(condition, dict):
        branch = schema.get("then") if not schema_errors(value, condition, root, path) else schema.get("else")
        if isinstance(branch, dict):
            errors.extend(schema_errors(value, branch, root, path))

    if "const" in schema and value != schema["const"]:
        errors.append(f"{path}: expected constant {schema['const']!r}")
    if "enum" in schema and value not in schema["enum"]:
        errors.append(f"{path}: value {value!r} is not in enum")

    expected_types = schema.get("type")
    if isinstance(expected_types, str):
        expected_types = [expected_types]
    if isinstance(expected_types, list) and not any(matches_type(value, item) for item in expected_types):
        errors.append(f"{path}: expected type {expected_types}, got {type(value).__name__}")
        return errors

    if isinstance(value, dict):
        required = schema.get("required", [])
        for name in required:
            if name not in value:
                errors.append(f"{path}: missing required property {name!r}")
        properties = schema.get("properties", {})
        if isinstance(properties, dict):
            if schema.get("additionalProperties") is False:
                for name in value.keys() - properties.keys():
                    errors.append(f"{path}: additional property {name!r} is forbidden")
            for name, child in properties.items():
                if name in value and isinstance(child, dict):
                    errors.extend(schema_errors(value[name], child, root, f"{path}.{name}"))

    if isinstance(value, list):
        minimum = schema.get("minItems")
        maximum = schema.get("maxItems")
        if isinstance(minimum, int) and len(value) < minimum:
            errors.append(f"{path}: expected at least {minimum} items")
        if isinstance(maximum, int) and len(value) > maximum:
            errors.append(f"{path}: expected at most {maximum} items")
        if schema.get("uniqueItems") is True:
            encoded = [json.dumps(item, sort_keys=True, separators=(",", ":")) for item in value]
            if len(encoded) != len(set(encoded)):
                errors.append(f"{path}: array items must be unique")
        child = schema.get("items")
        if isinstance(child, dict):
            for index, item in enumerate(value):
                errors.extend(schema_errors(item, child, root, f"{path}[{index}]"))

    if isinstance(value, str):
        minimum = schema.get("minLength")
        maximum = schema.get("maxLength")
        if isinstance(minimum, int) and len(value) < minimum:
            errors.append(f"{path}: string is shorter than {minimum}")
        if isinstance(maximum, int) and len(value) > maximum:
            errors.append(f"{path}: string is longer than {maximum}")
        pattern = schema.get("pattern")
        if isinstance(pattern, str) and re.search(pattern, value) is None:
            errors.append(f"{path}: string does not match {pattern!r}")
        if schema.get("format") == "date-time":
            try:
                datetime.fromisoformat(value.replace("Z", "+00:00"))
            except ValueError:
                errors.append(f"{path}: invalid RFC3339 date-time")

    if isinstance(value, (int, float)) and not isinstance(value, bool):
        minimum = schema.get("minimum")
        maximum = schema.get("maximum")
        if isinstance(minimum, (int, float)) and value < minimum:
            errors.append(f"{path}: value is less than {minimum}")
        if isinstance(maximum, (int, float)) and value > maximum:
            errors.append(f"{path}: value is greater than {maximum}")
    return errors


def validate_examples(schema: dict[str, Any], errors: list[str]) -> None:
    paths = sorted(WS_EXAMPLES.glob("*.json"))
    if not paths:
        errors.append("contracts/ws/examples: at least one example is required")
        return
    command_enum = set(
        schema.get("$defs", {})
        .get("commandBase", {})
        .get("properties", {})
        .get("name", {})
        .get("enum", [])
    )
    event_enum = set(
        schema.get("$defs", {})
        .get("serverEvent", {})
        .get("properties", {})
        .get("type", {})
        .get("enum", [])
    )
    seen_types: set[str] = set()
    for path in paths:
        value = load_object(path, errors)
        if not value:
            continue
        for validation_error in schema_errors(value, schema, schema):
            errors.append(f"{path.relative_to(ROOT)}: {validation_error}")
        if value.get("protocolVersion") != "1.2":
            errors.append(f"{path.relative_to(ROOT)}: protocolVersion must be 1.1")
        message_type = value.get("type")
        seen_types.add(str(message_type))
        if message_type == "authenticate":
            required = {"protocolVersion", "type", "correlationId", "ticket", "connectionId"}
            if set(value) != required:
                errors.append(f"{path.relative_to(ROOT)}: authenticate keys do not match schema")
        elif message_type == "command":
            required = {
                "protocolVersion",
                "type",
                "commandId",
                "matchId",
                "expectedRevision",
                "name",
                "payload",
            }
            if set(value) != required or value.get("name") not in command_enum:
                errors.append(f"{path.relative_to(ROOT)}: invalid command example")
        elif message_type not in event_enum:
            errors.append(f"{path.relative_to(ROOT)}: unknown message type {message_type!r}")
        if message_type in {"matchSnapshot", "stateUpdate", "choiceRequested", "matchFinished"}:
            leaked = FORBIDDEN_PLAYER_KEYS & walk_keys(value.get("payload"))
            if leaked:
                errors.append(f"{path.relative_to(ROOT)}: hidden PlayerView keys {sorted(leaked)}")
        if message_type != "authenticate" and "ticket" in walk_keys(value):
            errors.append(f"{path.relative_to(ROOT)}: ticket is only valid in authenticate")
    if not {"authenticate", "command", "matchSnapshot", "error"} <= seen_types:
        errors.append("contracts/ws/examples: authenticate, command, matchSnapshot and error are required")


def main() -> int:
    errors: list[str] = []
    openapi = load_object(OPENAPI, errors)
    schema = load_object(WS_SCHEMA, errors)

    if openapi.get("openapi") != "3.1.1":
        errors.append("openapi.json: expected OpenAPI 3.1.1")
    info = openapi.get("info")
    if not isinstance(info, dict) or info.get("version") != "1.2":
        errors.append("openapi.json: expected info.version 1.1")
    paths = openapi.get("paths")
    if not isinstance(paths, dict) or set(paths) != EXPECTED_PATHS:
        errors.append(f"openapi.json: expected paths {sorted(EXPECTED_PATHS)}")
    servers = openapi.get("servers")
    if not isinstance(servers, list) or any(
        not isinstance(server, dict)
        or not str(server.get("url", "")).startswith("http://127.0.0.1")
        for server in servers
    ):
        errors.append("openapi.json: committed M1 server must remain explicit loopback")
    session_cookie = (
        openapi.get("components", {}).get("securitySchemes", {}).get("sessionCookie", {})
    )
    if session_cookie.get("in") != "cookie" or session_cookie.get("name") != "response_session":
        errors.append("openapi.json: cookie authentication scheme is missing")

    if schema.get("$schema") != "https://json-schema.org/draft/2020-12/schema":
        errors.append("envelope.schema.json: expected JSON Schema 2020-12")
    definitions = schema.get("$defs")
    if not isinstance(definitions, dict):
        errors.append("envelope.schema.json: missing $defs")
        definitions = {}
    command_enum = set(
        definitions.get("commandBase", {})
        .get("properties", {})
        .get("name", {})
        .get("enum", [])
    )
    if command_enum != EXPECTED_COMMANDS:
        errors.append(f"envelope.schema.json: command names are {sorted(command_enum)}")
    payload_names = command_payload_names(schema)
    if payload_names != EXPECTED_COMMANDS:
        errors.append(f"envelope.schema.json: payload schemas missing for {sorted(EXPECTED_COMMANDS - payload_names)}")
    event_enum = set(
        definitions.get("serverEvent", {})
        .get("properties", {})
        .get("type", {})
        .get("enum", [])
    )
    if event_enum != EXPECTED_EVENTS:
        errors.append(f"envelope.schema.json: event types are {sorted(event_enum)}")
    player_view = definitions.get("playerView", {})
    if player_view.get("additionalProperties") is not False:
        errors.append("envelope.schema.json: PlayerView must be an allowlist")
    leaked_schema_keys = FORBIDDEN_PLAYER_KEYS & walk_keys(player_view)
    if leaked_schema_keys:
        errors.append(f"envelope.schema.json: PlayerView exposes {sorted(leaked_schema_keys)}")

    try:
        typescript = TS_PROTOCOL.read_text(encoding="utf-8")
        go_protocol = GO_PROTOCOL.read_text(encoding="utf-8")
    except OSError as exc:
        errors.append(str(exc))
        typescript = ""
        go_protocol = ""
    if literal_array(typescript, "clientCommandNames") != EXPECTED_COMMANDS:
        errors.append("frontend/src/protocol.ts: client command parity failed")
    if literal_array(typescript, "serverEventTypes") != EXPECTED_EVENTS:
        errors.append("frontend/src/protocol.ts: server event parity failed")
    if 'protocolVersion = "1.2"' not in typescript:
        errors.append("frontend/src/protocol.ts: protocol version parity failed")
    if 'ProtocolVersion = "1.2"' not in go_protocol:
        errors.append("backend protocol.go: protocol version parity failed")

    http_errors = set(
        openapi.get("components", {})
        .get("schemas", {})
        .get("Error", {})
        .get("properties", {})
        .get("code", {})
        .get("enum", [])
    )
    ws_errors = set(
        definitions.get("error", {})
        .get("properties", {})
        .get("code", {})
        .get("enum", [])
    )
    if not http_errors or http_errors != ws_errors:
        errors.append("HTTP and WebSocket error code parity failed")

    validate_examples(schema, errors)

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print("Contract validation passed: OpenAPI, WS schemas/examples and Go/TypeScript parity.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
