#!/usr/bin/env python3
"""Build a standalone macOS/Linux archive using only Go and Python's stdlib."""

import argparse
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

from verify_release import verify_archive


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, help="release version, e.g. v0.3.0")
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
                bundle.add(binary, arcname="sunbook")
            verify_archive(staging)
            staging.replace(archive)
        finally:
            staging.unlink(missing_ok=True)
    print(archive)


if __name__ == "__main__":
    main()
