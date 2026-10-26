#!/usr/bin/env python3
"""Fixture-only loopback proxy that drops one committed Nomad job response.

The proxy forwards all HTTP requests to a fixed loopback Nomad API origin. It
never logs request headers or bodies. On the first successful PUT /v1/jobs it
records only a timestamp and status, then closes the client connection without
forwarding the response. This models a response lost after Nomad committed the
job. It is not a production proxy.
"""

from __future__ import annotations

import argparse
import http.client
import json
import os
import re
import socket
import stat
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


class ProxyError(ValueError):
    pass


def _loopback(value: str, label: str) -> tuple[str, int]:
    parsed = urlsplit(value if "://" in value else f"http://{value}")
    if parsed.scheme != "http" or parsed.hostname != "127.0.0.1" or not parsed.port:
        raise ProxyError(f"{label} must be an explicit IPv4 loopback HTTP endpoint")
    if parsed.username or parsed.password or parsed.path not in ("", "/") or parsed.query or parsed.fragment:
        raise ProxyError(f"{label} must be an origin without credentials or path")
    return parsed.hostname, parsed.port


def _private_fixture_json(path: Path, root: Path) -> dict:
    info = path.lstat()
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        raise ProxyError("fixture manifest must be a regular non-symlink file")
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ProxyError("fixture manifest must be user-owned and private")
    resolved = path.resolve(strict=True)
    if resolved != root and root not in resolved.parents:
        raise ProxyError("fixture manifest must remain under fixture_root")
    try:
        value = json.loads(resolved.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ProxyError("fixture manifest is unreadable or invalid") from exc
    if not isinstance(value, dict):
        raise ProxyError("fixture manifest must be an object")
    return value


def derive_fixture_route(config_path: Path, profile_path: Path, listen: str) -> tuple[Path, str, str]:
    """Derive the upstream only from the captured fixture HTTP API endpoint."""
    config_path = Path(config_path).resolve(strict=True)
    config_probe = json.loads(config_path.read_text(encoding="utf-8"))
    root_text = config_probe.get("fixture_root") if isinstance(config_probe, dict) else None
    if not isinstance(root_text, str) or not os.path.isabs(root_text):
        raise ProxyError("fixture config must declare an absolute fixture_root")
    root = Path(root_text).resolve(strict=True)
    info = root.lstat()
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ProxyError("fixture_root must be a private user-owned directory")
    config = _private_fixture_json(config_path, root)
    profile = _private_fixture_json(Path(profile_path), root)
    if config.get("fixture_only") is not True or profile.get("schema_version") != 1:
        raise ProxyError("route preflight requires owned fixture manifests")
    session = config.get("session")
    nomad = config.get("nomad")
    profile_config = profile.get("config")
    if not isinstance(session, dict) or not isinstance(nomad, dict) or not isinstance(profile_config, dict):
        raise ProxyError("fixture route identity is incomplete")
    proxy_host, proxy_port = _loopback(listen, "listen")
    expected_proxy = f"http://{proxy_host}:{proxy_port}"
    profile_address = profile_config.get("address")
    if not isinstance(profile_address, str) or profile_address.rstrip("/") != expected_proxy:
        raise ProxyError("Nomad profile address does not match the owned proxy listener")
    upstream = session.get("nomad_http")
    upstream_host, upstream_port = _loopback(upstream, "fixture HTTP API")
    rpc_host, rpc_port = _loopback(nomad.get("server_rpc"), "fixture RPC")
    if upstream_host == rpc_host and upstream_port == rpc_port:
        raise ProxyError("refusing to use Nomad RPC as an HTTP API upstream")
    if proxy_host == rpc_host and proxy_port == rpc_port:
        raise ProxyError("refusing to bind the fixture HTTP proxy on the Nomad RPC port")
    if proxy_host == upstream_host and proxy_port == upstream_port:
        raise ProxyError("fixture HTTP proxy cannot loop back to itself")
    if nomad.get("version") != "2.0.7":
        raise ProxyError("fixture Nomad binary version is not pinned to 2.0.7")
    node_id = session.get("node_id")
    if not isinstance(node_id, str) or not node_id:
        raise ProxyError("fixture NodeID is missing")
    return root, str(upstream).rstrip("/"), node_id


def verify_nomad_http_identity(base_url: str, expected_node_id: str, opener=urllib.request.urlopen) -> dict:
    """Verify version, leader and exact ready node without emitting raw config."""
    try:
        with opener(base_url + "/v1/agent/self", timeout=4) as response:
            if response.status != 200:
                raise ProxyError("Nomad agent identity endpoint did not return success")
            agent = json.loads(response.read())
        version = agent.get("config", {}).get("Version", {})
        stats = agent.get("stats", {}).get("nomad", {})
        if version.get("Version") != "2.0.7" or not version.get("Revision"):
            raise ProxyError("Nomad server version identity differs from the pinned fixture")
        if stats.get("server") != "true" or stats.get("leader") != "true":
            raise ProxyError("Nomad HTTP endpoint is not the expected fixture leader")
        with opener(base_url + "/v1/status/leader", timeout=4) as response:
            if response.status != 200:
                raise ProxyError("Nomad leader endpoint did not return success")
            leader = json.loads(response.read())
        if not isinstance(leader, str) or not leader:
            raise ProxyError("Nomad leader identity is empty")
        with opener(base_url + "/v1/node/" + expected_node_id, timeout=4) as response:
            if response.status != 200:
                raise ProxyError("expected fixture node endpoint did not return success")
            node = json.loads(response.read())
        if node.get("ID") != expected_node_id or node.get("Status") != "ready":
            raise ProxyError("expected fixture node identity is not ready")
        return {
            "version": version["Version"],
            "revision": version["Revision"],
            "leader_verified": True,
            "node_id": expected_node_id,
            "node_ready": True,
        }
    except (urllib.error.URLError, TimeoutError, OSError, json.JSONDecodeError) as exc:
        raise ProxyError("Nomad HTTP identity preflight failed") from exc


def _marker_write(path: Path, status: int) -> None:
    payload = json.dumps({"schema": 1, "committed_http_status": status, "dropped_at_unix_ns": time.time_ns()}) + "\n"
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            stream.write(payload)
            stream.flush()
            os.fsync(stream.fileno())
    except Exception:
        try:
            path.unlink()
        except OSError:
            pass
        raise


def make_server(listen: str, upstream: str, marker: Path, release: Path | None = None) -> ThreadingHTTPServer:
    bind_host, bind_port = _loopback(listen, "listen")
    upstream_host, upstream_port = _loopback(upstream, "upstream")
    marker_parent = marker.parent.resolve(strict=True)
    if marker_parent not in marker.resolve(strict=False).parents:
        raise ProxyError("event marker must be a file directly within an existing fixture directory")
    try:
        existing = marker.lstat()
    except FileNotFoundError:
        existing = None
    if existing is not None and (stat.S_ISLNK(existing.st_mode) or not stat.S_ISREG(existing.st_mode) or existing.st_uid != os.getuid() or existing.st_mode & 0o077):
        raise ProxyError("event marker path must be absent or an owned private regular file")
    if release is not None:
        release_parent = release.parent.resolve(strict=True)
        if release_parent != marker_parent or release.exists() or release.is_symlink():
            raise ProxyError("release marker must be a new path beside the event marker")

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"
        _drop_lock = threading.Lock()

        def log_message(self, *_: object) -> None:
            return

        def _forward(self) -> None:
            if not self.path.startswith("/v1/") or "\r" in self.path or "\n" in self.path:
                self.send_error(404)
                return
            length_text = self.headers.get("Content-Length", "0")
            if not length_text.isdigit() or int(length_text) > 8 * 1024 * 1024:
                self.send_error(413)
                return
            body = self.rfile.read(int(length_text)) if int(length_text) else None
            headers = {key: value for key, value in self.headers.items() if key.lower() not in {"connection", "transfer-encoding", "host"}}
            connection = http.client.HTTPConnection(upstream_host, upstream_port, timeout=30)
            try:
                connection.request(self.command, self.path, body=body, headers=headers)
                response = connection.getresponse()
                response_body = response.read()
                drop = False
                if self.command == "PUT" and self.path.split("?", 1)[0] == "/v1/jobs" and 200 <= response.status < 300:
                    with self._drop_lock:
                        if not marker.exists():
                            _marker_write(marker, response.status)
                            drop = True
                if drop:
                    if release is not None:
                        deadline = time.monotonic() + 120
                        while time.monotonic() < deadline and not release.exists():
                            time.sleep(0.02)
                    self.close_connection = True
                    self.connection.shutdown(socket.SHUT_RDWR)
                    self.connection.close()
                    return
                self.send_response(response.status, response.reason)
                for key, value in response.getheaders():
                    if key.lower() not in {"connection", "transfer-encoding", "content-length", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "upgrade"}:
                        self.send_header(key, value)
                self.send_header("Content-Length", str(len(response_body)))
                self.end_headers()
                if response_body:
                    self.wfile.write(response_body)
            finally:
                connection.close()

        do_GET = _forward
        do_POST = _forward
        do_PUT = _forward
        do_DELETE = _forward
        do_PATCH = _forward

    server = ThreadingHTTPServer((bind_host, bind_port), Handler)
    server.daemon_threads = True
    return server


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--listen", required=True, help="127.0.0.1:port")
    parser.add_argument("--fixture-config", required=True, type=Path)
    parser.add_argument("--profile-source", required=True, type=Path)
    parser.add_argument("--event-marker", required=True, type=Path)
    parser.add_argument("--release-marker", type=Path)
    args = parser.parse_args()
    try:
        root, upstream, node_id = derive_fixture_route(args.fixture_config, args.profile_source, args.listen)
        direct_identity = verify_nomad_http_identity(upstream, node_id)
        if args.event_marker.parent.resolve(strict=True) != root or (args.release_marker and args.release_marker.parent.resolve(strict=True) != root):
            raise ProxyError("fault markers must be direct children of the exact fixture root")
        server = make_server(args.listen, upstream, args.event_marker, args.release_marker)
    except (OSError, ProxyError) as exc:
        parser.error(str(exc))
    serving = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.1}, daemon=True)
    try:
        serving.start()
        proxied_identity = verify_nomad_http_identity(f"http://{args.listen}", node_id)
        if proxied_identity != direct_identity:
            raise ProxyError("proxy identity differs from the direct fixture HTTP API")
        print(json.dumps({"proxy_ready": True, **proxied_identity}, sort_keys=True), flush=True)
        serving.join()
    except KeyboardInterrupt:
        pass
    except ProxyError as exc:
        print(json.dumps({"proxy_ready": False, "error": str(exc)}, sort_keys=True), flush=True)
        return 2
    finally:
        server.shutdown()
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
