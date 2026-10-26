from __future__ import annotations

import http.client
import json
import os
import socket
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from scripts.nomad_response_loss_proxy import (
    ProxyError,
    derive_fixture_route,
    make_server,
    verify_nomad_http_identity,
)


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


class UpstreamHandler(BaseHTTPRequestHandler):
    calls = []

    def log_message(self, *_):
        return

    def do_PUT(self):
        type(self).calls.append((self.command, self.path))
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        body = json.dumps({"EvalID": "eval-fixture", "JobModifyIndex": 7}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        type(self).calls.append((self.command, self.path))
        body = b'{"ID":"job-fixture"}'
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class ResponseLossProxyTest(unittest.TestCase):
    def fixture_manifests(self, root: Path, *, profile_address="http://127.0.0.1:26461"):
        config = {
            "fixture_only": True,
            "fixture_root": str(root),
            "nomad": {"server_rpc": "127.0.0.1:26447", "version": "2.0.7"},
            "session": {"nomad_http": "http://127.0.0.1:26446", "node_id": "node-fixture"},
        }
        profile = {"schema_version": 1, "config": {"address": profile_address}}
        config_path = root / "fixture.json"
        profile_path = root / "profile.json"
        for path, value in ((config_path, config), (profile_path, profile)):
            path.write_text(json.dumps(value), encoding="utf-8")
            os.chmod(path, 0o600)
        return config_path, profile_path

    def test_route_derives_http_api_from_manifest_not_rpc_endpoint(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            os.chmod(root, 0o700)
            config, profile = self.fixture_manifests(root)
            got_root, upstream, node_id = derive_fixture_route(config, profile, "127.0.0.1:26461")
            self.assertEqual(got_root, root.resolve())
            self.assertEqual(upstream, "http://127.0.0.1:26446")
            self.assertNotEqual(upstream, "http://127.0.0.1:26447")
            self.assertEqual(node_id, "node-fixture")

    def test_route_rejects_rpc_as_proxy_or_api_target(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            os.chmod(root, 0o700)
            config, profile = self.fixture_manifests(root, profile_address="http://127.0.0.1:26447")
            with self.assertRaises(ProxyError):
                derive_fixture_route(config, profile, "127.0.0.1:26447")
            data = json.loads(config.read_text())
            data["session"]["nomad_http"] = "http://127.0.0.1:26447"
            config.write_text(json.dumps(data), encoding="utf-8")
            with self.assertRaises(ProxyError):
                derive_fixture_route(config, profile, "127.0.0.1:26461")

    def test_route_rejects_profile_proxy_listener_mismatch(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            os.chmod(root, 0o700)
            config, profile = self.fixture_manifests(root)
            with self.assertRaises(ProxyError):
                derive_fixture_route(config, profile, "127.0.0.1:26462")

    def test_preflight_accepts_only_pinned_leader_and_exact_ready_node(self):
        payloads = {
            "/v1/agent/self": {
                "config": {"Version": {"Version": "2.0.7", "Revision": "fixture-revision"}},
                "stats": {"nomad": {"server": "true", "leader": "true"}},
            },
            "/v1/status/leader": "127.0.0.1:26449",
            "/v1/node/node-fixture": {"ID": "node-fixture", "Status": "ready"},
        }

        class Response:
            status = 200

            def __init__(self, value):
                self.value = value

            def __enter__(self):
                return self

            def __exit__(self, *_):
                return None

            def read(self):
                return json.dumps(self.value).encode()

        def opener(url, timeout):
            self.assertEqual(timeout, 4)
            return Response(payloads[url.removeprefix("http://fixture")])

        result = verify_nomad_http_identity("http://fixture", "node-fixture", opener)
        self.assertEqual(result, {
            "version": "2.0.7", "revision": "fixture-revision",
            "leader_verified": True, "node_id": "node-fixture", "node_ready": True,
        })
        self.assertNotIn("config", result)

    def test_preflight_rejects_changed_version_or_node(self):
        payloads = {
            "/v1/agent/self": {
                "config": {"Version": {"Version": "2.0.8", "Revision": "fixture-revision"}},
                "stats": {"nomad": {"server": "true", "leader": "true"}},
            },
            "/v1/status/leader": "127.0.0.1:26449",
            "/v1/node/node-fixture": {"ID": "different-node", "Status": "ready"},
        }

        class Response:
            status = 200

            def __init__(self, value): self.value = value
            def __enter__(self): return self
            def __exit__(self, *_): return None
            def read(self): return json.dumps(self.value).encode()

        with self.assertRaises(ProxyError):
            verify_nomad_http_identity("http://fixture", "node-fixture", lambda url, timeout: Response(payloads[url.removeprefix("http://fixture")]))

    def test_drops_one_successful_registration_response_then_forwards_reconciliation(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            upstream = ThreadingHTTPServer(("127.0.0.1", 0), UpstreamHandler)
            upstream_thread = threading.Thread(target=upstream.serve_forever, daemon=True)
            upstream_thread.start()
            marker = root / "event.json"
            proxy = make_server(f"127.0.0.1:{free_port()}", f"127.0.0.1:{upstream.server_port}", marker)
            proxy_thread = threading.Thread(target=proxy.serve_forever, daemon=True)
            proxy_thread.start()
            UpstreamHandler.calls = []
            try:
                with self.assertRaises(http.client.RemoteDisconnected):
                    conn = http.client.HTTPConnection("127.0.0.1", proxy.server_port, timeout=3)
                    conn.request("PUT", "/v1/jobs", body=b'{"fixture":"no-secret"}', headers={"Content-Type": "application/json"})
                    conn.getresponse()
                self.assertTrue(marker.is_file())
                event = json.loads(marker.read_text())
                self.assertEqual(event["committed_http_status"], 200)
                self.assertNotIn("fixture", marker.read_text())
                conn = http.client.HTTPConnection("127.0.0.1", proxy.server_port, timeout=3)
                conn.request("GET", "/v1/job/job-fixture")
                response = conn.getresponse()
                self.assertEqual(response.status, 200)
                self.assertEqual(json.loads(response.read()), {"ID": "job-fixture"})
                self.assertEqual(UpstreamHandler.calls, [("PUT", "/v1/jobs"), ("GET", "/v1/job/job-fixture")])
                conn.close()
            finally:
                proxy.shutdown()
                proxy.server_close()
                upstream.shutdown()
                upstream.server_close()
                proxy_thread.join(timeout=2)
                upstream_thread.join(timeout=2)

    def test_rejects_non_loopback_upstream_and_listener(self):
        with tempfile.TemporaryDirectory() as temp:
            marker = Path(temp) / "event.json"
            with self.assertRaises(ProxyError):
                make_server("0.0.0.0:26460", "127.0.0.1:26446", marker)
            with self.assertRaises(ProxyError):
                make_server("127.0.0.1:26460", "nomad.example:4646", marker)


if __name__ == "__main__":
    unittest.main()
