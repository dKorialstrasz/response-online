#!/usr/bin/env python3
"""Require an explicit license record for every direct Go runtime module."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
GO_MOD = ROOT / "backend" / "go.mod"
REGISTRY = ROOT / "config" / "dependency-licenses.json"
APPROVED = {"Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "MIT"}


def direct_requirements(source: str) -> dict[str, str]:
    result: dict[str, str] = {}
    block = re.search(r"require\s*\((.*?)\)", source, re.DOTALL)
    lines = block.group(1).splitlines() if block else []
    lines += re.findall(r"^require\s+(\S+)\s+(v\S+)\s*$", source, re.MULTILINE)
    for line in lines:
        if isinstance(line, tuple):
            module, version = line
        else:
            parts = line.split()
            if len(parts) < 2 or "// indirect" in line:
                continue
            module, version = parts[:2]
        result[module] = version
    return result


def main() -> int:
    requirements = direct_requirements(GO_MOD.read_text(encoding="utf-8"))
    registry = json.loads(REGISTRY.read_text(encoding="utf-8"))
    errors: list[str] = []
    if set(requirements) != set(registry):
        errors.append(
            f"license registry mismatch: missing={sorted(set(requirements) - set(registry))}, "
            f"extra={sorted(set(registry) - set(requirements))}"
        )
    for module, version in requirements.items():
        record = registry.get(module, {})
        if record.get("version") != version:
            errors.append(f"{module}: version does not match go.mod")
        if record.get("license") not in APPROVED:
            errors.append(f"{module}: license is not approved")
        if not str(record.get("source", "")).startswith("https://"):
            errors.append(f"{module}: authoritative license source is required")
    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print(f"Dependency license check passed: {len(requirements)} direct Go module(s).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
