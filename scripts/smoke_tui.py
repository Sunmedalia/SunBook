#!/usr/bin/env python3
"""End-to-end smoke test through a real Unix PTY (no desktop or dependencies)."""
import errno
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import time


def main():
    binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else "./sunbook").resolve())
    with tempfile.TemporaryDirectory(prefix="sunbook-smoke-") as workspace:
        path = Path(workspace) / "notes.txt"
        path.write_text("original\n", encoding="utf-8")
        nested = Path(workspace) / "nested" / "needle.txt"
        nested.parent.mkdir()
        nested.write_text("nested sample", encoding="utf-8")
        config = Path(workspace) / "smoke-config.json"
        config.write_text(json.dumps({"shell": "/bin/sh"}), encoding="utf-8")
        subprocess.run(["git", "init", "-q", workspace], check=True, capture_output=True)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 110, 0, 0))
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor",
                   TCELL_KEYBOARD_PROTOCOL="legacy", TCELL_NEGOTIATE="disable")
        proc = subprocess.Popen([binary, "--config", str(config), str(path)], stdin=slave, stdout=slave,
                                stderr=slave, cwd=workspace, env=env, start_new_session=True,
                                preexec_fn=lambda: fcntl.ioctl(0, termios.TIOCSCTTY, 0))
        os.close(slave)
        received = bytearray()

        def wait_for(predicate, description, timeout=8):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if predicate():
                    return
                readable, _, _ = select.select([master], [], [], .05)
                if readable:
                    try:
                        received.extend(os.read(master, 65536))
                    except OSError as exc:
                        if exc.errno != errno.EIO:
                            raise
                if proc.poll() is not None:
                    raise AssertionError(f"editor exited {proc.returncode} while waiting for {description}: {received[-3000:]!r}")
            raise AssertionError(f"timed out: {description}\n{received[-3000:]!r}")

        def paste(text):
            os.write(master, b"\x1b[200~" + text.encode() + b"\x1b[201~")

        def click(x, y):
            os.write(master, f"\x1b[<0;{x};{y}M\x1b[<0;{x};{y}m".encode())

        try:
            wait_for(lambda: b"SUNBOOK" in received, "first screen")
            # Bracketed paste is one edit and must preserve all UTF-8 graphemes.
            prefix = "你好 👩‍💻\n"
            os.write(master, b"\x1b[200~" + prefix.encode() + b"\x1b[201~\x13")
            expected = prefix + "original\n"
            wait_for(lambda: path.read_text(encoding="utf-8") == expected, "saved Unicode paste")

            # F4 focuses the shell. Ctrl+J and Hide must preserve its session.
            received.clear()
            os.write(master, b"\x1bOS")
            wait_for(lambda: b"[Hide]" in received, "terminal controls")
            paste("SUNBOOK_KEEP=retained; printf 'SHELL_%s\\n' READY\n")
            wait_for(lambda: b"SHELL_READY" in received, "shell command")
            received.clear()
            os.write(master, b"\x0a\x1bOS")  # Ctrl+J hides, F4 opens same session
            paste('printf \'SESSION_%s\\n\' "$SUNBOOK_KEEP"\n')
            wait_for(lambda: b"SESSION_retained" in received, "Ctrl+J preserves shell")
            received.clear()
            click(107, 18)  # Hide in a 110x32 terminal, default 12-row panel
            os.write(master, b"\x1bOS")
            paste('printf \'HIDDEN_%s\\n\' "$SUNBOOK_KEEP"\n')
            wait_for(lambda: b"HIDDEN_retained" in received, "Hide button preserves shell")
            received.clear()
            click(100, 18)  # Kill, then F4 must create a new session
            os.write(master, b"\x1bOS")
            paste('printf \'KILL_%s\\n\' "${SUNBOOK_KEEP-unset}"\n')
            wait_for(lambda: b"KILL_unset" in received, "Kill ends shell session")
            os.write(master, b"\x0a")  # hide panel from shell focus

            # Filter files in an unopened directory, then open a result.
            received.clear()
            click(8, 4)
            paste("needle")
            wait_for(lambda: b"nested/needle.txt" in received, "Explorer filter")
            received.clear()
            os.write(master, b"\r")
            wait_for(lambda: b"nested sample" in received, "filtered file opens")

            # Workspace Search and Git now live in persistent left sidebars.
            received.clear()
            click(3, 6)  # activity S
            paste("nested sample")
            wait_for(lambda: b"1 results" in received, "Search sidebar results")
            received.clear()
            click(3, 9)  # activity G
            wait_for(lambda: b"CONTROL" in received and b"CHANGES" in received, "Git sidebar")
            os.write(master, b"s")  # stage first row: nested/needle.txt
            wait_for(lambda: subprocess.run(["git", "-C", workspace, "ls-files", "--error-unmatch", "--", "nested/needle.txt"],
                                            capture_output=True).returncode == 0, "Git sidebar stages file")
            staged = subprocess.check_output(["git", "-C", workspace, "show", ":nested/needle.txt"])
            if staged != b"nested sample":
                raise AssertionError("Git sidebar staged an unexpected file")
            click(3, 3)  # activity E restores Explorer
            received.clear()
            click(3, 6)  # activity S restores query and results
            wait_for(lambda: b"1 results" in received, "Search state survives activity switches")

            # Use the top New button and save a new buffer through Save As.
            click(13, 1)
            paste("toolbar draft\n")
            received.clear()
            os.write(master, b"\x13")
            wait_for(lambda: b"Save As" in received, "toolbar New and Save As")
            toolbar_path = Path(workspace) / "toolbar.txt"
            paste(str(toolbar_path))
            os.write(master, b"\r")
            wait_for(lambda: toolbar_path.exists() and toolbar_path.read_text() == "toolbar draft\n",
                     "toolbar-created file saves")
            received.clear()
            os.write(master, b"\x1bOP")  # F1: palette
            wait_for(lambda: b"Command Palette" in received, "command palette")
            os.write(master, b"View: Toggle Split Editor\r")
            time.sleep(.1)
            # SGR mouse: click and drag in the first editor, exercising input parsing.
            os.write(master, b"\x1b[<0;37;4M\x1b[<32;41;4M\x1b[<0;41;4m")
            # Resize the host terminal while the split editor is active.
            fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 28, 100, 0, 0))
            received.clear()
            os.write(master, b"\x11")  # Ctrl+Q: clean exit
            deadline = time.monotonic() + 5
            while proc.poll() is None and time.monotonic() < deadline:
                readable, _, _ = select.select([master], [], [], .05)
                if readable:
                    try:
                        os.read(master, 65536)
                    except OSError as exc:
                        if exc.errno != errno.EIO:
                            raise
            if proc.poll() is None:
                raise AssertionError("editor did not restore the terminal and exit")
            if proc.returncode != 0:
                raise AssertionError(f"editor exited {proc.returncode}")
            if path.read_text(encoding="utf-8") != expected:
                raise AssertionError("mouse or resize changed the document")
            print("PTY smoke passed: Unicode save, terminal Hide/Kill, persistent Explorer/Search/Git sidebars, staging, toolbar, split, mouse, resize, exit")
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()
            os.close(master)


if __name__ == "__main__":
    main()
