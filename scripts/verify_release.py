#!/usr/bin/env python3
"""Reject release archives containing anything except one executable binary."""

import argparse
from pathlib import Path
import tarfile


def verify_archive(path):
    with tarfile.open(path, "r:gz") as bundle:
        members = bundle.getmembers()
        if len(members) != 1 or members[0].name != "sunbook":
            raise ValueError(f"{path}: expected only a root-level sunbook binary")
        member = members[0]
        if not member.isfile() or member.mode != 0o755 or member.size < 4:
            raise ValueError(f"{path}: sunbook must be a regular executable (0755)")
        with bundle.extractfile(member) as binary:
            if binary.read(4) not in (b"\x7fELF", b"\xcf\xfa\xed\xfe"):
                raise ValueError(f"{path}: expected an ELF or Mach-O binary")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archives", nargs="+", type=Path)
    for archive in parser.parse_args().archives:
        verify_archive(archive)
        print(f"Verified single-binary archive: {archive}")
