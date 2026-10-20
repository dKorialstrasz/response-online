#!/usr/bin/env python3
"""Create the local M1 build manifest from immutable artifacts."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
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


def source_tree_state() -> dict[str, object]:
    diff = subprocess.run(
        ["git", "diff", "--binary", "HEAD"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    ).stdout
    untracked_output = subprocess.run(
        ["git", "ls-files", "--others", "--exclude-standard", "-z"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    ).stdout
    untracked = sorted(item.decode() for item in untracked_output.split(b"\0") if item)
    if not diff and not untracked:
        return {"clean": True}
    digest = hashlib.sha256()
    digest.update(diff)
    for relative in untracked:
        digest.update(relative.encode())
        digest.update(b"\0")
        digest.update((ROOT / relative).read_bytes())
        digest.update(b"\0")
    return {"clean": False, "diffSha256": digest.hexdigest()}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--commit", required=True)
    parser.add_argument("--qa-status", choices=("passed", "not-run"), default="not-run")
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
        "sourceTree": source_tree_state(),
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
            "qa": {
                "status": args.qa_status,
                "commands": ["make check", "make smoke"],
            },
            "ci": {
                "provider": "github-actions" if os.environ.get("GITHUB_ACTIONS") == "true" else "local",
                "workflow": os.environ.get("GITHUB_WORKFLOW", "not-applicable"),
                "runId": os.environ.get("GITHUB_RUN_ID", "not-applicable"),
                "runAttempt": os.environ.get("GITHUB_RUN_ATTEMPT", "not-applicable"),
                "prerequisiteStatus": "passed" if os.environ.get("GITHUB_ACTIONS") == "true" else "not-run",
                "prerequisiteJobs": ["backend", "frontend", "contracts-content", "security", "smoke"],
            },
            "content": "schema and semantic release gate passed by Issue #9",
            "rules": "deterministic core #10, effect pipeline #11, closed catalog #12, server-authoritative playCard #37",
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

