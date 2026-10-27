"""Throwaway TLS material and server settings for the runner's TLS plus scoped-ACL fixture mode.

The runner does not start the Nomad server. `security-material` writes a throwaway CA, a server certificate for
127.0.0.1 and localhost, and a server config fragment; the operator starts the throwaway server with that fragment,
bootstraps ACLs, and keeps the management token in nomad.token_file for runner setup, teardown and assertions only.
"""

from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path
from typing import Any, Callable

from nomad_native_e2e import PreflightError

CA_DAYS = 3


def render_server_config(directory: Path) -> str:
    """Server config fragment for a loopback throwaway server: TLS (HTTP only; rpc TLS is mutual and would block a client node without certs), ACLs on, spread, no oversubscription."""
    return f'''tls {{
  http                   = true
  rpc                    = false
  ca_file   = "{directory}/ca.pem"
  cert_file = "{directory}/server.pem"
  key_file  = "{directory}/server.key"
  verify_server_hostname = false
  verify_https_client = false
}}

acl {{
  enabled = true
}}

server {{
  default_scheduler_config {{
    scheduler_algorithm             = "spread"
    memory_oversubscription_enabled = false
  }}
}}
'''


def _write_private(path: Path, text: str) -> None:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as handle:
        handle.write(text)


def generate_material(out: Path, run: Callable[..., Any] = subprocess.run) -> dict[str, str]:
    """Writes ca.pem, server.pem, server.key and server.hcl into a new private directory; returns their paths."""
    out = Path(out)
    out.mkdir(mode=0o700, parents=True, exist_ok=False)
    out.chmod(0o700)
    ca_key, ca_pem, key, csr, pem = (out / name for name in ("ca.key", "ca.pem", "server.key", "server.csr", "server.pem"))
    ext = out / "server.ext"
    _write_private(ext, "subjectAltName=DNS:localhost,IP:127.0.0.1\nextendedKeyUsage=serverAuth,clientAuth\nbasicConstraints=CA:FALSE\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid\n")
    steps = [
        ["openssl", "req", "-sha256", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1", "-pkeyopt", "ec_param_enc:named_curve", "-nodes", "-days", str(CA_DAYS), "-subj", "/CN=kandev-fixture-ca",
         "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", "-addext", "subjectKeyIdentifier=hash", "-keyout", str(ca_key), "-out", str(ca_pem)],
        ["openssl", "req", "-sha256", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1", "-pkeyopt", "ec_param_enc:named_curve", "-nodes", "-subj", "/CN=kandev-fixture-server", "-keyout", str(key), "-out", str(csr)],
        ["openssl", "x509", "-sha256", "-req", "-in", str(csr), "-CA", str(ca_pem), "-CAkey", str(ca_key), "-CAcreateserial", "-days", str(CA_DAYS),
         "-extfile", str(ext), "-out", str(pem)],
    ]
    for argv in steps:
        result = run(argv, capture_output=True, text=True, timeout=60, check=False)
        if result.returncode != 0:
            raise PreflightError(f"openssl {argv[1]} failed while creating the throwaway TLS material")
    for path in (ca_key, key, pem, ca_pem):
        if path.exists():
            path.chmod(0o600)
    server_config = out / "server.hcl"
    _write_private(server_config, render_server_config(out))
    return {"ca_file": str(ca_pem), "cert_file": str(pem), "key_file": str(key), "server_config": str(server_config)}


def material_command(out_text: str) -> int:
    try:
        files = generate_material(Path(out_text))
    except (PreflightError, OSError) as exc:
        print(f"security-material failed: {exc}", file=sys.stderr)
        return 2
    print("throwaway TLS material written; start the throwaway server with server_config, then:")
    for name, path in files.items():
        print(f"  {name}: {path}")
    print("  note: rpc TLS is off in the fragment (it is mutual and a client node would need its own certs); only HTTP TLS matters for Kandev's ca_file")
    print("  nomad acl bootstrap   # keep the management token in nomad.token_file; the runner creates the scoped Kandev token")
    print("  nomad namespace apply kandev && nomad node pool apply -description kandev kandev   # or the pool spec file")
    print("  runner config: nomad.address https://127.0.0.1:<port>, nomad.namespace kandev, pool.node_pool kandev, security {tls, acl_policy, ca_file}")
    return 0
