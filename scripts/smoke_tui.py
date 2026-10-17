#!/usr/bin/env python3
"""End-to-end smoke test through a real Unix PTY (no desktop or dependencies)."""
from ansi_screen import Screen

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
        screen = Screen()
        screen_generation_at_clear = 0
        probe_tail = b""
        host_colors = {b"10": b"rgb:2020/2020/2020", b"11": b"rgb:f8f8/f8f8/f8f8"}

        def read_output():
            nonlocal probe_tail
            chunk = os.read(master, 65536)
            received.extend(chunk)
            screen.feed(chunk)
            probes = probe_tail + chunk
            for code, color in host_colors.items():
                query = b"\x1b]" + code + b";?\a"
                for _ in range(probes.count(query)):
                    os.write(master, b"\x1b]" + code + b";" + color + b"\x1b\\")
            probe_tail = probes[-6:]

        def clear_output():
            nonlocal screen_generation_at_clear
            received.clear()
            screen_generation_at_clear = screen.generation

        def has_text(token):
            return token in received or (screen.generation > screen_generation_at_clear
                                         and token.decode("utf-8") in screen.text)

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
                                 f"screen: {screen.text!r}\nlast output: {received[-2000:]!r}")

        def paste(text):
            os.write(master, b"\x1b[200~" + text.encode() + b"\x1b[201~")

        def click(x, y):
            os.write(master, f"\x1b[<0;{x};{y}M\x1b[<0;{x};{y}m".encode())

        try:
            wait_for(lambda: has_text(b"SUNBOOK"), "first screen")
            wait_for(lambda: has_text("🐍".encode()) and has_text("🦀".encode()),
                     "Python and Rust use mascot icons in Explorer")
            # Bracketed paste is one edit and must preserve all UTF-8 graphemes.
            prefix = "你好 👩‍💻\n"
            os.write(master, b"\x1b[200~" + prefix.encode() + b"\x1b[201~\x13")
            expected = prefix + "original\n"
            wait_for(lambda: path.read_text(encoding="utf-8") == expected, "saved Unicode paste")

            # F4 focuses the shell. Ctrl+J and Hide must preserve its session.
            clear_output()
            os.write(master, b"\x1bOS")
            wait_for(lambda: has_text("\uf1f8".encode()), "terminal controls")
            paste("SUNBOOK_KEEP=retained; printf 'SHELL_%s\\n' READY\n")
            wait_for(lambda: has_text(b"SHELL_READY"), "shell command")
            clear_output()
            click(61, 18)  # Terminal + creates a separate shell.
            paste("SUNBOOK_SECOND=second; printf 'SECOND_%s\\n' \"${SUNBOOK_KEEP-unset}\"\n")
            wait_for(lambda: has_text(b"SECOND_unset"), "new terminal has independent environment")
            clear_output()
            click(69, 18)  # Session selector.
            wait_for(lambda: has_text("✓".encode()), "terminal dropdown beneath its button")
            click(69, 20)  # First session, directly below the selector.
            paste("printf 'FIRST_%s\\n' \"$SUNBOOK_KEEP\"\n")
            wait_for(lambda: has_text(b"FIRST_retained"), "switching restores original shell")
            clear_output()
            click(69, 18)
            wait_for(lambda: has_text("✓".encode()), "terminal dropdown reopens")
            click(69, 21)  # Second session.
            click(104, 18)  # Kill only the second shell, automatically restoring the first.
            paste("printf 'SURVIVING_%s\\n' \"$SUNBOOK_KEEP\"\n")
            wait_for(lambda: has_text(b"SURVIVING_retained"), "Kill preserves other terminal sessions")
            clear_output()
            click(96, 18)  # Right docking control in the bottom panel header
            wait_for(lambda: has_text("\uf063".encode()), "right dock control")
            paste('printf \'DOCK_%s\\n\' "$SUNBOOK_KEEP"\n')
            wait_for(lambda: has_text(b"DOCK_retained"), "right dock preserves shell session")
            clear_output()
            click(100, 2)  # Bottom docking control in the right panel header
            paste('printf \'BOTTOM_%s\\n\' "$SUNBOOK_KEEP"\n')
            wait_for(lambda: has_text(b"BOTTOM_retained"), "bottom dock preserves shell session")
            clear_output()
            os.write(master, b"\x0a\x1bOS")  # Ctrl+J hides, F4 opens same session
            paste('printf \'SESSION_%s\\n\' "$SUNBOOK_KEEP"\n')
            wait_for(lambda: has_text(b"SESSION_retained"), "Ctrl+J preserves shell")
            clear_output()
            click(107, 18)  # Hide in a 110x32 terminal, default 12-row panel
            os.write(master, b"\x1bOS")
            paste('printf \'HIDDEN_%s\\n\' "$SUNBOOK_KEEP"\n')
            wait_for(lambda: has_text(b"HIDDEN_retained"), "Hide button preserves shell")
            clear_output()
            click(104, 18)  # Kill, then F4 must create a new session
            os.write(master, b"\x1bOS")
            paste('printf \'KILL_%s\\n\' "${SUNBOOK_KEEP-unset}"\n')
            wait_for(lambda: has_text(b"KILL_unset"), "Kill ends shell session")
            os.write(master, b"\x0a")  # hide panel from shell focus

            # Filter files in an unopened directory, preview, then keep a result.
            clear_output()
            click(8, 4)
            paste("needle")
            wait_for(lambda: has_text(b"nested/needle.txt"), "Explorer filter")
            clear_output()
            click(12, 5)
            wait_for(lambda: has_text(b"nested sample") and has_text(b"PREVIEW"),
                     "single click previews the filtered file")
            clear_output()
            click(12, 5)
            click(12, 5)
            wait_for(lambda: has_text(b"EDITOR"), "double click keeps the preview open")

            # Workspace Search and Git now live in persistent left sidebars.
            clear_output()
            click(3, 6)  # activity S
            paste("nested sample")
            wait_for(lambda: has_text(b"1 results"), "Search sidebar results")
            clear_output()
            click(3, 9)  # activity G
            wait_for(lambda: has_text(b"CONTROL") and (has_text(b"CHANGES") or has_text(b"UNTRACKED")), "Git sidebar")
            # Terminal diff rendering may skip unchanged spaces between words.
            wait_for(lambda: has_text(b"GIT LOG") and has_text(b"SMOKE"), "inline Git log")
            # Group headings fold with the mouse and expand with the keyboard.
            def sidebar_has(text):
                return any(text in line[5:31] for line in screen.text.splitlines()[2:30])

            wait_for(lambda: sidebar_has("UNTRACKED") and sidebar_has("library.rs"),
                     "untracked group and file rows")
            untracked_row = next(i for i, line in enumerate(screen.text.splitlines()) if "UNTRACKED" in line[5:31])
            click(12, untracked_row + 1)
            wait_for(lambda: sidebar_has("UNTRACKED") and not sidebar_has("library.rs"),
                     "mouse collapses untracked files but keeps the heading")
            os.write(master, b"\x1b[C")
            wait_for(lambda: sidebar_has("library.rs"), "Right expands the selected group")
            os.write(master, b"c")
            paste("draft subject")
            os.write(master, b"\x0a")  # Ctrl+J is scoped to message newlines.
            paste("body line")
            wait_for(lambda: sidebar_has("draft subject") and sidebar_has("body line"),
                     "multiline commit composer in the real terminal")
            os.write(master, b"\x15\x1b")  # Clear test draft and return to file list.
            wait_for(lambda: " GIT " in screen.text.splitlines()[-1], "leave commit composer")
            # Select by path instead of relying on row order or a selection
            # preserved by an asynchronous Git refresh.
            clear_output()
            os.write(master, b"\x06")  # Ctrl+F: Git path filter
            paste("nested/needle.txt")
            wait_for(lambda: has_text(b"nested/needle.txt"), "Git filter selects one fixture")
            os.write(master, b"\rs")  # focus the filtered list and stage its only file
            wait_for(lambda: subprocess.run(["git", "-C", workspace, "ls-files", "--error-unmatch", "--", "nested/needle.txt"],
                                            capture_output=True).returncode == 0, "Git sidebar stages file")
            staged = subprocess.check_output(["git", "-C", workspace, "show", ":nested/needle.txt"])
            if staged != b"nested sample":
                raise AssertionError("Git sidebar staged an unexpected file")
            clear_output()
            os.write(master, b"d")
            wait_for(lambda: has_text(b"Read-only") and has_text(b"INDEX"),
                     "split comparison in editor")
            click(90, 4)  # right comparison pane
            os.write(master, b"x\x13\x17")  # read-only typing/save, then Ctrl+W closes diff
            if nested.read_text() != "nested sample":
                raise AssertionError("comparison changed the working file")
            # The expanded commit lists its files directly below its metadata.
            clear_output()
            wait_for(lambda: any("history.go" in line[:31] for line in screen.text.splitlines()),
                     "expanded commit file row")
            history_row = next(i for i, line in enumerate(screen.text.splitlines()) if "history.go" in line[:31])
            click(15, history_row + 1)
            wait_for(lambda: has_text(b"EMPTY") and has_text(b"committed"),
                     "Git log file opens committed snapshots")
            os.write(master, b"\x17")  # Ctrl+W closes the historic comparison
            click(3, 3)  # activity E restores Explorer
            clear_output()
            click(3, 6)  # activity S restores query and results
            wait_for(lambda: has_text(b"1 results"), "Search state survives activity switches")

            # Open browses folders; Quick Open reuses the workspace index and jumps.
            clear_output()
            click(23, 1)  # top Open works regardless of sidebar focus
            wait_for(lambda: has_text(b"Open File"), "Open browser")
            os.write(master, b"\x0c")  # Ctrl+L: edit the directory path
            paste(str(nested.parent))
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"needle.txt"), "editable Open directory path")
            clear_output()
            click(12, 6)  # ↑ Up in the Open dialog header (110x32)
            wait_for(lambda: has_text(b"notes.txt") and has_text(b"nested"),
                     "Open parent button")
            os.write(master, b"\x0c")
            paste(str(nested.parent))
            os.write(master, b"\r")
            clear_output()
            wait_for(lambda: has_text(b"needle.txt"), "return to nested directory")
            clear_output()
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"nested sample"), "Open browser opens selected file")
            clear_output()
            os.write(master, b"\x10")  # Ctrl+P
            wait_for(lambda: has_text(b"Quick Open"), "Quick Open picker")
            paste("notes.txt:3:1")
            clear_output()
            os.write(master, b"\r")
            wait_for(lambda: has_text("你好".encode()) or has_text(b"original"),
                     "Quick Open line jump")

            # Help stays at the top right and closes without editing the file.
            clear_output()
            click(107, 1)
            wait_for(lambda: has_text(b"Keyboard Shortcuts") and has_text(b"ctrl+s"),
                     "Help button shows shortcuts")
            clear_output()
            os.write(master, b"\x1b[6~\x1b")  # PageDown, then Escape
            # Let the legacy terminal parser finish Escape before sending a mouse sequence.
            wait_for(lambda: has_text(b"\x1b[?25h"), "Help closes and restores editor cursor")

            # Theme selection applies live and preserves the active config.
            for theme, label in [("github-light", "GitHub Light"), ("auto", "Auto"),
                                 ("terminal-default", "Terminal Default"), ("sunbook", "SunBook")]:
                clear_output()
                click(3, 29)  # Settings gear: Theme is the first entry.
                wait_for(lambda: has_text(b"Settings"), "theme Settings entry")
                clear_output()
                os.write(master, b"\r")
                wait_for(lambda: has_text(b"Gruvbox"), "theme picker lists presets")
                paste(label)
                os.write(master, b"\r")
                wait_for(lambda: json.loads(config.read_text()).get("theme", {}).get("name") == theme,
                         f"persist {theme} theme")
                if theme == "auto":
                    wait_for(lambda: has_text(b"48;2;248;248;248"), "Auto uses queried light terminal background")
                    host_colors[b"10"], host_colors[b"11"] = b"rgb:eeee/eeee/eeee", b"rgb:1212/1212/1212"
                    clear_output()
                    os.write(master, b"\x1b[I")  # Host focus requests an updated palette.
                    wait_for(lambda: has_text(b"48;2;18;18;18"), "Auto follows terminal palette changes")
            if json.loads(config.read_text())["shell"] != "/bin/sh":
                raise AssertionError("theme selection overwrote shell configuration")

            # Settings keeps the original icon set and persists each selection.
            for icon_theme, arrow in [("classic", b"\x1b[B"), ("expressive", b"\x1b[A")]:
                clear_output()
                click(3, 29)  # Bottom-left Settings gear at 110x32.
                wait_for(lambda: has_text(b"Settings"), "Settings opens from activity gear")
                clear_output()
                os.write(master, b"\x1b[B\r")  # File Icons is below Theme.
                wait_for(lambda: has_text(b"File Icon Theme"),
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
            clear_output()
            os.write(master, b"\x13")
            wait_for(lambda: has_text(b"Save As"), "toolbar New and Save As")
            toolbar_path = Path(workspace) / "toolbar.txt"
            paste("toolbar.txt")
            os.write(master, b"\r")
            wait_for(lambda: toolbar_path.exists() and toolbar_path.read_text() == draft,
                     "toolbar-created file saves")
            clear_output()
            click(110, 4)  # one-column scrollbar: jump to the start without editing
            wait_for(lambda: has_text(b"toolbar draft"), "content scrollbar scrolls to the start")
            if toolbar_path.read_text() != draft:
                raise AssertionError("scrollbar changed saved content")
            # Reorder a tab with a real SGR mouse drag; a caret previews insertion.
            clear_output()
            os.write(master, b"\x1b[<0;36;2M\x1b[<32;107;2M")
            wait_for(lambda: has_text("┃".encode()), "tab drag insertion marker")
            os.write(master, b"\x1b[<0;107;2m")
            clear_output()
            os.write(master, b"\x1bOP")  # F1: palette
            wait_for(lambda: has_text(b"Command Palette"), "command palette")
            os.write(master, b"View: Toggle Split Editor\r")
            time.sleep(.1)
            # SGR mouse: click and drag in the first editor, exercising input parsing.
            os.write(master, b"\x1b[<0;37;4M\x1b[<32;41;4M\x1b[<0;41;4m")
            # Branch management uses the same modal/keyboard path as real users.
            def run_command(label):
                clear_output()
                os.write(master, b"\x1bOP")
                wait_for(lambda: has_text(b"Command Palette"), "command palette for " + label)
                paste(label)
                os.write(master, b"\r")

            original_branch = subprocess.run(["git", "-C", workspace, "symbolic-ref", "--short", "HEAD"],
                                             capture_output=True, text=True, check=True).stdout.strip()
            run_command("Git: Manage Branches")
            wait_for(lambda: has_text(b"Create and switch"), "branch manager")
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"New branch name"), "new branch input")
            paste("smoke/branches")
            os.write(master, b"\r")
            wait_for(lambda: subprocess.run(["git", "-C", workspace, "symbolic-ref", "--short", "HEAD"],
                                            capture_output=True, text=True).stdout.strip() == "smoke/branches"
                     and has_text(b"smoke/branches"), "create and switch branch")
            run_command("Git: Manage Branches")
            wait_for(lambda: has_text(b"Create and switch"), "reopen branch manager")
            paste(original_branch)
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"Switch to branch"), "branch actions")
            os.write(master, b"\r")
            wait_for(lambda: subprocess.run(["git", "-C", workspace, "symbolic-ref", "--short", "HEAD"],
                                            capture_output=True, text=True).stdout.strip() == original_branch,
                     "switch back to original branch")

            example = Path(__file__).resolve().parent.parent / "examples" / "text-tools"
            run_command("Plugins: Install Local Plugin")
            wait_for(lambda: has_text(b"Install Plugin Folder"), "plugin folder prompt")
            paste(str(example))
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"Text Tools") and has_text(b"disabled"), "installed plugin is disabled")
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"Enable plugin"), "plugin actions")
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"document.edit"), "plugin permission review")
            os.write(master, b"\r")
            wait_for(lambda: has_text("ready · 1.0.0".encode()), "plugin starts after review")
            run_command("Text Tools: Count Words")
            wait_for(lambda: has_text(b"Document summary") and has_text(b"Words:"), "plugin command output")
            clear_output()
            os.write(master, b"\x1b")
            wait_for(lambda: screen.generation > screen_generation_at_clear and "Document summary" not in screen.text,
                     "plugin output closes before reopening commands")
            run_command("Plugins: Manage Installed Plugins")
            wait_for(lambda: has_text(b"PLUGINS"), "plugin health sidebar")
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"Disable plugin"), "enabled plugin actions")
            os.write(master, b"\r")
            wait_for(lambda: has_text("disabled · 1.0.0".encode()), "disable plugin stops its process")

            os.write(master, b"\r")
            wait_for(lambda: has_text(b"Remove plugin"), "disabled plugin removal action")
            os.write(master, b"\x1b[B\x1b[B\r")
            wait_for(lambda: has_text(b"Remove local plugin files"), "plugin removal confirmation")
            os.write(master, b"\r")
            wait_for(lambda: not (Path(workspace) / "plugins" / "text-tools").exists(),
                     "plugin removal deletes only installed copy")

            # A consistently failing plugin is quarantined while editing remains usable.
            broken = Path(workspace) / "broken-demo"
            broken.mkdir()
            (broken / "plugin.json").write_text(json.dumps({
                "id": "broken-demo", "name": "Broken Demo", "version": "1.0.0", "apiVersion": 1,
                "command": ["python3", "-c", "raise SystemExit(7)"], "permissions": [],
                "commands": [{"id": "run", "title": "Broken Command"}]}))
            run_command("Plugins: Install Local Plugin")
            wait_for(lambda: has_text(b"Install Plugin Folder"), "broken plugin install input")
            paste(str(broken))
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"Broken Demo") and has_text(b"disabled"), "broken plugin installed safely")
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"Enable plugin"), "broken plugin actions")
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"Enable trusted local plugin"), "broken plugin permission review")
            os.write(master, b"\r")
            wait_for(lambda: has_text(b"quarantined"), "crash loop is quarantined", timeout=15)
            run_command("File: New Untitled File")
            paste("editor survives plugin failure")
            os.write(master, b"\x13")
            wait_for(lambda: has_text(b"Save As"), "saving works after plugin quarantine")
            recovery = Path(workspace) / "after-plugin-failure.txt"
            paste(str(recovery))
            os.write(master, b"\r")
            wait_for(lambda: recovery.exists() and recovery.read_text() == "editor survives plugin failure",
                     "core editing is available after plugin failure")

            # Resize the host terminal while the split editor is active.
            fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 28, 100, 0, 0))
            clear_output()
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
            print("PTY smoke passed: Unicode save, terminal +/session switching/Hide/Kill, persistent Explorer/Search/Git sidebars, single-click preview, double-click keep, collapsible Git groups, multiline commit composer, staging, Git log file tree and historic comparison, Open browser, Quick Open, Help, themes/Auto color detection/terminal defaults, Settings/Classic/Expressive icon persistence, branch creation/switching, plugin install/permission review/commands/disable/remove/quarantine recovery, content scrollbar, toolbar, dock switching, tab drag, Split, mouse, resize, exit")
        finally:
            # Closing the PTY master first also releases a macOS child that is
            # exiting while its tty output is blocked after a failed assertion.
            os.close(master)
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=5)


if __name__ == "__main__":
    main()
