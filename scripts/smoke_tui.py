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
        (Path(workspace) / "language.py").write_text("pass\n", encoding="utf-8")
        (Path(workspace) / "library.rs").write_text("fn main() {}\n", encoding="utf-8")
        config = Path(workspace) / "smoke-config.json"
        config.write_text(json.dumps({"shell": "/bin/sh"}), encoding="utf-8")
        history = Path(workspace) / "history.go"
        history.write_text('package history\nvar value = "committed"\n', encoding="utf-8")
        subprocess.run(["git", "init", "-q", workspace], check=True, capture_output=True)
        subprocess.run(["git", "-C", workspace, "add", "--", "history.go"], check=True, capture_output=True)
        subprocess.run(["git", "-C", workspace, "-c", "user.name=SunBook Test",
                        "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false",
                        "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "SMOKE LOG"],
                       check=True, capture_output=True)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 110, 0, 0))
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor",
                   NO_COLOR="", TCELL_TRUECOLOR="enable",
                   TCELL_KEYBOARD_PROTOCOL="legacy", TCELL_NEGOTIATE="disable")
        proc = subprocess.Popen([binary, "--config", str(config), str(path)], stdin=slave, stdout=slave,
                                stderr=slave, cwd=workspace, env=env, start_new_session=True,
                                preexec_fn=lambda: fcntl.ioctl(0, termios.TIOCSCTTY, 0))
        os.close(slave)
        received = bytearray()
        probe_tail = b""
        host_colors = {b"10": b"rgb:2020/2020/2020", b"11": b"rgb:f8f8/f8f8/f8f8"}

        def read_output():
            nonlocal probe_tail
            chunk = os.read(master, 65536)
            received.extend(chunk)
            probes = probe_tail + chunk
            for code, color in host_colors.items():
                query = b"\x1b]" + code + b";?\a"
                for _ in range(probes.count(query)):
                    os.write(master, b"\x1b]" + code + b";" + color + b"\x1b\\")
            probe_tail = probes[-6:]

        def wait_for(predicate, description, timeout=8):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if predicate():
                    return
                readable, _, _ = select.select([master], [], [], .05)
                if readable:
                    try:
                        read_output()
                    except OSError as exc:
                        if exc.errno != errno.EIO:
                            raise
                if proc.poll() is not None:
                    raise AssertionError(f"editor exited {proc.returncode} while waiting for {description}: {received[-3000:]!r}")
            status = subprocess.run(["git", "-C", workspace, "status", "--porcelain=v1"],
                                    capture_output=True, text=True)
            raise AssertionError(f"timed out: {description}\ngit status: {status.stdout!r}\n"
                                 f"initial output: {received[:5000]!r}\nlast output: {received[-2000:]!r}")

        def paste(text):
            os.write(master, b"\x1b[200~" + text.encode() + b"\x1b[201~")

        def click(x, y):
            os.write(master, f"\x1b[<0;{x};{y}M\x1b[<0;{x};{y}m".encode())

        try:
            wait_for(lambda: b"SUNBOOK" in received, "first screen")
            wait_for(lambda: "🐍".encode() in received and "🦀".encode() in received,
                     "Python and Rust use mascot icons in Explorer")
            # Bracketed paste is one edit and must preserve all UTF-8 graphemes.
            prefix = "你好 👩‍💻\n"
            os.write(master, b"\x1b[200~" + prefix.encode() + b"\x1b[201~\x13")
            expected = prefix + "original\n"
            wait_for(lambda: path.read_text(encoding="utf-8") == expected, "saved Unicode paste")

            # F4 focuses the shell. Ctrl+J and Hide must preserve its session.
            received.clear()
            os.write(master, b"\x1bOS")
            wait_for(lambda: "\uf1f8".encode() in received, "terminal controls")
            paste("SUNBOOK_KEEP=retained; printf 'SHELL_%s\\n' READY\n")
            wait_for(lambda: b"SHELL_READY" in received, "shell command")
            received.clear()
            click(61, 18)  # Terminal + creates a separate shell.
            paste("SUNBOOK_SECOND=second; printf 'SECOND_%s\\n' \"${SUNBOOK_KEEP-unset}\"\n")
            wait_for(lambda: b"SECOND_unset" in received, "new terminal has independent environment")
            received.clear()
            click(69, 18)  # Session selector.
            wait_for(lambda: "✓".encode() in received, "terminal dropdown beneath its button")
            click(69, 20)  # First session, directly below the selector.
            paste("printf 'FIRST_%s\\n' \"$SUNBOOK_KEEP\"\n")
            wait_for(lambda: b"FIRST_retained" in received, "switching restores original shell")
            received.clear()
            click(69, 18)
            wait_for(lambda: "✓".encode() in received, "terminal dropdown reopens")
            click(69, 21)  # Second session.
            click(104, 18)  # Kill only the second shell, automatically restoring the first.
            paste("printf 'SURVIVING_%s\\n' \"$SUNBOOK_KEEP\"\n")
            wait_for(lambda: b"SURVIVING_retained" in received, "Kill preserves other terminal sessions")
            received.clear()
            click(96, 18)  # Right docking control in the bottom panel header
            wait_for(lambda: "\uf063".encode() in received, "right dock control")
            paste('printf \'DOCK_%s\\n\' "$SUNBOOK_KEEP"\n')
            wait_for(lambda: b"DOCK_retained" in received, "right dock preserves shell session")
            received.clear()
            click(100, 2)  # Bottom docking control in the right panel header
            paste('printf \'BOTTOM_%s\\n\' "$SUNBOOK_KEEP"\n')
            wait_for(lambda: b"BOTTOM_retained" in received, "bottom dock preserves shell session")
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
            click(104, 18)  # Kill, then F4 must create a new session
            os.write(master, b"\x1bOS")
            paste('printf \'KILL_%s\\n\' "${SUNBOOK_KEEP-unset}"\n')
            wait_for(lambda: b"KILL_unset" in received, "Kill ends shell session")
            os.write(master, b"\x0a")  # hide panel from shell focus

            # Filter files in an unopened directory, preview, then keep a result.
            received.clear()
            click(8, 4)
            paste("needle")
            wait_for(lambda: b"nested/needle.txt" in received, "Explorer filter")
            received.clear()
            click(12, 5)
            wait_for(lambda: b"nested sample" in received and b"PREVIEW" in received,
                     "single click previews the filtered file")
            received.clear()
            click(12, 5)
            click(12, 5)
            wait_for(lambda: b"EDITOR" in received, "double click keeps the preview open")

            # Workspace Search and Git now live in persistent left sidebars.
            received.clear()
            click(3, 6)  # activity S
            paste("nested sample")
            wait_for(lambda: b"1 results" in received, "Search sidebar results")
            received.clear()
            click(3, 9)  # activity G
            wait_for(lambda: b"CONTROL" in received and (b"CHANGES" in received or b"UNTRACKED" in received), "Git sidebar")
            # Terminal diff rendering may skip unchanged spaces between words.
            wait_for(lambda: b"GIT LOG" in received and b"SMOKE" in received, "inline Git log")
            # Select by path instead of relying on row order or a selection
            # preserved by an asynchronous Git refresh.
            received.clear()
            os.write(master, b"\x06")  # Ctrl+F: Git path filter
            paste("nested/needle.txt")
            wait_for(lambda: b"nested/needle.txt" in received, "Git filter selects one fixture")
            os.write(master, b"\rs")  # focus the filtered list and stage its only file
            wait_for(lambda: subprocess.run(["git", "-C", workspace, "ls-files", "--error-unmatch", "--", "nested/needle.txt"],
                                            capture_output=True).returncode == 0, "Git sidebar stages file")
            staged = subprocess.check_output(["git", "-C", workspace, "show", ":nested/needle.txt"])
            if staged != b"nested sample":
                raise AssertionError("Git sidebar staged an unexpected file")
            received.clear()
            os.write(master, b"d")
            wait_for(lambda: b"Read-only" in received and b"INDEX" in received,
                     "split comparison in editor")
            click(90, 4)  # right comparison pane
            os.write(master, b"x\x13\x17")  # read-only typing/save, then Ctrl+W closes diff
            if nested.read_text() != "nested sample":
                raise AssertionError("comparison changed the working file")
            # The expanded commit lists its files directly below its metadata.
            received.clear()
            click(15, 27)  # history.go beneath the first commit in the log
            wait_for(lambda: b"EMPTY" in received and b"committed" in received,
                     "Git log file opens committed snapshots")
            os.write(master, b"\x17")  # Ctrl+W closes the historic comparison
            click(3, 3)  # activity E restores Explorer
            received.clear()
            click(3, 6)  # activity S restores query and results
            wait_for(lambda: b"1 results" in received, "Search state survives activity switches")

            # Open browses folders; Quick Open reuses the workspace index and jumps.
            received.clear()
            click(23, 1)  # top Open works regardless of sidebar focus
            wait_for(lambda: b"Open File" in received, "Open browser")
            os.write(master, b"\x0c")  # Ctrl+L: edit the directory path
            paste(str(nested.parent))
            os.write(master, b"\r")
            wait_for(lambda: b"needle.txt" in received, "editable Open directory path")
            received.clear()
            click(12, 6)  # ↑ Up in the Open dialog header (110x32)
            wait_for(lambda: b"notes.txt" in received and b"nested" in received,
                     "Open parent button")
            os.write(master, b"\x0c")
            paste(str(nested.parent))
            os.write(master, b"\r")
            received.clear()
            wait_for(lambda: b"needle.txt" in received, "return to nested directory")
            received.clear()
            os.write(master, b"\r")
            wait_for(lambda: b"nested sample" in received, "Open browser opens selected file")
            received.clear()
            os.write(master, b"\x10")  # Ctrl+P
            wait_for(lambda: b"Quick Open" in received, "Quick Open picker")
            paste("notes.txt:3:1")
            received.clear()
            os.write(master, b"\r")
            wait_for(lambda: "你好".encode() in received or b"original" in received,
                     "Quick Open line jump")

            # Help stays at the top right and closes without editing the file.
            received.clear()
            click(107, 1)
            wait_for(lambda: b"Keyboard Shortcuts" in received and b"ctrl+s" in received,
                     "Help button shows shortcuts")
            received.clear()
            os.write(master, b"\x1b[6~\x1b")  # PageDown, then Escape
            # Let the legacy terminal parser finish Escape before sending a mouse sequence.
            wait_for(lambda: b"\x1b[?25h" in received, "Help closes and restores editor cursor")

            # Theme selection applies live and preserves the active config.
            for theme, label in [("github-light", "GitHub Light"), ("auto", "Auto"),
                                 ("terminal-default", "Terminal Default"), ("sunbook", "SunBook")]:
                received.clear()
                os.write(master, b"\x1bOP")
                wait_for(lambda: b"Command Palette" in received, "theme command palette")
                os.write(master, b"Preferences: Color Theme\r")
                received.clear()
                wait_for(lambda: b"Gruvbox" in received, "theme picker lists presets")
                paste(label)
                os.write(master, b"\r")
                wait_for(lambda: json.loads(config.read_text()).get("theme", {}).get("name") == theme,
                         f"persist {theme} theme")
                if theme == "auto":
                    wait_for(lambda: b"48;2;248;248;248" in received, "Auto uses queried light terminal background")
                    host_colors[b"10"], host_colors[b"11"] = b"rgb:eeee/eeee/eeee", b"rgb:1212/1212/1212"
                    received.clear()
                    os.write(master, b"\x1b[I")  # Host focus requests an updated palette.
                    wait_for(lambda: b"48;2;18;18;18" in received, "Auto follows terminal palette changes")
            if json.loads(config.read_text())["shell"] != "/bin/sh":
                raise AssertionError("theme selection overwrote shell configuration")

            # Settings keeps the original icon set and persists each selection.
            for icon_theme, arrow in [("classic", b"\x1b[B"), ("expressive", b"\x1b[A")]:
                received.clear()
                click(3, 29)  # Bottom-left Settings gear at 110x32.
                wait_for(lambda: b"File Icons" in received, "Settings opens from activity gear")
                received.clear()
                os.write(master, b"\r")
                wait_for(lambda: b"File Icon Theme" in received,
                         "file icon picker retains original icons")
                os.write(master, arrow + b"\r")
                wait_for(lambda: json.loads(config.read_text()).get("fileIcons") == icon_theme,
                         f"Settings persists {icon_theme} icons")
            saved_icons = json.loads(config.read_text())
            if saved_icons.get("activityIcons") != "nerd" or saved_icons.get("theme", {}).get("name") != "sunbook":
                raise AssertionError("file icons overwrote theme or button icons")

            # New uses one optional filename input; blank creates an untitled draft.
            click(13, 1)
            paste("created-from-new.txt")
            os.write(master, b"\r")
            named_path = nested.parent / "created-from-new.txt"
            wait_for(lambda: named_path.exists() and named_path.read_text() == "",
                     "New creates a file in the previously selected Explorer directory")
            os.write(master, b"\x1bOP")
            os.write(master, b"File: Delete Current File\r\r")
            wait_for(lambda: not named_path.exists(), "Delete confirms and removes the current file")
            if nested.read_text() != "nested sample":
                raise AssertionError("Delete affected a neighboring file")
            click(3, 3)  # Search was active; switch to Explorer.
            click(9, 3)  # Workspace heading resets the creation directory.
            click(13, 1)
            os.write(master, b"\r")  # Empty filename creates an untitled file.
            draft = "toolbar draft\n" + "".join(f"scroll {i}\n" for i in range(80))
            paste(draft)
            received.clear()
            os.write(master, b"\x13")
            wait_for(lambda: b"Save As" in received, "toolbar New and Save As")
            toolbar_path = Path(workspace) / "toolbar.txt"
            paste("toolbar.txt")
            os.write(master, b"\r")
            wait_for(lambda: toolbar_path.exists() and toolbar_path.read_text() == draft,
                     "toolbar-created file saves")
            received.clear()
            click(110, 4)  # one-column scrollbar: jump to the start without editing
            wait_for(lambda: b"toolbar draft" in received, "content scrollbar scrolls to the start")
            if toolbar_path.read_text() != draft:
                raise AssertionError("scrollbar changed saved content")
            # Reorder a tab with a real SGR mouse drag; a caret previews insertion.
            received.clear()
            os.write(master, b"\x1b[<0;36;2M\x1b[<32;107;2M")
            wait_for(lambda: "┃".encode() in received, "tab drag insertion marker")
            os.write(master, b"\x1b[<0;107;2m")
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
            print("PTY smoke passed: Unicode save, terminal +/session switching/Hide/Kill, persistent Explorer/Search/Git sidebars, single-click preview, double-click keep, staging, Git log file tree and historic comparison, Open browser, Quick Open, Help, themes/Auto color detection/terminal defaults, Settings/Classic/Expressive icon persistence, content scrollbar, toolbar, dock switching, tab drag, Split, mouse, resize, exit")
        finally:
            # Closing the PTY master first also releases a macOS child that is
            # exiting while its tty output is blocked after a failed assertion.
            os.close(master)
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=5)


if __name__ == "__main__":
    main()
