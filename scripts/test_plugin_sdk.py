import importlib.util
import io
import json
import re
from types import SimpleNamespace
from unittest.mock import patch
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location("sunbook_plugin", ROOT / "sdk/python/sunbook_plugin.py")
sdk = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sdk)


class PluginSDKTests(unittest.TestCase):
    def test_markdown_preview_requires_capability_and_valid_payload(self):
        plugin = sdk.Plugin({"preview": lambda doc: {"preview": {"kind": "markdown", "text": "# 中文"}}})
        plugin.dispatch("initialize", {"protocolVersion": 1})
        with self.assertRaises(sdk.PluginError):
            plugin.dispatch("execute", {"command": "preview"})
        self.assertEqual(plugin.dispatch("ping", {}), {})
        plugin.dispatch("initialize", {"protocolVersion": 1, "capabilities": ["preview.markdown"]})
        self.assertEqual(plugin.dispatch("execute", {"command": "preview"})["preview"]["text"], "# 中文")
        for bad in ({"kind": "html", "text": "<script>"}, {"kind": "markdown", "text": "bad\0text"}, {"kind": "markdown", "text": "x" * (1024 * 1024 + 1)}):
            plugin.commands["preview"] = lambda doc: {"preview": bad}
            with self.assertRaises(sdk.PluginError):
                plugin.dispatch("execute", {"command": "preview"})
        self.assertEqual(plugin.dispatch("ping", {}), {})

    def test_schema_version_pattern_accepts_checked_in_example(self):
        schema = json.loads((ROOT / "docs/plugins/manifest-v1.schema.json").read_text())
        pattern = schema["properties"]["version"]["pattern"]
        for manifest_path in (ROOT / "examples").glob("*/plugin.json"):
            manifest = json.loads(manifest_path.read_text())
            self.assertIsNotNone(re.fullmatch(pattern, manifest["version"]))
        self.assertIsNone(re.fullmatch(pattern, "1x0x0"))

    def test_handshake_health_and_command_errors(self):
        plugin = sdk.Plugin({"count": lambda doc: {"message": str(len(doc["text"]))}})
        with self.assertRaises(sdk.PluginError):
            plugin.dispatch("execute", {"command": "count"})
        with self.assertRaises(sdk.PluginError):
            plugin.dispatch("initialize", {"protocolVersion": 2})
        self.assertEqual(plugin.dispatch("initialize", {"protocolVersion": 1}), {"protocolVersion": 1})
        self.assertEqual(plugin.dispatch("ping", {}), {})
        self.assertEqual(plugin.dispatch("execute", {"command": "count", "document": {"text": "中hello"}}), {"message": "6"})
        with self.assertRaises(sdk.PluginError):
            plugin.dispatch("execute", {"command": "unknown"})

    def test_utf8_byte_offsets(self):
        selected, start, end = sdk.selected_text({"text": "中hello👩‍💻"}, {"anchor": 8, "head": 3})
        self.assertEqual((selected, start, end), ("hello", 3, 8))
        with self.assertRaises(UnicodeDecodeError):
            sdk.selected_text({"text": "中hello"}, {"anchor": 1, "head": 4})

    def test_command_errors_and_bad_results_keep_the_protocol_alive(self):
        for handler in (lambda doc: {"body": "中" * 300000},
                        lambda doc: {"custom": {1, 2}},
                        lambda doc: {"custom": float("nan")}):
            plugin = sdk.Plugin({"bad": handler})
            requests = [{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": 1}},
                        {"jsonrpc": "2.0", "id": 2, "method": "execute", "params": {"command": "bad"}},
                        {"jsonrpc": "2.0", "id": 3, "method": "ping", "params": {}}]
            incoming = io.BytesIO(b"".join((json.dumps(r) + "\n").encode() for r in requests))
            outgoing = io.BytesIO()
            with patch.object(sdk.sys, "stdin", SimpleNamespace(buffer=incoming)), patch.object(sdk.sys, "stdout", SimpleNamespace(buffer=outgoing)):
                plugin.run()
            responses = [json.loads(line) for line in outgoing.getvalue().splitlines()]
            self.assertIn("error", responses[1])
            self.assertEqual(responses[1]["id"], 2)
            self.assertEqual(responses[2]["result"], {})

    def test_vendored_example_sdk_matches(self):
        canonical = (ROOT / "sdk/python/sunbook_plugin.py").read_bytes()
        for vendored in (ROOT / "examples").glob("*/sunbook_plugin.py"):
            with self.subTest(plugin=vendored.parent.name):
                self.assertEqual(canonical, vendored.read_bytes())


if __name__ == "__main__":
    unittest.main()
