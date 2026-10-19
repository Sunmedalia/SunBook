#!/usr/bin/env python3
"""Opt-in live GitHub store smoke test through a disposable real terminal."""
import argparse
import errno
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import tempfile
import termios
import time

from ansi_screen import Screen


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", nargs="?", default="./sunbook")
    parser.add_argument("--gh-proxy", action="store_true", help="toggle and exercise gh-proxy downloads")
    args = parser.parse_args()
    binary = str(Path(args.binary).resolve())
    with tempfile.TemporaryDirectory(prefix="sunbook-store-smoke-") as workspace:
        config = Path(workspace) / "config.json"
        config.write_text(json.dumps({"shell": "/bin/sh"}))
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 38, 140, 0, 0))
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor",
                   TCELL_KEYBOARD_PROTOCOL="legacy", TCELL_NEGOTIATE="disable")
        proc = subprocess.Popen([binary, "--config", str(config), workspace],
                                stdin=slave, stdout=slave, stderr=slave,
                                cwd=workspace, env=env, start_new_session=True,
                                preexec_fn=lambda: fcntl.ioctl(0, termios.TIOCSCTTY, 0))
        os.close(slave)
        screen = Screen()
        tail = b""

        def wait_for(predicate, description, timeout=40):
            nonlocal tail
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if predicate():
                    print("Passed:", description, flush=True)
                    return
                ready, _, _ = select.select([master], [], [], .05)
                if ready:
                    chunk = os.read(master, 65536)
                    screen.feed(chunk)
                    probes = tail + chunk
                    for code in (b"10", b"11"):
                        query = b"\x1b]" + code + b";?\a"
                        for _ in range(probes.count(query)):
                            os.write(master, b"\x1b]" + code + b";rgb:2020/2020/2020\x1b\\")
                    tail = probes[-6:]
                if proc.poll() is not None:
                    raise AssertionError(f"Editor exited while waiting for {description}")
            print(f"Timed out: {description}\n{screen.text}", flush=True)
            raise AssertionError(f"Timed out: {description}")

        def paste(text):
            os.write(master, b"\x1b[200~" + text.encode() + b"\x1b[201~")

        def command(label):
            os.write(master, b"\x1bOP")  # F1
            wait_for(lambda: "Command Palette" in screen.text, "command palette")
            paste(label)
            wait_for(lambda: label in screen.text, label)
            os.write(master, b"\r")

        try:
            wait_for(lambda: "SUNBOOK" in screen.text, "first screen")
            command("Plugins: Browse Store")
            wait_for(lambda: "Use gh-proxy for downloads" in screen.text, "proxy checkbox before catalog loading")
            if args.gh_proxy:
                paste("gh-proxy")
                wait_for(lambda: "gh-proxy" in screen.text and "Markdown Tools" not in screen.text, "find proxy checkbox")
                os.write(master, b"\r")
                wait_for(lambda: "[x] Use gh-proxy" in screen.text, "proxy enabled")
                assert json.loads(config.read_text())["pluginUseGHProxy"]
            wait_for(lambda: "Plugin Store" in screen.text and "Markdown Tools" in screen.text, "live GitHub catalog")
            paste("Markdown")
            wait_for(lambda: "Markdown Tools" in screen.text and "Text Tools" not in screen.text, "filtered catalog")
            os.write(master, b"\r")
            wait_for(lambda: "Install from GitHub" in screen.text and "Python 3.9+" in screen.text, "plugin details")
            os.write(master, b"\r")
            target = Path(workspace) / "plugins/markdown-tools/plugin.json"
            wait_for(lambda: target.exists() and "disabled" in screen.text, "verified disabled installation")
            manifest = json.loads(target.read_text())
            assert manifest["version"] == "1.2.0", manifest
            assert not json.loads(config.read_text())["plugins"]["markdown-tools"]["enabled"]
            assert (target.parent / "main.py").is_file()
            command("Plugins: Browse Store")
            wait_for(lambda: "Plugin Store" in screen.text and "Installed 1.2.0" in screen.text, "installed state in store")
            os.write(master, b"\x1b")
            wait_for(lambda: "Plugin Store" not in screen.text, "close store")
            os.write(master, b"\x11")
            # Drain terminal restoration output so PTY backpressure cannot
            # block exit on macOS.
            deadline = time.monotonic() + 8
            while proc.poll() is None and time.monotonic() < deadline:
                ready, _, _ = select.select([master], [], [], .05)
                if ready:
                    try:
                        os.read(master, 65536)
                    except OSError as exc:
                        if exc.errno != errno.EIO:
                            raise
            proc.wait(timeout=1)
            assert proc.returncode == 0
            print("Live GitHub store PTY passed: catalog, search, metadata, verified download, disabled installation, installed state, exit")
        finally:
            os.close(master)
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=5)


if __name__ == "__main__":
    main()
