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

- VS Code style activity bar with compact folder, magnifier, and branch icons
  for Explorer, Search, and Source Control. Icons highlight on hover and
  selection; hover shows the panel name and shortcut. The default icons use
  Nerd Font glyphs; choose a Nerd Font in your terminal (such as Maple Mono NF),
  or set `"activityIcons": "unicode"` for ordinary character icons. This setting
  applies to toolbar and panel action icons too.
  Each opens a persistent left sidebar; clicking the active activity hides it.
  Ctrl+B hides/restores the sidebar while the activity bar stays visible.
  Drag file tabs to reorder them, or drop a tab on an editor's left/right edge
  to open it in a split. Existing groups accept dropped tabs; drafts and cursor
  positions are preserved. A marker previews the drop; Esc cancels a drag.
  Panel borders can be dragged to resize them.
- Clickable top toolbar: New, Open, Save, Find, Undo/Redo, Split, Terminal,
  More (command palette), and **Help** at the top right. Help opens a scrollable
  shortcut reference showing current keybindings and controls for each panel;
  Esc closes it. Actions use the same icon family as the activity bar; wider
  windows also show names for New, Open, Save, Find, Terminal, and Help.
  Undo, redo, split, refresh, and menu controls use compact icons. Hover shows
  the operation and shortcut. Narrow windows use icon buttons for more actions;
  all actions remain available under More/F1. Raised button faces show hover,
  press, active, and disabled states; hover shows an action hint.
- Explorer, Search, and Source Control share inset headers, count badges,
  section bands, and clear selections. Explorer adds tree guides; long lists
  have scroll indicators. Git status and log refresh silently.
- A slate blue workbench with warm gold accents, inset input fields, framed
  dialogs, and a compact status bar. Long file tabs keep their close button
  visible. The configured background, foreground, and accent also shape the
  surrounding surfaces and selection colors.
- Explorer has an inline filename/path filter, including files in collapsed
  directories. Click the field, or press `/` / Ctrl+F
  while the Explorer is focused. Matching is case-insensitive and fuzzy.
  Down selects results, Enter opens a file, and Esc or `x` clears the filter.
  The selected file keeps its highlight and left marker after focus moves to
  the editor. Open / Quick Open also keep a distinct marker beside their icons.
  Normal expanded folders are restored when the filter is cleared. With
  ripgrep installed, filtering respects ignore rules; the fallback skips
  `.git`, `node_modules`, and `vendor` and lists up to 20,000 files.
- File type icons appear in Explorer, Open / Quick Open, editor tabs, Search,
  and Git file lists. Folders show separate open/closed marks; source files,
  documents, settings, images, and archives have distinct colors and symbols.
  Language marks follow filename extensions: `Go`, `Py`, `JS`, `JSX`, `TS`,
  `TSX`, `C`, `C++`, `C#`, `Jav`, `Kt`, `Sw`, `Rb`, `PHP`, `Lua`, and more.
  Vue / Svelte, HTML / XML, CSS / Sass / Less, and JSON / YAML / TOML also
  have separate marks. Matching is case-insensitive; TypeScript declarations
  use `dTS`. Icons use standard characters, without a patched font.
- Single-click a file in Explorer or a Search result to preview it in an italic
  tab. Selecting another file reuses that preview tab. Double-click the file or
  its tab to keep it open; editing, saving, or dragging the tab also keeps it.
  Enter, Open / Quick Open, and opening in a split keep files open immediately.
  Existing open files and unsaved changes are preserved when browsing previews.
- Click, drag, double-click a word, triple-click a line, scroll with the wheel.
- Alt+click adds a cursor. Ctrl+D selects a word and then its next occurrence.
- Shared documents in splits; independent selections and scroll positions.
- Unicode grapheme movement/deletion, automatic indentation and bracket pairing,
  line comments, line movement, multi-cursor edits, and transactional undo/redo.
- Syntax highlighting, file search/replace, quick open, and workspace search.
- Ctrl+P ranks workspace files by filename and fuzzy path matching, with recent
  files first when the query is empty. The file index is shared with Explorer.
  Add `:line[:column]` to jump (for example `main.go:20:5`); columns follow the
  displayed text, including tabs and wide characters. Recent files last for
  the current session.
- Ctrl+O opens a file browser with folders first, path completion with Tab,
  and an **↑ Up** button (or Alt+Up) to go to the parent directory. Click the
  directory path or press Ctrl+L to edit it, then Enter / **Go** to navigate.
  Tab returns from the path bar to the filename field; Esc cancels path edits
  before closing the dialog. Invalid paths keep the current folder visible.
  Enter or click on a list row opens a file or browses a folder. Relative, absolute, and `~/` paths are supported. A new
  filename opens an empty buffer that is written only when saved. Ctrl+Enter
  in either picker opens the file in the other editor group (when the terminal
  can report that key combination). Errors keep the picker open.
- UTF-8/BOM and LF/CRLF preservation, atomic saves with original permissions,
  dirty-file prompts, disk change detection, and reload/overwrite choices.
- Binary and non-UTF-8 files are rejected with a message. Existing symlinks are
  resolved when opened so saving does not replace the symlink.

## Keys

| Key | Action |
| --- | --- |
| F1 / Ctrl+Shift+P | Command palette (all actions) |
| Ctrl+P / Ctrl+O | Quick open / browse and open files |
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
| Ctrl+B / Ctrl+J | Toggle sidebar / panel |
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
Click **Right** in the panel header to dock it on the right, and **Bottom** to
return it below the editor. Drag the divider to resize its width or height.
The panel uses an X icon to hide and a trash icon to end the shell; hover
shows the action before clicking. Position, size, output, and the shell session
survive switching and hiding.
Narrow windows temporarily use the bottom layout and restore the right dock
when there is enough room. The command palette also includes **View: Toggle
Panel Position (Bottom / Right)**.
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

Click the branch icon, use Ctrl+Shift+G, or **View: Source Control** from F1 to open the Git
sidebar. Staged files, tracked changes, untracked files, and conflicts have
separate counted groups and colored status badges. A file with both staged and
unstaged edits appears in each group. Filenames stay readable in narrow sidebars;
hover shows the full path. Branch arrows show ahead/behind counts from local refs.

Click a row or press `d`/Enter to open an editor comparison tab. Staged changes
compare HEAD with Index; unstaged changes compare Index with the saved worktree.
Added and removed rows are aligned and colored, with line numbers and synchronized
vertical scrolling. Click either pane to focus it; Tab switches sides, Left/Right
pan, `n`/`p` jump between changes, and the divider can be dragged. The comparison
is read-only: `o` opens the working file for editing; Ctrl+W / the tab's close
button restores the original editor. Open buffers and unsaved edits are preserved.
`D` shows the selected file's staged comparison. Binary files show byte counts.

Git log is shown at the bottom of the sidebar with commit hashes, subjects,
authors, and relative times. Its wheel scroll is independent of the change list.
`h`, **Log**, or Tab from the change list focuses it; arrows select and Enter or
click views a commit patch. Drag the log header divider up/down to resize the
log and change list; the size is remembered while switching sidebars. The header
arrow expands the log and restores the chosen size. **More →
Selected file history** restricts the log to that file (following renames);
**Log** returns to repository history. Background refresh keeps the selected
commit and scroll position without progress messages or flickering buttons.

`s` stages a file; `u` unstages it; `o` opens it. Group-header `+` stages the
entire group; `−` unstages all staged files. **More** also provides repository-wide
stage/unstage and repository details. `S` / `U` stage / unstage all changes.

Press `c` or click the message field to write a commit message, then Enter or
**Commit** to commit staged files only. Drafts survive sidebar switches and failed
commits; successful commits clear the submitted draft. Conflicts block commits
until resolved and staged. Git operates on saved files on disk. `/` or Ctrl+F
filters changed paths; Esc in the filter clears it. Small terminals provide the
filter in a prompt. Filtering does not change the scope of commits or group/batch
actions. F5 refreshes status. Errors and hook failures appear in the status
message, with a persistent error summary in the sidebar. Commit messages and
paths are passed directly to Git, never interpolated into shell commands.
Configure Git name/email and signing separately as usual. There is no push,
branch management, or discard-changes command in this release.

Click the magnifier or use Ctrl+Shift+F for the Search sidebar. It searches after a short
typing pause, or immediately with Enter / Go, using ripgrep regexes and smart
case. **Aa** forces case-sensitive matching, **W** matches whole words, and
`.*` switches between regular expressions and literal text. **...** reveals
include/exclude file globs, separated by commas (for example `src/**, *.go` and
`vendor/**`). Small windows use prompts for these fields. Results arrive as
search progresses, grouped into collapsible file sections with counts. Matching
text is highlighted even far into a long line; hover shows the full path. Click
a match or select it with arrows and Enter to jump and select its text. Tab
returns to the search input. At 2,000 matches the list reports its limit; narrow
the query or paths for more targeted results. Clear resets results; Stop cancels work.
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
