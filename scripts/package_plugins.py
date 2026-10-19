#!/usr/bin/env python3
"""Build reproducible GitHub-hosted plugin packages and a SHA-256 catalog."""
import argparse
import hashlib
import json
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]
REPOSITORY = "https://github.com/Sunmedalia/SunBook-plugins"


def build(output):
    packages = output / "packages"
    packages.mkdir(parents=True, exist_ok=True)
    entries = []
    for source in sorted((ROOT / "examples").iterdir()):
        if not (source / "plugin.json").is_file():
            continue
        manifest = json.loads((source / "plugin.json").read_text())
        filename = f"{manifest['id']}-{manifest['version']}.zip"
        archive = packages / filename
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            for file in sorted(source.rglob("*")):
                if "__pycache__" in file.parts or file.suffix == ".pyc":
                    continue
                if file.is_symlink():
                    raise ValueError(f"Symlinks are not allowed: {file}")
                if not file.is_file():
                    continue
                info = zipfile.ZipInfo(f"{source.name}/{file.relative_to(source).as_posix()}", (2026, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = (0o100644 | (file.stat().st_mode & 0o111)) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                zf.writestr(info, file.read_bytes())
        entries.append({
            **{key: manifest[key] for key in ("id", "name", "version", "description")},
            "author": "Sunmedalia",
            "repository": REPOSITORY,
            "runtime": "Python 3.9+ (python3)",
            "url": f"https://raw.githubusercontent.com/Sunmedalia/SunBook-plugins/main/packages/{filename}",
            "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
        })
    (output / "index.json").write_text(json.dumps({"schemaVersion": 1, "plugins": entries}, ensure_ascii=False, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "marketplace")
    build(parser.parse_args().output)
