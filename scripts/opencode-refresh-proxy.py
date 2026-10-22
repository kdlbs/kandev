"""Test-only OAuth refresh observation; never logs request/response contents.

Import RefreshProxy from the opt-in credential harness. The caller owns the
temporary certificate directory and must trust it only in disposable clients.
"""

import base64
import hmac
import http.client
import http.server
import json
from pathlib import Path
import secrets
import select
import socket
import ssl
import subprocess
import threading
import time
import urllib.parse


class RefreshProxy:
    def __init__(self, directory, host="127.0.0.1", participants=3):
        self.directory = Path(directory)
        self.directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.cert = self.directory / "refresh-test-ca.pem"
        key = self.directory / "refresh-test-key.pem"
        subprocess.run([
            "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
            "-keyout", str(key), "-out", str(self.cert), "-days", "1",
            "-subj", "/CN=auth.openai.com",
            "-addext", "subjectAltName=DNS:auth.openai.com",
            "-addext", "basicConstraints=critical,CA:TRUE",
        ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        key.chmod(0o600)
        self.tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        self.tls.load_cert_chain(self.cert, key)
        self.password = secrets.token_urlsafe(32)
        self.authorization = "Basic " + base64.b64encode(
            ("validation:" + self.password).encode()).decode()
        self.barrier = threading.Barrier(participants, timeout=120)
        self.lock = threading.Lock()
        self.records = []
        self.aborted = False
        self.rejected = {"host": 0, "path": 0, "grant": 0, "authentication": 0}
        owner = self

        class QuietHandler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

        class Inspection(QuietHandler):
            def do_POST(self):
                # Only the documented refresh endpoint is decrypted/forwarded.
                if self.path not in ("/api/accounts/oauth/token", "/oauth/token"):
                    owner.rejected["path"] += 1
                    self.send_error(403)
                    return
                try:
                    length = int(self.headers.get("Content-Length", "0"))
                    if not 0 < length <= 65536 or owner.aborted:
                        self.send_error(409)
                        return
                    body = self.rfile.read(length)
                    if len(body) != length:
                        self.send_error(400)
                        return
                    content_type = self.headers.get("Content-Type", "").split(";", 1)[0]
                    if content_type == "application/json":
                        grant = json.loads(body).get("grant_type")
                    elif content_type == "application/x-www-form-urlencoded":
                        grant = urllib.parse.parse_qs(body.decode()).get("grant_type", [None])[0]
                    else:
                        grant = None
                    if grant != "refresh_token":
                        self.send_error(403)
                        return
                    with owner.lock:
                        record = {"arrived": time.monotonic()}
                        owner.records.append(record)
                        cohort = len(owner.records) <= participants
                    if cohort:
                        owner.barrier.wait()
                    # No shared client or connection serializes the participants.
                    upstream = http.client.HTTPSConnection("auth.openai.com", timeout=60)
                    try:
                        record["started"] = time.monotonic()
                        upstream.request("POST", self.path, body=body, headers={
                            "Content-Type": self.headers.get("Content-Type", "application/json"),
                            "Accept": self.headers.get("Accept", "application/json"),
                        })
                        response = upstream.getresponse()
                        result = response.read((1 << 20) + 1)
                        if len(result) > 1 << 20:
                            raise ValueError("Oversized refresh response")
                        record.update(finished=time.monotonic(), status=response.status)
                        if response.status != 200:
                            owner.aborted = True
                        self.send_response(response.status)
                        self.send_header("Content-Type", response.getheader("Content-Type", "application/json"))
                        self.send_header("Content-Length", str(len(result)))
                        self.send_header("Connection", "close")
                        self.end_headers()
                        self.wfile.write(result)
                    finally:
                        upstream.close()
                except Exception:
                    owner.aborted = True
                    owner.barrier.abort()
                    self.close_connection = True

        class Connect(QuietHandler):
            def do_CONNECT(self):
                supplied = self.headers.get("Proxy-Authorization", "")
                if not hmac.compare_digest(supplied, owner.authorization):
                    owner.rejected["authentication"] += 1
                    self.send_error(407)
                    return
                # Keep this proxy narrowly scoped to the authorized provider.
                if self.path not in ("auth.openai.com:443", "api.openai.com:443", "chatgpt.com:443"):
                    owner.rejected["host"] += 1
                    self.send_error(403)
                    return
                try:
                    self.send_response(200)
                    self.end_headers()
                    if self.path == "auth.openai.com:443":
                        with owner.tls.wrap_socket(self.connection, server_side=True) as tls:
                            tls.settimeout(180)
                            Inspection(tls, self.client_address, self.server)
                    else:
                        # Provider inference traffic stays encrypted end to end.
                        with socket.create_connection((self.path.split(":")[0], 443), timeout=30) as upstream:
                            deadline = time.monotonic() + 300
                            while time.monotonic() < deadline:
                                ready, _, _ = select.select([self.connection, upstream], [], [], 10)
                                for source in ready:
                                    data = source.recv(65536)
                                    if not data:
                                        return
                                    (upstream if source is self.connection else self.connection).sendall(data)
                except Exception:
                    pass
                finally:
                    self.close_connection = True

        class Server(http.server.ThreadingHTTPServer):
            daemon_threads = True

            def handle_error(self, request, client_address):
                # Avoid traceback diagnostics containing request context.
                pass

        self.server = Server((host, 0), Connect)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

    def start(self):
        self.thread.start()
        return self

    def url(self, reachable_host="127.0.0.1"):
        return f"http://validation:{self.password}@{reachable_host}:{self.server.server_port}"

    def evidence(self):
        with self.lock:
            records = [dict(record) for record in self.records]
        cohort = records[:self.barrier.parties]
        overlap = (len(cohort) == self.barrier.parties and
                   all("finished" in record for record in cohort) and
                   max(record["started"] for record in cohort) <
                   min(record["finished"] for record in cohort))
        return {"overlap": overlap, "aborted": self.aborted,
                "rejected": dict(self.rejected),
                "statuses": [record.get("status") for record in records],
                "requests": records}

    def close(self):
        self.barrier.abort()
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
