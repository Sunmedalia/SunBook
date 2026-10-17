#!/usr/bin/env python3
"""Install and exercise Markdown Tools in a real SunBook PTY.

The default configuration is disposable. --config explicitly installs/enables
the plugin persistently while preserving all other configuration fields.
"""

import argparse
import errno
import fcntl
import json
import os
import pty
import select
import shutil
import struct
import subprocess
import tempfile
import termios
import time
from pathlib import Path

from ansi_screen import Screen


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", nargs="?", default="./sunbook")
    parser.add_argument(
        "--config", help="install and leave enabled in this persistent config"
    )
    parser.add_argument(
        "--replace",
        action="store_true",
        help="back up and replace an existing Markdown Tools package using editor actions",
    )
    args = parser.parse_args()
    binary = str(Path(args.binary).resolve())
    source = Path(__file__).resolve().parent.parent / "examples/markdown-tools"
    with tempfile.TemporaryDirectory(prefix="sunbook-markdown-") as workspace:
        config = (
            Path(args.config).absolute()
            if args.config
            else Path(workspace) / "config.json"
        )
        target = config.parent / "plugins/markdown-tools"
        replacing = target.exists()
        if target.is_symlink() or replacing and not args.replace:
            raise RuntimeError(
                "Markdown Tools is already installed; this test never overwrites installed plugins"
            )
        before = (json.loads(config.read_text()) or {}) if config.exists() else {}
        backup = None
        if replacing:
            backup_root = config.parent / "plugin-backups"
            backup_root.mkdir(exist_ok=True)
            backup = (
                Path(tempfile.mkdtemp(prefix="markdown-tools-", dir=backup_root))
                / "markdown-tools"
            )
            shutil.copytree(target, backup, symlinks=True)
        notes = Path(workspace) / "markdown-notes.md"
        original = "#   项目计划 👩‍💻   ###\n\n##   准备\n- [ ] 编写文档\n- [x] 测试功能\n\n###   发布\n"
        notes.write_text(original, encoding="utf-8")
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 110, 0, 0))
        env = dict(
            os.environ,
            TERM="xterm-256color",
            COLORTERM="truecolor",
            NO_COLOR="",
            TCELL_TRUECOLOR="enable",
            TCELL_KEYBOARD_PROTOCOL="legacy",
            TCELL_NEGOTIATE="disable",
        )
        proc = subprocess.Popen(
            [binary, "--config", str(config), str(notes)],
            stdin=slave,
            stdout=slave,
            stderr=slave,
            cwd=workspace,
            env=env,
            start_new_session=True,
            preexec_fn=lambda: fcntl.ioctl(0, termios.TIOCSCTTY, 0),  # noqa: PLW1509 - single-threaded PTY harness
        )
        os.close(slave)
        screen, tail = Screen(), b""

        def read():
            nonlocal tail
            chunk = os.read(master, 65536)
            screen.feed(chunk)
            queries = tail + chunk
            for code, color in (
                (b"10", b"rgb:2020/2020/2020"),
                (b"11", b"rgb:f8f8/f8f8/f8f8"),
            ):
                for _ in range(queries.count(b"\x1b]" + code + b";?\a")):
                    os.write(master, b"\x1b]" + code + b";" + color + b"\x1b\\")
            tail = queries[-6:]

        def wait(predicate, description, timeout=8):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if predicate():
                    return
                ready, _, _ = select.select([master], [], [], 0.05)
                if ready:
                    try:
                        read()
                    except OSError as exc:
                        if exc.errno != errno.EIO:
                            raise
                if proc.poll() is not None:
                    if predicate():
                        return
                    raise AssertionError(
                        f"editor exited during {description}: {proc.returncode}"
                    )
            raise AssertionError(f"timed out: {description}\n{screen.text}")

        def shown(text):
            return text in screen.text

        def paste(text):
            os.write(master, b"\x1b[200~" + text.encode("utf-8") + b"\x1b[201~")

        def command(label):
            os.write(master, b"\x1bOP")
            wait(lambda: shown("Command Palette"), "command palette")
            paste(label)
            os.write(master, b"\r")
            wait(lambda: not shown("Command Palette"), label)

        try:
            wait(lambda: shown("SUNBOOK"), "editor startup")
            if replacing:
                command("Plugins: Manage Installed Plugins")
                wait(lambda: shown("Markdown Tools"), "existing plugin discovery")
                ids = {
                    p.name
                    for p in target.parent.iterdir()
                    if p.is_dir() and not p.is_symlink() and not p.name.startswith(".")
                }
                ids.update(
                    key
                    for key, pref in (before.get("plugins") or {}).items()
                    if pref.get("enabled")
                )
                index = sorted(ids).index("markdown-tools")
                os.write(master, b"\x1b[H" + b"\x1b[B" * index + b"\r")
                wait(lambda: shown("Health and logs"), "existing plugin actions")
                if shown("Disable plugin") or shown("Disable configured plugin"):
                    os.write(master, b"\r")
                    wait(lambda: shown("disabled ·"), "disable previous plugin")
                    os.write(master, b"\r")
                    wait(lambda: shown("Remove plugin"), "disabled plugin actions")
                os.write(master, b"\x1b[B\x1b[B\r")
                wait(
                    lambda: shown("Remove local plugin files"),
                    "replace previous package confirmation",
                )
                os.write(master, b"\r")
                wait(lambda: not target.exists(), "previous package removed")
            command("Plugins: Install Local Plugin")
            wait(lambda: shown("Install Plugin Folder"), "install input")
            paste(str(source))
            os.write(master, b"\r")
            wait(
                lambda: shown("Markdown Tools") and shown("disabled"),
                "installed disabled plugin",
            )
            os.write(master, b"\r")
            wait(lambda: shown("Enable plugin"), "plugin actions")
            os.write(master, b"\r")
            wait(lambda: shown("document.edit"), "permission review")
            os.write(master, b"\r")
            wait(lambda: shown("ready · 1.2.0"), "sandboxed plugin readiness")
            command("Markdown Tools: Format Markdown in Split Preview")
            wait(lambda: shown("Live preview"), "live preview beside original file")

            def right_shown(text):
                starts = [
                    x
                    for (y, x), char in screen.cells.items()
                    if y == 2
                    and char == "L"
                    and "".join(
                        screen.cells.get((2, col), " ") for col in range(x, x + 12)
                    )
                    == "Live preview"
                ]
                return bool(starts) and any(
                    text
                    in "".join(
                        screen.cells.get((y, x), " ") for x in range(min(starts), 110)
                    )
                    for y in range(4, 30)
                )

            wait(
                lambda: right_shown("☑ 测试功能"),
                "rendered task checkbox in right pane",
            )
            if shown("Diff ·") or shown("Markdown ·"):
                raise AssertionError("live preview added a separate comparison tab")
            if notes.read_text() != original:
                raise AssertionError("opening preview modified the source file")
            # Left input remains ordinary input; unsaved edits render on the right.
            command("Edit: Select All")
            paste(original + "\n## 实时更新\n未保存的预览内容\n")
            wait(
                lambda: right_shown("未保存的预览内容"), "live render of unsaved input"
            )
            if notes.read_text() != original:
                raise AssertionError("live rendering saved the source unexpectedly")
            os.write(master, b"\x1a")
            wait(
                lambda: not right_shown("未保存的预览内容"),
                "undo refreshes live preview",
            )
            command("Plugins: Toggle Preview Source / Reading")
            wait(lambda: shown("Markdown source"), "current Markdown source mode")
            command("Plugins: Toggle Preview Source / Reading")
            wait(lambda: shown("Live preview"), "reading mode restored")
            command("Plugins: Format Preview Document")
            formatted = "# 项目计划 👩‍💻\n\n## 准备\n\n- [ ] 编写文档\n- [x] 测试功能\n\n### 发布\n"
            wait(lambda: shown("Markdown formatted"), "fresh formatting applied")
            os.write(master, b"\x13")
            wait(lambda: notes.read_text() == formatted, "explicit formatting saved")
            if not shown("Live preview"):
                raise AssertionError("formatting closed the live preview")
            os.write(master, b"\x1a\x13")
            wait(
                lambda: notes.read_text() == original,
                "formatting undoes in one transaction",
            )
            command("Plugins: Close Formatting Preview")
            wait(lambda: not shown("Live preview"), "close right preview")
            if notes.read_text() != original:
                raise AssertionError("closing preview changed the document")
            command("Markdown Tools: Update Table of Contents")
            wait(lambda: shown("Apply Markdown Tools edits?"), "contents preview")
            if notes.read_text() != original:
                raise AssertionError("plugin saved before preview acceptance")
            os.write(master, b"\r\x13")
            wait(
                lambda: "- [准备](#准备)" in notes.read_text(),
                "Unicode contents saved after acceptance",
            )
            updated = notes.read_text()
            command("Markdown Tools: Update Table of Contents")
            wait(lambda: shown("already up to date"), "idempotent contents update")
            if notes.read_text() != updated:
                raise AssertionError("repeated update duplicated contents")
            os.write(master, b"\x1a\x13")
            wait(lambda: notes.read_text() == original, "contents undo in one step")
            command("Markdown Tools: Show Task Progress")
            wait(
                lambda: (
                    shown("Markdown task progress") and shown("Completed: 1/2 (50%)")
                ),
                "task progress report",
            )
            os.write(master, b"\x1b")
            wait(lambda: not shown("Markdown task progress"), "report closes")
            command("Edit: Select All")
            command("Markdown Tools: Toggle Selected Tasks")
            wait(lambda: shown("Apply Markdown Tools edits?"), "checkbox preview")
            os.write(master, b"\r\x13")
            toggled = original.replace("[ ] 编写", "[x] 编写").replace(
                "[x] 测试", "[ ] 测试"
            )
            wait(
                lambda: notes.read_text() == toggled,
                "checkbox edits saved at UTF-8 positions",
            )
            os.write(master, b"\x1a\x13")
            wait(lambda: notes.read_text() == original, "checkbox edits undo together")
            after = json.loads(config.read_text())
            if not after["plugins"]["markdown-tools"]["enabled"]:
                raise AssertionError("plugin enablement was not persisted")
            if {k: v for k, v in after.items() if k != "plugins"} != {
                k: v for k, v in before.items() if k != "plugins"
            }:
                raise AssertionError("plugin installation changed unrelated settings")
            if {k: v for k, v in after["plugins"].items() if k != "markdown-tools"} != {
                k: v
                for k, v in (before.get("plugins") or {}).items()
                if k != "markdown-tools"
            }:
                raise AssertionError(
                    "plugin installation changed existing plugin approvals"
                )
            for path in source.rglob("*"):
                if (
                    path.is_file()
                    and "__pycache__" not in path.parts
                    and (target / path.relative_to(source)).read_bytes()
                    != path.read_bytes()
                ):
                    raise AssertionError(
                        "installed package differs from the tested source"
                    )
            os.write(master, b"\x11")
            wait(lambda: proc.poll() is not None, "clean editor shutdown")
            if proc.returncode != 0:
                raise AssertionError(f"editor exited {proc.returncode}")
            print(
                "Markdown Tools PTY passed: install, permissions, sandbox startup, live edit/render/source/format/save/undo/close, contents/idempotency, task report, checkbox edits/undo, preserved settings"
            )
            if args.config:
                print(f"Installed and enabled: {target}")
            if backup:
                print(f"Previous package backup: {backup}")
        finally:
            os.close(master)
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait(timeout=2)


if __name__ == "__main__":
    main()
