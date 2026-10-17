# Contributing to SunBook

Keep editor mutations on the UI event loop. File IO, Git operations, language
servers and plugins must deliver bounded background results and reject stale
results before modifying editor state. Preserve uncommitted working-tree changes,
unsaved buffers, Unicode grapheme boundaries and existing undo semantics.

## Local checks

```sh
make check
make smoke
go mod verify
```

Use focused tests during development, then run these gates before submitting.
The existing CI runs them on macOS and Linux. Changes to rendering should be
checked in both a dark and a light theme, with keyboard and mouse input and a
narrow terminal. `SUNBOOK_PREVIEW_DIR=/absolute/output/path go test ./internal/ui
-run TestExtensionPreviewArtifact` exports actual extension UI cells for review.

## Plugin changes

Read [Plugin development and release rules](docs/plugins.md) before adding or
changing a plugin or host capability. Keep manifests compatible with the
[API v1 schema](docs/plugins/manifest-v1.schema.json), declare only required
editor permissions, and keep plugin implementations independent of `internal/ui`.
macOS plugins require the Seatbelt backend: keep file access restricted, network
denied unless explicitly declared, and never fall back to unrestricted execution.
Linux currently provides process isolation only; keep this distinction visible.
Approval covers package contents, so plugin state must live in the disposable
HOME/TMPDIR. Never pass the editor's credentials or loader environment through.

New examples belong in `examples/<id>/`, must be directly installable and must
include a README, runtime dependencies, command tests and version history.
`TestCheckedInPluginExamplesConform` discovers every checked-in example and
requires its initialization and ping contract to pass in CI. Add any new runtime
to the CI environment rather than skipping its conformance gate. Keep vendored
SDK copies in sync with their canonical `sdk/` version.

An extension API change must include matching schema/protocol documentation,
host validation, an SDK/example update and compatibility tests. Incompatible wire
changes require a new API version and a migration path. New failure paths need
regressions for timeouts, protocol failures, process cleanup and isolation from a
healthy neighboring plugin. On macOS, exercise actual denied file/network access,
permitted networking, descendant restrictions and unavailable-backend behavior.
UI changes must cover install, review, enable,
command execution, stale edits, disable/restart/remove and quarantine recovery.

## Review notes

Explain the final behavior, why it is needed, applicable API/permission changes
and the checks actually run. Describe remaining platform or dependency gaps.
Keep unrelated local changes intact; do not include generated binaries, caches or
personal configuration in the change.
