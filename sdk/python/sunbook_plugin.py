"""SunBook plugin API v1. Python 3.9+, standard library only.

stdout is reserved for JSON-RPC. Use stderr for diagnostic logging.
Document edit offsets are UTF-8 bytes in the supplied immutable snapshot.
The host filters inherited environment variables and supplies disposable HOME
and TMPDIR directories. macOS denies networking unless declared in plugin.json.
"""
import json
import sys

API_VERSION = 1
MAX_MESSAGE = 4 * 1024 * 1024


class PluginError(Exception):
    def __init__(self, message, code=-32000):
        super().__init__(message)
        self.code = code


def selected_text(document, selection):
    raw = document["text"].encode("utf-8")
    start, end = sorted((selection["anchor"], selection["head"]))
    if start < 0 or end > len(raw):
        raise PluginError("Selection outside document")
    return raw[start:end].decode("utf-8"), start, end


class Plugin:
    def __init__(self, commands):
        self.commands = commands
        self.initialized = False
        self.capabilities = set()

    def dispatch(self, method, params):
        if method == "initialize":
            if params.get("protocolVersion") != API_VERSION:
                raise PluginError("Unsupported protocol", -32602)
            self.initialized = True
            self.capabilities = set(params.get("capabilities", []))
            return {"protocolVersion": API_VERSION}
        if not self.initialized:
            raise PluginError("Initialize first", -32002)
        if method == "ping":
            return {}
        if method == "execute":
            command = params.get("command")
            if command not in self.commands:
                raise PluginError("Unknown command", -32601)
            result = self.commands[command](params.get("document"))
            if not isinstance(result, dict):
                raise PluginError("Command must return a result object")
            for key, limit in (("title", 200), ("message", 4096), ("body", 256 * 1024)):
                if key in result and (not isinstance(result[key], str)
                                      or len(result[key].encode("utf-8")) > limit):
                    raise PluginError(f"{key} exceeds the host's text limit")
            if "edits" in result and (not isinstance(result["edits"], list)
                                      or len(result["edits"]) > 10000):
                raise PluginError("edits must be an array of at most 10000 changes")
            if "preview" in result:
                preview = result["preview"]
                if "preview.markdown" not in self.capabilities:
                    raise PluginError("Update SunBook to a version that supports split Markdown previews.")
                if (not isinstance(preview, dict) or preview.get("kind") != "markdown"
                        or not isinstance(preview.get("text"), str)
                        or len(preview["text"].encode("utf-8")) > 1024 * 1024
                        or "\0" in preview["text"]):
                    raise PluginError("preview must contain Markdown text up to 1 MiB without NUL")
            return result
        raise PluginError("Unknown method", -32601)

    def run(self):
        while True:
            line = sys.stdin.buffer.readline(MAX_MESSAGE + 1)
            if not line:
                return
            if len(line) > MAX_MESSAGE:
                print("SunBook request exceeds 4 MiB", file=sys.stderr)
                return
            request = None
            try:
                request = json.loads(line)
                if (not isinstance(request, dict) or request.get("jsonrpc") != "2.0"
                        or type(request.get("id")) is not int
                        or not isinstance(request.get("params"), dict)):
                    raise PluginError("Invalid request", -32600)
                result = self.dispatch(request.get("method"), request["params"])
                response = {"jsonrpc": "2.0", "id": request["id"], "result": result}
            except Exception as exc:
                request_id = request.get("id") if isinstance(request, dict) else None
                response = {"jsonrpc": "2.0", "id": request_id,
                            "error": {"code": getattr(exc, "code", -32000),
                                      "message": str(exc).encode("utf-8", "replace")[:2000].decode("utf-8", "ignore")}}
            try:
                data = json.dumps(response, ensure_ascii=False, allow_nan=False).encode("utf-8") + b"\n"
            except (TypeError, ValueError, UnicodeError):
                data = (json.dumps({"jsonrpc": "2.0", "id": response["id"],
                                    "error": {"code": -32603, "message": "Command result is not valid JSON/UTF-8"}}) + "\n").encode()
            if len(data) > MAX_MESSAGE:
                data = (json.dumps({"jsonrpc": "2.0", "id": response["id"],
                                    "error": {"code": -32000, "message": "Result exceeds 4 MiB"}}) + "\n").encode()
            sys.stdout.buffer.write(data)
            sys.stdout.buffer.flush()
