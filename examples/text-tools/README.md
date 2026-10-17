# Text Tools

Requires Python 3.9+. Install this complete `text-tools` folder through SunBook's
Plugins sidebar, review `document.read` and `document.edit`, then enable it.

- **Count Words:** show word, line, character and UTF-8 byte counts.
- **Uppercase Selection:** preview uppercase edits for all nonempty selections;
  Unicode selections retain UTF-8 byte offsets. Changes remain unsaved and undo
  together. Empty selections return guidance without changing the document.

The plugin does not perform file or network IO. Both permissions are used because
one process contributes a read-only count command and an editing command.
`sunbook_plugin.py` is a vendored copy of the API v1 Python SDK.
On macOS it runs in the required filesystem sandbox with networking disabled.
The host supplies a private temporary HOME and filters the editor's environment;
this example requires no inherited credentials or persistent state.

```sh
go run ./cmd/sunbook --check-plugin examples/text-tools
python3 -m unittest discover -s scripts -p 'test_plugin_sdk.py'
go test ./internal/plugin -run TestManifestValidationAndPythonExampleConformance
```

Version 1.0.0: initial API v1 implementation. See [development rules](../../docs/plugins.md).
