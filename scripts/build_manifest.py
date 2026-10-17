#!/usr/bin/env python3
"""Create the local M1 build manifest from immutable artifacts."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def sha256_tree(directory: Path) -> str:
    digest = hashlib.sha256()
    for path in sorted(item for item in directory.rglob("*") if item.is_file()):
        digest.update(path.relative_to(directory).as_posix().encode())
        digest.update(b"\0")
        digest.update(path.read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--commit", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    artifacts = ROOT / "build" / "artifacts"
    backend = artifacts / "response-server"
    frontend = artifacts / "frontend.tar.gz"
    for required in (backend, frontend):
        if not required.is_file():
            parser.error(f"missing build artifact: {required.relative_to(ROOT)}")

    release = json.loads(
        (ROOT / "content" / "releases" / "m1-local.json").read_text(encoding="utf-8")
    )
    release_versions = release["versions"]

    manifest = {
        "schemaVersion": 1,
        "commit": args.commit,
        "versions": {
            "protocol": release_versions["protocol"],
            "content": release_versions["content"],
            "deckManifests": release_versions["deckManifests"],
            "effectSpec": release_versions["effectSpec"],
            "rules": release_versions["rules"],
            "engine": release_versions["engine"],
        },
        "artifacts": {
            "backend": {"path": "artifacts/response-server", "sha256": sha256_file(backend)},
            "frontend": {"path": "artifacts/frontend.tar.gz", "sha256": sha256_file(frontend)},
            "contracts": {"path": "../contracts", "sha256": sha256_tree(ROOT / "contracts")},
            "content": {"path": "../content", "sha256": sha256_tree(ROOT / "content")},
            "contentRelease": {
                "path": "../content/releases/m1-local.json",
                "sha256": sha256_file(ROOT / "content" / "releases" / "m1-local.json"),
            },
        },
        "verification": {
            "content": "schema and semantic release gate passed by Issue #9",
            "rules": "not ready until Issues #10-#12",
            "publicDeployment": "out of scope",
            "gamePix": "out of scope",
        },
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"Build manifest written to {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

