# SunBook

A Go terminal editor with VS Code style interaction, mouse selection, split views,
multiple cursors, language servers, Git, and an integrated shell. English UI;
macOS and Linux. 中文、Emoji 与组合字符按字素编辑，支持鼠标和键盘操作。

```sh
go build -o sunbook ./cmd/sunbook
./sunbook .                 # workspace
./sunbook path/to/file.go    # file, with its parent as the workspace
./sunbook --doctor          # external dependency report; no TUI required
```

Requires Go 1.26.2 or newer to build. Optional: `go install ./cmd/sunbook` to put
the binary in your Go bin directory. No external tools are required for basic
editing. Use a UTF-8 terminal with mouse reporting and truecolor support.

## Editor

- VS Code style activity bar: `E` Explorer, `S` Search, `G` Source Control.
  Each opens a persistent left sidebar; clicking the active activity hides it.
  Ctrl+B hides/restores the sidebar while the activity bar stays visible.
  File tabs, draggable panel borders, and two side-by-side editors.
- Clickable top toolbar: New, Open, Save, Find, Undo/Redo, Split, Terminal,
  and More (command palette). Narrow windows keep the actions that fit;
  all actions remain available under More/F1. Raised button faces show hover,
  press, active, and disabled states; hover shows an action hint.
- A slate blue workbench with warm gold accents, inset input fields, framed
  dialogs, and a compact status bar. Long file tabs keep their close button
  visible. The configured background, foreground, and accent also shape the
  surrounding surfaces and selection colors.
- Explorer has an inline filename/path filter, including files in collapsed
  directories. Click the field, or press `/` / Ctrl+F
  while the Explorer is focused. Matching is case-insensitive and fuzzy.
  Down selects results, Enter opens a file, and Esc or `x` clears the filter.
  Normal expanded folders are restored when the filter is cleared. With
  ripgrep installed, filtering respects ignore rules; the fallback skips
  `.git`, `node_modules`, and `vendor` and lists up to 20,000 files.
- Click, drag, double-click a word, triple-click a line, scroll with the wheel.
- Alt+click adds a cursor. Ctrl+D selects a word and then its next occurrence.
- Shared documents in splits; independent selections and scroll positions.
- Unicode grapheme movement/deletion, automatic indentation and bracket pairing,
  line comments, line movement, multi-cursor edits, and transactional undo/redo.
- Syntax highlighting, file search/replace, quick open, and workspace search.
- UTF-8/BOM and LF/CRLF preservation, atomic saves with original permissions,
  dirty-file prompts, disk change detection, and reload/overwrite choices.
- Binary and non-UTF-8 files are rejected with a message. Existing symlinks are
  resolved when opened so saving does not replace the symlink.

## Keys

| Key | Action |
| --- | --- |
| F1 / Ctrl+Shift+P | Command palette (all actions) |
| Ctrl+P / Ctrl+O | Quick open / open a path |
| Ctrl+N / Ctrl+W | New file / close tab |
| Ctrl+S / Ctrl+Shift+S | Save / Save As |
| Ctrl+Z / Ctrl+Y | Undo / redo |
| Ctrl+C / Ctrl+X / Ctrl+V | Copy / cut / paste |
| Ctrl+A / Ctrl+D | Select all / add next matching selection |
| Ctrl+Alt+Up/Down | Add cursor above/below |
| Shift+arrows | Extend selection |
| Ctrl+Left/Right | Move by word |
| Alt+Up/Down | Move selected lines |
| Tab / Shift+Tab | Indent / outdent |
| Ctrl+/ | Toggle line comments |
| Ctrl+F / Ctrl+H | Find / replace in file |
| Ctrl+Shift+F | Search workspace (ripgrep) |
| Ctrl+G | Go to line[:column] |
| Ctrl+B / Ctrl+J | Toggle sidebar / bottom panel |
| Ctrl+Shift+E | Show Explorer sidebar |
| Ctrl+Shift+G | Show Source Control sidebar |
| Ctrl+\ | Toggle split editor |
| Ctrl+K, Ctrl+O | Focus the other editor |
| Ctrl+Tab / Ctrl+Shift+Tab | Next / previous file tab |
| F4 | Open shell / switch focus between shell and editor |
| F6 | Cycle editor, active sidebar, and bottom panel focus |
| Ctrl+Space | Completion |
| F12 / Shift+F12 | Definition / references |
| F2 / Alt+H / Alt+Shift+F | Rename symbol / hover / format |
| Esc | Dismiss popup or reduce to one cursor |
| Ctrl+Q | Quit (prompts for unsaved changes) |

In the shell, ordinary keys including Ctrl+C go to the child process. F1, F4,
and F6 remain editor focus controls; sidebar shortcuts also work with shell
focus. Ctrl+J toggles the bottom panel even with shell
focus. Click **Hide** in the panel header to hide it and keep the shell running.
Click **Kill** to end the shell session and close its panel. F4 switches focus
between the visible shell and the editor, and opens a hidden terminal again.
Opening a terminal tab is lazy: it does not execute
a shell on editor startup. After the shell exits, F4 starts a fresh session.

Traditional terminals cannot distinguish some combinations (for example,
Ctrl+H and Backspace, or Ctrl+Space and NUL), and may reserve Cmd/Alt shortcuts.
Modern keyboard protocols improve this; F1 and configurable bindings provide
alternatives. If Alt+click is intercepted by the terminal, use Ctrl+Alt+Down or
the command palette to add cursors. Use the terminal's selection modifier
(often Shift) to select terminal text instead of editor text. The input method
and candidate window belong to the host terminal.

Clipboard uses `pbcopy`/`pbpaste` on macOS, `wl-copy`/`wl-paste` on Wayland, or
`xclip` under X11. An internal clipboard remains available when these fail.
Modal text fields support Ctrl+V and Ctrl+U to clear the field.

## Language services

Install the tools you want; SunBook never downloads or installs them.

```sh
go install golang.org/x/tools/gopls@latest
npm install -g pyright typescript-language-server typescript@6
# Python formatting; use your preferred Python package manager:
pipx install ruff
# Workspace text search:
brew install ripgrep         # macOS
# sudo apt install ripgrep   # Debian/Ubuntu
```

The current gopls release may require a newer Go toolchain than SunBook itself;
Go's toolchain selection handles that, or install a gopls version compatible
with your toolchain. TypeScript 6 supplies `tsserver.js`; TypeScript 7 packages
without that file are not compatible with this TypeScript server setup.

Go uses gopls. Python uses Pyright for intelligence and Ruff for formatting.
JS/JSX and TS/TSX use typescript-language-server with TypeScript. Available
features: completion (including import edits), hover, diagnostics, definition,
references, rename preview, and formatting. A rename updates editor buffers;
use **File: Save All** to write them. Each modified document has its own undo
history. Resource operations and server-initiated edits are not supported.

Servers start when a matching file is opened. Missing or failed servers leave
editing available. Run `sunbook --doctor`, then **Language: Restart Language
Servers** after fixing the installation. A project with a proper `go.mod`,
Python environment, or `tsconfig.json` gives better results than loose files.

## Git and terminal

Use `G`, Ctrl+Shift+G, or **View: Source Control** from F1 to open the Git
sidebar. Staged Changes and Changes are separate groups; a file with both
staged and unstaged edits appears in each. Click a row or press `d`/Enter for
that version's diff; `D` always shows the staged diff. `s` stages a file;
`u` unstages it; `o` opens the file; `c` commits staged
changes. Errors and hook failures appear in the status message. Commit messages
and paths are passed directly to Git, never interpolated into shell commands.
Configure Git name/email and signing separately as usual. There is no push,
branch management, or discard-changes command in this release.

Use `S` or Ctrl+Shift+F for the Search sidebar. It searches after a short
typing pause, or immediately with Enter / Go, using ripgrep regexes and smart
case. Results are grouped by file; click a match or select it with arrows and
Enter to jump to its line and column. Clear resets results; Stop cancels work.
The query, results, and scroll position remain when switching sidebars.
Explorer's filter and Git selection also keep their state. Terminal and
Problems stay in the independent bottom panel.

The terminal uses a real Unix PTY and a VT emulator, with colors, cursor
addressing, alternate screen, resize, and 10,000 lines of scrollback. Wheel
scrolling reads history. It is one session per workspace, using `$SHELL` (or
`/bin/sh`) in the workspace directory. Mouse events are forwarded when a child
program enables mouse reporting; Shift+wheel always scrolls terminal history.
Selection/copy of shell scrollback is left to the host terminal in this release.

## Configuration

Defaults work without a config file. `--doctor` prints the default path:

- macOS: `~/Library/Application Support/SunBook/config.json`
- Linux: `$XDG_CONFIG_HOME/SunBook/config.json` or `~/.config/SunBook/config.json`

See [config.example.json](config.example.json), or pass `--config path.json`
before the positional path. Configuration merges with defaults. The `servers`
keys are `go`, `python`, and `typescript`. Keybindings map lowercase key names
to command IDs; set an ID to `""` to disable a binding. The modifier order is
`ctrl+alt+shift+key`; chords use a space, such as `ctrl+k ctrl+o`.

Optional fields:

```json
{
  "shell": "/bin/zsh",
  "pythonPath": "/path/to/project/.venv/bin/python",
  "servers": {
    "typescript": {
      "initializationOptions": {
        "tsserver": { "path": "/path/to/typescript/lib/tsserver.js" }
      }
    },
    "python": { "settings": { "python": { "analysis": { "typeCheckingMode": "basic" } } } }
  }
}
```

File search uses fuzzy filename matching. File find/replace uses literal,
case-sensitive text; workspace search uses ripgrep regular expressions and
smart case, respects ignore rules, and displays up to 2,000 results. Syntax
highlighting is disabled above 2 MiB; basic editing remains available. The
editor is intended for ordinary source files; it is not a large-file viewer.

## Development and verification

```sh
make check                  # tests with race detection, then vet
make smoke                  # real PTY UI test; requires git and ripgrep
SUNBOOK_LSP_INTEGRATION=1 go test ./internal/lsp -run TestRealServers -v
go test ./internal/text -run '^$' -bench . -benchmem
```

The optional LSP suite requires all three servers in PATH; normal tests use
a local protocol fixture. Core tests cover randomized piece-table operations,
Unicode and UTF-16 positions, undo/redo, disk conflicts, encoding/permission
preservation, mouse hit testing, split synchronization, modal save decisions,
Git operations in temporary repositories, and shell ANSI/resize/interrupt.
CI runs race tests, vet, and the PTY smoke test on macOS and Linux.

Code is divided into `text` (piece table/history), `editor` (documents/views),
`ui` (event loop/rendering/commands), `lsp`, `git`, `terminal`, `clipboard`, and
`config`. The UI event loop owns mutable editor state. Workers post immutable,
version-checked results. The first release does not provide DAP debugging,
extensions, remote development, Vim modes, or VS Code configuration imports.
