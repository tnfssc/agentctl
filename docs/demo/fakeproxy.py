"""A stand-in CLIProxyAPI for recording the demo: health, and a model list
that, like the real proxy, disguises non-Claude ids for Claude clients. It
also answers T3 Code's environment probe for the fake T3 Code server."""

import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

KEY = "sk-demo"
T3_VERSION = "0.1.8-nightly.20261111.4420"
MODELS = {
    "claude-opus-5-5": "anthropic",
    "claude-sonnet-5-5": "anthropic",
    "claude-haiku-5-5": "anthropic",
    "gpt-6.1-sol": "openai",
    "gpt-6-luna": "openai",
    "gpt-6-astra": "openai",
    "gemini-3.1-pro": "google",
    "gpt-image-2": "openai",
}


class Proxy(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/healthz":
            return self.send({"status": "ok"})
        if self.path == "/.well-known/t3/environment":
            return self.send({"label": "studio", "serverVersion": T3_VERSION})
        if self.path.startswith("/v1/models"):
            if self.headers.get("Authorization") != "Bearer " + KEY:
                return self.send({"error": "invalid api key"}, 401)
            claude_client = self.headers.get("anthropic-version") is not None
            data = []
            for model_id, owner in MODELS.items():
                if claude_client and owner != "anthropic":
                    model_id = "claude-fable-5-dd-" + model_id[::-1]
                data.append({"id": model_id, "object": "model", "owned_by": owner})
            return self.send({"object": "list", "data": data})
        self.send({"error": "not found"}, 404)

    def send(self, body, status=200):
        raw = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, *args):
        pass


HTTPServer(("127.0.0.1", int(sys.argv[1])), Proxy).serve_forever()
