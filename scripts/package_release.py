#!/usr/bin/env python3
"""Build a standalone macOS/Linux archive using only Go and Python's stdlib."""

import argparse
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile


def release_member(info):
    """Exclude runtime caches from installable plugin packages."""
    parts = Path(info.name).parts
    if "__pycache__" in parts or Path(info.name).name == ".DS_Store":
        return None
    if info.name.endswith((".pyc", ".pyo")):
        return None
    return info


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, help="release version, e.g. v0.1.0")
    parser.add_argument("--os", choices=["darwin", "linux"], required=True)
    parser.add_argument("--arch", choices=["amd64", "arm64"], required=True)
    parser.add_argument("--output", type=Path, default=Path("dist"))
    args = parser.parse_args()
    version = args.version.removeprefix("v")
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        parser.error("version must be X.Y.Z or vX.Y.Z")

    root = Path(__file__).resolve().parent.parent
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    name = f"sunbook_{version}_{args.os}_{args.arch}"
    archive = output / f"{name}.tar.gz"
    env = dict(os.environ, GOOS=args.os, GOARCH=args.arch, CGO_ENABLED="0")
    with tempfile.TemporaryDirectory(prefix="sunbook-release-") as temp:
        binary = Path(temp) / "sunbook"
        subprocess.run([
            "go", "build", "-trimpath", "-ldflags", f"-s -w -X main.version={version}",
            "-o", str(binary), "./cmd/sunbook",
        ], cwd=root, env=env, check=True)
        binary.chmod(0o755)
        # Complete the archive before replacing a previous local build.
        staging = output / f".{name}.tar.gz.tmp"
        try:
            with tarfile.open(staging, "w:gz") as bundle:
                bundle.add(binary, arcname=f"{name}/sunbook")
                for path in ["README.md", "CHANGELOG.md", "config.example.json", "docs", "examples", "sdk"]:
                    bundle.add(root / path, arcname=f"{name}/{path}", filter=release_member)
            staging.replace(archive)
        finally:
            staging.unlink(missing_ok=True)
    print(archive)


if __name__ == "__main__":
    main()
