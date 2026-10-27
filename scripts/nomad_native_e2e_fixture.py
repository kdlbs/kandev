"""Create-and-crash driver for the native Nomad pre-ready recovery scenario.

One invocation creates a fresh private fixture, runs its own headless Kandev
through normal APIs, SIGKILLs the backend at the deterministic pre-publish
barrier, restarts it, asserts recovery, then stops the session and records the
retained allocation and fence as an operator teardown. The first failed
assertion stops the run without cleanup;
`cleanup-fixture` then performs the teardown against the retained fixture.

Nomad access is HTTP GET, with two operator-teardown mutations: ending a job the runner recorded as
fixture-created (DELETE /v1/job/<id> with purge disabled) and compare-owner removal of the fixture's
own worker fence over pinned SSH. The WO06 admission and disconnect scenarios (nomad_native_e2e_wo06) are the
only callers of those mutations; neither is production cleanup.
"""

from __future__ import annotations

import contextlib
import hashlib
import importlib.util
import json
import os
import posixpath
import re
import secrets
import shlex
import shutil
import signal
import ssl
import socket
import sqlite3
import stat
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable

import nomad_native_e2e as base
import nomad_native_e2e_bootstrap as bootstrap
from nomad_native_e2e import PreflightError
from nomad_profile_payload import build_profile_payload, validate_profile_source

PROCESS_LABEL = "kandev-fixture"
BARRIER_ENV = "KANDEV_FIXTURE_NOMAD_PRE_PUBLISH_BARRIER"
FIXTURE_ONLY_ENV = "KANDEV_FIXTURE_ONLY"
SERVER_HOST_ENV = "KANDEV_SERVER_HOST"
LOOPBACK_HOST = "127.0.0.1"
MARKER_KEYS = {"execution_id", "environment_id", "session_id"}
MARKER_TIMEOUT = 180.0
API_READY_TIMEOUT = 60.0
TERMINAL_ALLOCATION_STATES = {"complete", "failed", "lost"}
STATE_FILE = "fixture.json"
EVIDENCE_FILE = "evidence.json"
_INVENTORY_KEYS = (
    "environment_id", "generation", "operation_id", "execution_id", "job_id", "job_create_index",
    "job_definition_index", "allocation_id", "node_id", "workspace_path", "server_identity", "phase",
)
_JOB_ID = re.compile(r"kandev-[A-Za-z0-9]+")
_PS_ROW = re.compile(r"\s*(\d+)\s+(\d+)\s+(\d+)\s+(\w{3}\s+\w{3}\s+\d+\s+[\d:]{8}\s+\d{4})\s+(.*)$")


def cleanup_command(root: Path | str) -> str:
    return f"python3 scripts/nomad_native_e2e.py cleanup-fixture --fixture-root {shlex.quote(str(root))}"


GIT_DAEMON_LABEL = "git-daemon"
_GROUP_KILL_SAFE = ("process group still has members", "did not stop before timeout")
FIXTURE_REPO_OWNER, FIXTURE_REPO_NAME = "fixture-owner", "fixture-repo"
INERT_AGENT_NAME = "nomad-fixture-inert"
INERT_ACP_SCRIPT = Path(__file__).resolve().parent.parent / "apps/backend/internal/agent/nomad/integration/fixtures/inert_acp.py"


def _fixture_argv(binary: str) -> list[str]:
    return [binary, "__backend"]


def _default_free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def _default_port_open(port: int) -> bool:
    with socket.socket() as probe:
        probe.settimeout(0.5)
        return probe.connect_ex(("127.0.0.1", port)) == 0


def _default_pid_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def _default_listener_pids(port: int) -> list[int]:
    result = subprocess.run(["/usr/sbin/lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN", "-t"], capture_output=True, text=True, timeout=5, check=False)
    return [int(line) for line in result.stdout.splitlines() if line.isdigit()]


def _default_listener_hosts(port: int) -> set[str]:
    """Addresses the TCP listeners on `port` are bound to, as printed by lsof's machine-readable output."""
    result = subprocess.run(["/usr/sbin/lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN", "-Fn"], capture_output=True, text=True, timeout=5, check=False)
    return {line[1:].rsplit(":", 1)[0].strip("[]") for line in result.stdout.splitlines() if line.startswith("n") and ":" in line}


def _default_registry(root: Path) -> Any:
    spec = importlib.util.spec_from_file_location("owned_processes", Path(__file__).with_name("owned_processes.py"))
    if spec is None or spec.loader is None:
        raise PreflightError("fixture process registry is unavailable")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.OwnedProcessRegistry(root)


def _default_read_db(database: str, fn: Callable[[sqlite3.Connection], Any]) -> Any:
    db = sqlite3.connect(f"file:{database}?mode=ro", uri=True, timeout=3)
    try:
        return fn(db)
    finally:
        db.close()


def _default_api(port: int) -> Callable[[str, str, Any], Any]:
    prefix = f"http://127.0.0.1:{port}/api/v1"
    interlock: dict[str, str] = {}

    def call(method: str, path: str, body: Any = None) -> Any:
        data = None if body is None else json.dumps(body).encode()
        headers = {"Accept": "application/json", "Content-Type": "application/json"}
        if method != "GET":
            # Settings mutations require the per-boot SPA interlock token from the boot payload.
            if "token" not in interlock:
                interlock["token"] = call("GET", "/app-state?path=%2Fsettings%2Fagents").get("interimSettingsInterlockToken", "")
            if interlock["token"]:
                headers["X-Kandev-Interim-Settings-Interlock"] = interlock["token"]
        request = urllib.request.Request(prefix + path, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                raw = response.read()
        except urllib.error.HTTPError as exc:
            raise PreflightError(f"fixture API {method} {path.split('?')[0]} failed with status {exc.code}") from None
        except (OSError, urllib.error.URLError):
            raise PreflightError(f"fixture API {method} {path.split('?')[0]} is unreachable") from None
        return json.loads(raw) if raw else {}

    return call


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Nomad answers directly; following a redirect would forward X-Nomad-Token to another origin."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


_NOMAD_OPENER = urllib.request.build_opener(_NoRedirect)
_TLS_OPENERS: dict[str, urllib.request.OpenerDirector] = {}


def configure_nomad_tls(address: str, ca_file: str) -> None:
    """Verify the https Nomad API at `address` against the throwaway CA only (hostname checks stay on)."""
    context = ssl.create_default_context(cafile=ca_file)
    _TLS_OPENERS[address] = urllib.request.build_opener(_NoRedirect, urllib.request.HTTPSHandler(context=context))


def clear_nomad_tls() -> None:
    _TLS_OPENERS.clear()


def _nomad_opener(address: str) -> urllib.request.OpenerDirector:
    return _TLS_OPENERS.get(address, _NOMAD_OPENER)


def _default_nomad_get(address: str, token: str, namespace: str, region: str) -> Callable[[str], Any]:
    def call(path: str) -> Any:
        sep = "&" if "?" in path else "?"
        request = urllib.request.Request(f"{address}{path}{sep}namespace={namespace}&region={region}", method="GET", headers={"X-Nomad-Token": token, "Accept": "application/json"})
        try:
            with _nomad_opener(address).open(request, timeout=10) as response:
                return json.loads(response.read())
        except urllib.error.HTTPError as exc:
            raise PreflightError(f"Nomad GET {path.split('?')[0]} failed with status {exc.code}") from None
        except (OSError, urllib.error.URLError):
            raise PreflightError(f"Nomad GET {path.split('?')[0]} is unreachable") from None

    return call


def _default_keyscan(host: str, port: int) -> list[str]:
    result = subprocess.run(["/usr/bin/ssh-keyscan", "-p", str(port), "-T", "3", host], capture_output=True, text=True, timeout=15, check=False)
    return [line for line in result.stdout.splitlines() if line.strip() and not line.startswith("#")]


def _default_key_fingerprint(key_line: str) -> str:
    result = subprocess.run(["/usr/bin/ssh-keygen", "-lf", "-"], input=key_line + "\n", capture_output=True, text=True, timeout=10, check=False)
    parts = result.stdout.split()
    return parts[1] if result.returncode == 0 and len(parts) > 1 and parts[1].startswith("SHA256:") else ""


def pin_host_key(ops: "Ops", ssh: dict[str, Any], known_hosts: Path) -> None:
    """Writes only the scanned key matching the pinned fingerprint; fails before any remote command."""
    host = ssh["target"].rsplit("@", 1)[-1]
    matching = [line for line in ops.keyscan(host, ssh["port"]) if ops.key_fingerprint(line) == ssh["host_fingerprint"]]
    if not matching:
        raise PreflightError("SSH host did not present a key matching the pinned fingerprint")
    _write_private(known_hosts, matching[0] + "\n")


def _default_local_exec(command: str) -> tuple[int, str]:
    result = subprocess.run(["/bin/sh", "-c", command], capture_output=True, text=True, timeout=20, check=False)
    return result.returncode, result.stdout


def _default_ssh(ssh: dict[str, Any]) -> Callable[[str], tuple[int, str]]:
    def call(command: str) -> tuple[int, str]:
        result = subprocess.run(
            ["/usr/bin/ssh", "-F", "/dev/null", "-i", ssh["identity_file"], "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes",
             "-o", "StrictHostKeyChecking=yes", "-o", f"UserKnownHostsFile={ssh['known_hosts']}",
             "-o", "ConnectTimeout=5", "-p", str(ssh["port"]), ssh["target"], command],
            capture_output=True, text=True, timeout=20, check=False,
        )
        return result.returncode, result.stdout

    return call


def _default_ws(payload: dict[str, Any]) -> dict[str, Any]:
    script = Path(__file__).with_name("nomad_native_e2e_ws.js")
    result = subprocess.run(["node", str(script)], input=json.dumps(payload), text=True, capture_output=True, timeout=120, check=False)
    if result.returncode != 0:
        raise PreflightError(f"fixture WebSocket driver {payload['mode']} failed")
    try:
        return json.loads(result.stdout.strip().splitlines()[-1])
    except (json.JSONDecodeError, IndexError):
        raise PreflightError("fixture WebSocket driver result is unreadable") from None


def stop_owned_process(registry: Any, ops: Any, label: str) -> str:
    """Stop one owned process: SIGTERM within a bound, then SIGKILL of its recorded group.

    The group is signalled only after the registry verified the recorded leader identity and reported that the
    leader outlived the bound or its group still has members; any other refusal leaves the process untouched.
    """
    try:
        return registry.stop(label, timeout=15)
    except Exception as exc:
        record = next((item for item in registry.read_records() if item["label"] == label), None)
        if record is None or not any(marker in str(exc) for marker in _GROUP_KILL_SAFE):
            return "unverified"
    try:
        ops.kill_group(record["pgid"])
        ops.sleep(0.5)
        return registry.stop(label, timeout=15)
    except Exception:
        return "unverified"


def _default_kill_group(pgid: int) -> None:
    if pgid == os.getpgrp() or pgid == os.getpgid(os.getppid()):
        raise PreflightError("refusing to signal the process group of this runner or its caller")
    try:
        os.killpg(pgid, signal.SIGKILL)
    except ProcessLookupError:
        pass


def _default_process_command(pid: int) -> str | None:
    """Command line of a local pid, or None when it does not exist."""
    out = subprocess.run(["ps", "-p", str(pid), "-ww", "-o", "command="], capture_output=True, text=True, env={**os.environ, "LC_ALL": "C"}, check=False).stdout
    return out.strip() or None


def _default_signal_local(pid: int, sig: str) -> None:
    """Send STOP or CONT to one local pid. Callers verify the process identity first."""
    if sig not in ("STOP", "CONT"):
        raise PreflightError("only STOP and CONT may be sent to a fixture process")
    if not isinstance(pid, int) or isinstance(pid, bool) or pid <= 1 or pid == os.getpid():
        raise PreflightError("refusing to signal this pid")
    os.kill(pid, signal.SIGSTOP if sig == "STOP" else signal.SIGCONT)


def _default_nomad_stop(address: str, token: str, namespace: str, job_id: str) -> Any:
    """Operator end of one fixture job: purge is always false so the job and its allocations stay inspectable."""
    if not _JOB_ID.fullmatch(job_id):
        raise PreflightError("refusing to end a job that does not follow the kandev-<hash> pattern")
    request = urllib.request.Request(f"{address}/v1/job/{job_id}?purge=false&namespace={namespace}", method="DELETE", headers={"X-Nomad-Token": token, "Accept": "application/json"})
    try:
        with _nomad_opener(address).open(request, timeout=15) as response:
            return json.loads(response.read() or b"{}")
    except urllib.error.HTTPError as exc:
        raise PreflightError(f"Nomad DELETE failed with status {exc.code}") from None
    except (OSError, urllib.error.URLError):
        raise PreflightError("Nomad DELETE is unreachable") from None


def _default_nomad_set_eligibility(address: str, token: str, region: str, node_id: str, eligible: bool) -> Any:
    """Operator toggle of the fixture node's scheduling eligibility; it never drains or deletes the node."""
    if not re.fullmatch(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", node_id):
        raise PreflightError("refusing to change eligibility of a malformed node id")
    body = json.dumps({"NodeID": node_id, "Eligibility": "eligible" if eligible else "ineligible"}).encode()
    request = urllib.request.Request(f"{address}/v1/node/{node_id}/eligibility?region={region}", data=body, method="POST",
                                     headers={"X-Nomad-Token": token, "Accept": "application/json", "Content-Type": "application/json"})
    try:
        with _nomad_opener(address).open(request, timeout=15) as response:
            return json.loads(response.read() or b"{}")
    except urllib.error.HTTPError as exc:
        raise PreflightError(f"Nomad eligibility change failed with status {exc.code}") from None
    except (OSError, urllib.error.URLError):
        raise PreflightError("Nomad eligibility change is unreachable") from None


def _default_kill_exact(ssh: Callable[[str], tuple[int, str]], identity: dict[str, Any], needles: list[str]) -> str:
    """SIGKILL one remote process only when its pid, start time and command still match the recorded identity.

    Returns "killed" or "already_gone"; any identity mismatch raises before a signal is sent.
    """
    pid = identity.get("pid")
    if not isinstance(pid, int) or isinstance(pid, bool) or pid <= 1:
        raise PreflightError("refusing to signal a process without a recorded pid")

    def present() -> bool:
        """True while the pid still carries the recorded identity; False once it is gone; a different identity raises."""
        _, out = ssh(f"LC_ALL=C ps -p {pid} -ww -o pid=,ppid=,pgid=,lstart=,command=")
        rows = [match for match in (_PS_ROW.match(line) for line in out.splitlines()) if match and int(match.group(1)) == pid]
        if not rows:
            return False
        command = rows[0].group(5)
        started = " ".join(rows[0].group(4).split())
        matches = any(needle and needle in command for needle in needles) or (identity.get("cmd_sha256") and hashlib.sha256(command.encode()).hexdigest() == identity["cmd_sha256"])
        if started != identity.get("started") or not matches:
            raise PreflightError(f"process {pid} no longer matches its recorded identity; refusing to signal it")
        return True

    if not present():
        return "already_gone"
    code, _ = ssh(f"kill -KILL {pid}")
    if code != 0:
        # The process can exit between the identity check and the signal, for example when its parent was killed first.
        if not present():
            return "already_gone"
        raise PreflightError(f"signalling process {pid} failed")
    return "killed"


def _default_signal_pid(ssh: Callable[[str], tuple[int, str]], pid: int, sig: str) -> str:
    """Send STOP or CONT to one remote pid over the existing ssh channel. Callers verify the process identity first."""
    if sig not in ("STOP", "CONT"):
        raise PreflightError("only STOP and CONT may be sent to a fixture process")
    if not isinstance(pid, int) or isinstance(pid, bool) or pid <= 1:
        raise PreflightError("refusing to signal a process without a recorded pid")
    code, _ = ssh(f"kill -{sig} {pid}")
    if code != 0:
        raise PreflightError(f"signalling process {pid} failed")
    return "signalled"


_QUARANTINE_VARIABLE = re.compile(r"kandev/quarantine/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")


def _default_nomad_delete_variable(address: str, token: str, namespace: str, path: str) -> str:
    """Operator removal of one node quarantine Variable; any other variable path is refused. Returns removed or absent."""
    if not _QUARANTINE_VARIABLE.fullmatch(path):
        raise PreflightError("refusing to delete a Nomad variable that is not a node quarantine marker")
    quoted = urllib.parse.quote(path, safe="")
    request = urllib.request.Request(f"{address}/v1/var/{quoted}?namespace={namespace}", method="DELETE", headers={"X-Nomad-Token": token, "Accept": "application/json"})
    try:
        with _nomad_opener(address).open(request, timeout=15):
            return "removed"
    except urllib.error.HTTPError as exc:
        if exc.code == 404:
            return "absent"
        raise PreflightError(f"Nomad variable DELETE failed with status {exc.code}") from None
    except (OSError, urllib.error.URLError):
        raise PreflightError("Nomad variable DELETE is unreachable") from None


def redact_path(text: str, root: Path | None = None) -> str:
    """Replaces the fixture root and the home directory in operator-facing text so evidence carries no local layout."""
    if root is not None:
        text = text.replace(str(root), "<fixture-root>")
    home = str(Path.home())
    return text.replace(home, "~") if home not in ("", "/") else text


_ACL_NAME = re.compile(r"[a-z][a-z0-9-]{0,62}")
_ACL_ACCESSOR = re.compile(r"[A-Za-z0-9-]{1,64}")
ACL_POLICY_NAME = "kandev-executor"
ACL_TOKEN_TTL = "24h"


def _acl_call(address: str, token: str, method: str, path: str, body: Any = None, tolerate_absent: bool = False) -> Any:
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(f"{address}{path}", data=data, method=method,
                                     headers={"X-Nomad-Token": token, "Accept": "application/json", "Content-Type": "application/json"})
    try:
        with _nomad_opener(address).open(request, timeout=15) as response:
            return json.loads(response.read() or b"{}")
    except urllib.error.HTTPError as exc:
        if tolerate_absent and exc.code == 404:
            return {}
        raise PreflightError(f"Nomad ACL {method} {path.rsplit('/', 1)[0]} failed with status {exc.code}") from None
    except (OSError, urllib.error.URLError):
        raise PreflightError(f"Nomad ACL {method} {path.rsplit('/', 1)[0]} is unreachable") from None


def _default_nomad_acl_provision(address: str, token: str, policy_name: str, rules: str) -> dict[str, Any]:
    """Management-token setup of the scoped Kandev token: the policy is written, then one client token bound to it alone."""
    if not _ACL_NAME.fullmatch(policy_name) or policy_name == "management":
        raise PreflightError("refusing to provision an ACL policy with an unsafe name")
    _acl_call(address, token, "POST", f"/v1/acl/policy/{policy_name}", {"Name": policy_name, "Description": "kandev fixture executor", "Rules": rules})
    issued = _acl_call(address, token, "POST", "/v1/acl/token", {"Name": "kandev-fixture-executor", "Type": "client", "Policies": [policy_name], "Global": False, "ExpirationTTL": ACL_TOKEN_TTL})
    return {"secret": issued.get("SecretID"), "accessor": issued.get("AccessorID"), "policies": issued.get("Policies"), "type": issued.get("Type")}


def _default_nomad_acl_revoke(address: str, token: str, accessor: str, policy_name: str) -> None:
    """Management-token removal of the scoped token and its policy; already-absent objects are fine."""
    if not _ACL_ACCESSOR.fullmatch(accessor) or not _ACL_NAME.fullmatch(policy_name) or policy_name == "management":
        raise PreflightError("refusing to revoke an ACL object with an unsafe name")
    _acl_call(address, token, "DELETE", f"/v1/acl/token/{accessor}", tolerate_absent=True)
    _acl_call(address, token, "DELETE", f"/v1/acl/policy/{policy_name}", tolerate_absent=True)


def _default_prompt(payload: dict[str, Any]) -> dict[str, Any]:
    """Send one inert prompt through the normal message API (the same driver the recovery scenario uses)."""
    script = Path(__file__).with_name("nomad_native_e2e_recovery.js")
    result = subprocess.run(["node", str(script)], input=json.dumps(payload), text=True, capture_output=True, timeout=25, check=False)
    if result.returncode != 0:
        raise PreflightError("fixture prompt was not accepted")
    try:
        return json.loads(result.stdout.strip().splitlines()[-1])
    except (json.JSONDecodeError, IndexError):
        raise PreflightError("fixture prompt acknowledgment is unreadable") from None


def _default_git_binary() -> str:
    path = shutil.which("git")
    if not path:
        raise PreflightError("git is not installed")
    return os.path.realpath(path)


def _default_git_bare_clone(source: str, dest: str) -> None:
    result = subprocess.run([_default_git_binary(), "clone", "--bare", "--quiet", source, dest], capture_output=True, text=True, timeout=120, check=False)
    if result.returncode != 0:
        raise PreflightError("git bare clone of the fixture repository failed")


def _default_git_head(path: str, ref: str) -> str:
    result = subprocess.run([_default_git_binary(), "-C", path, "rev-parse", "--verify", f"{ref}^{{commit}}"], capture_output=True, text=True, timeout=30, check=False)
    head = result.stdout.strip()
    if result.returncode != 0 or not re.fullmatch(r"[0-9a-f]{40}", head):
        raise PreflightError("could not resolve the fixture repository base branch commit")
    return head


@dataclass
class Ops:
    """Every external effect, injectable for offline tests."""
    free_port: Callable[[], int] = _default_free_port
    port_open: Callable[[int], bool] = _default_port_open
    pid_alive: Callable[[int], bool] = _default_pid_alive
    listener_pids: Callable[[int], list[int]] = _default_listener_pids
    listener_hosts: Callable[[int], set[str]] = _default_listener_hosts
    registry: Callable[[Path], Any] = _default_registry
    read_db: Callable[[str, Callable[[sqlite3.Connection], Any]], Any] = _default_read_db
    api_factory: Callable[[int], Callable[[str, str, Any], Any]] = _default_api
    nomad_factory: Callable[[str, str, str, str], Callable[[str], Any]] = _default_nomad_get
    keyscan: Callable[[str, int], list[str]] = _default_keyscan
    key_fingerprint: Callable[[str], str] = _default_key_fingerprint
    ssh_factory: Callable[[dict[str, Any]], Callable[[str], tuple[int, str]]] = _default_ssh
    local_exec: Callable[[str], tuple[int, str]] = _default_local_exec
    ws: Callable[[dict[str, Any]], dict[str, Any]] = _default_ws
    git_bare_clone: Callable[[str, str], None] = _default_git_bare_clone
    git_binary: Callable[[], str] = _default_git_binary
    git_head: Callable[[str, str], str] = _default_git_head
    kill_group: Callable[[int], None] = _default_kill_group
    prompt: Callable[[dict[str, Any]], dict[str, Any]] = _default_prompt
    retention_settle: float = 5.0
    clock: Callable[[], float] = time.monotonic
    sleep: Callable[[float], Any] = time.sleep
    wall: Callable[[], str] = lambda: datetime.now(timezone.utc).isoformat(timespec="seconds")
    marker_timeout: float = MARKER_TIMEOUT
    allocation_timeout: float = 120.0
    fence_timeout: float = 60.0
    recovery_timeout: float = 120.0
    nomad_stop: Callable[[str, str, str, str], Any] = _default_nomad_stop
    kill_exact: Callable[[Callable[[str], tuple[int, str]], dict[str, Any], list[str]], str] = _default_kill_exact
    nomad_set_eligibility: Callable[[str, str, str, str, bool], Any] = _default_nomad_set_eligibility
    relay_factory: Callable[[tuple[str, int], tuple[str, int]], Any] | None = None
    signal_pid: Callable[[Callable[[str], tuple[int, str]], int, str], str] = _default_signal_pid
    process_command: Callable[[int], str | None] = _default_process_command
    signal_local: Callable[[int, str], None] = _default_signal_local
    nomad_delete_variable: Callable[[str, str, str, str], str] = _default_nomad_delete_variable
    nomad_acl_provision: Callable[[str, str, str, str], dict[str, Any]] = _default_nomad_acl_provision
    nomad_acl_revoke: Callable[[str, str, str, str], None] = _default_nomad_acl_revoke
    scenario_timeout: float = 300.0
    poll_interval: float = 1.0


@dataclass
class Evidence:
    root: Path
    secrets: list[str]
    data: dict[str, Any] = field(default_factory=lambda: {"status": "running", "steps": []})

    def scrub(self, text: str) -> str:
        for secret in self.secrets:
            if secret:
                text = text.replace(secret, "[redacted]")
        return text

    def render(self) -> str:
        return self.scrub(json.dumps(self.data, sort_keys=True, indent=2))

    def save(self) -> None:
        _write_private(self.root / EVIDENCE_FILE, self.render() + "\n")


def _write_private(path: Path, text: str) -> None:
    tmp = path.with_name(path.name + ".tmp")
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as handle:
        handle.write(text)
    os.replace(tmp, path)


def _mkdir_private(path: Path) -> Path:
    path.mkdir(mode=0o700)
    path.chmod(0o700)
    return path


def _save_state(root: Path, state: dict[str, Any]) -> None:
    _write_private(root / STATE_FILE, json.dumps(state, sort_keys=True, indent=2) + "\n")


def _remote_state_dir(allocation_root: str, node_id: str) -> str:
    return posixpath.join(posixpath.dirname(posixpath.dirname(allocation_root)), "worker-state", node_id)


def _fence_path(state: dict[str, Any], environment_id: str | None = None) -> str:
    """The start fence. A pool node fences each environment alone under its kandev.state_root (recorded from node
    metadata); until an environment is recorded the node-level environments directory stands in. A pinned worker keeps
    the worker-wide fence."""
    if state.get("pool_state_root"):
        env = environment_id or state.get("fence_environment_id") or "unrecorded"
        return posixpath.join(state["pool_state_root"], "environments", env, "active")
    return posixpath.join(_remote_state_dir(state["allocation_root"], state["node_id"]), "active-environment")


class Runner:
    def __init__(self, root: Path, state: dict[str, Any], token: str, ops: Ops, evidence: Evidence):
        self.root, self.state, self.token, self.ops, self.evidence = root, state, token, ops, evidence
        self.home = root / "home"
        self.registry = ops.registry(root)
        self.api = ops.api_factory(state["port"])
        self.nomad = ops.nomad_factory(state["nomad"]["address"], token, state["nomad"]["namespace"], state["nomad"]["region"])
        known_hosts = root / "known_hosts"
        pin_host_key(ops, state["ssh"], known_hosts)
        self.ssh_remote = ops.ssh_factory({**state["ssh"], "identity_file": str(self.home / ".ssh" / "id_ed25519"), "known_hosts": str(known_hosts)})
        self.active_attach = "ssh"
        self.ssh = self._exec_on_active_node
        self.database = str(self.home / "data" / "kandev.db")

    def _exec_on_active_node(self, command: str) -> tuple[int, str]:
        """Runs on the node the scenario is inspecting: this host for a local node, the SSH target otherwise."""
        return self.ops.local_exec(command) if self.active_attach == "local" else self.ssh_remote(command)

    def use_node(self, attach: str, state_root: str) -> None:
        """Points worker commands and pool fence paths at the node a pool environment was placed on."""
        if attach not in ("local", "ssh"):
            raise PreflightError(f"unknown node attach {attach!r}")
        self.active_attach = attach
        self.save_state(pool_state_root=state_root)

    # -- bookkeeping -------------------------------------------------------
    def save_state(self, **updates: Any) -> None:
        self.state.update(updates)
        _save_state(self.root, self.state)

    def step(self, name: str, fn: Callable[[], Any]) -> Any:
        started, begin = self.ops.wall(), self.ops.clock()
        entry = {"name": name, "started": started, "status": "running"}
        self.evidence.data["steps"].append(entry)
        try:
            result = fn()
        except BaseException:
            entry["status"] = "failed"
            entry["duration_s"] = round(self.ops.clock() - begin, 3)
            raise
        entry["status"] = "passed"
        entry["duration_s"] = round(self.ops.clock() - begin, 3)
        self.evidence.save()
        return result

    def poll(self, timeout: float, interval: float, probe: Callable[[], Any], failure: str) -> Any:
        deadline = self.ops.clock() + timeout
        while True:
            value = probe()
            if value:
                return value
            if self.ops.clock() >= deadline:
                raise PreflightError(failure)
            self.ops.sleep(interval)

    # -- process lifecycle ---------------------------------------------------
    def launch_env(self, barrier: str | None) -> dict[str, str]:
        env = {**self.state["manifest_env"], "KANDEV_BUNDLE_DIR": self.state["bundle"], "PATH": "/usr/bin:/bin:/usr/sbin:/sbin", FIXTURE_ONLY_ENV: "1", SERVER_HOST_ENV: LOOPBACK_HOST}
        if barrier:
            env[BARRIER_ENV] = barrier
        return env

    def start_backend(self, barrier: str | None) -> dict[str, Any]:
        port = self.state["port"]
        if port == bootstrap.LIVE_KANDEV_PORT:
            raise PreflightError("refusing to use the live Kandev port")
        if self.ops.port_open(port):
            raise PreflightError("fixture API port is already in use")
        if "baseline_jobs" not in self.state:
            self.save_state(baseline_jobs=sorted(item["ID"] for item in self.nomad("/v1/jobs?prefix=kandev-") if isinstance(item, dict) and "ID" in item))
        record = self.registry.start(PROCESS_LABEL, _fixture_argv(self.state["kandev_binary"]), cwd=self.home, env=self.launch_env(barrier))
        self.poll(API_READY_TIMEOUT, 0.25, self._api_ready, "fixture backend did not serve its API before timeout")
        if self.ops.listener_pids(port) != [record["pid"]]:
            raise PreflightError("fixture API listener is not exactly the registered Kandev child")
        if self.ops.listener_hosts(port) != {LOOPBACK_HOST}:
            raise PreflightError("fixture API listener is not bound to loopback only")
        return record

    def _api_ready(self) -> bool:
        try:
            self.api("GET", "/workspaces", None)
            return True
        except PreflightError:
            return False

    def verify_gone(self, pid: int) -> None:
        if self.ops.pid_alive(pid):
            raise PreflightError("fixture backend process is still alive")
        if self.ops.port_open(self.state["port"]):
            raise PreflightError("fixture API port is still open")
        if any(item["label"] == PROCESS_LABEL for item in self.registry.read_records()):
            raise PreflightError("fixture backend ownership record was not retired")

    # -- reads -------------------------------------------------------------------
    def inventory(self, session_id: str | None = None, task_id: str | None = None) -> dict[str, Any]:
        session_id, task_id = session_id or self.state["session_id"], task_id or self.state["task_id"]

        def read(db: sqlite3.Connection) -> list[tuple[Any, ...]]:
            return db.execute("SELECT task_id, runtime, agent_execution_id, metadata FROM executors_running WHERE session_id = ?", (session_id,)).fetchall()

        rows = self.ops.read_db(self.database, read)
        if len(rows) != 1:
            raise PreflightError("expected exactly one durable executor inventory row")
        row_task_id, runtime, execution_id, raw = rows[0]
        try:
            nomad_state = json.loads(raw)["nomad_environment_state"]
        except (TypeError, KeyError, json.JSONDecodeError):
            raise PreflightError("Nomad inventory state is unreadable") from None
        if row_task_id != task_id or runtime != "nomad":
            raise PreflightError("durable executor inventory row has the wrong owner or runtime")
        snapshot = {key: nomad_state.get(key) for key in _INVENTORY_KEYS}
        snapshot["execution_id"] = execution_id
        snapshot["workspace_input_captured"] = nomad_state.get("workspace_input_captured")
        return snapshot

    def session_view(self) -> tuple[dict[str, Any], list[dict[str, Any]]]:
        sessions = self.api("GET", f"/tasks/{self.state['task_id']}/sessions", None).get("sessions", [])
        current = [item for item in sessions if item.get("id") == self.state["session_id"]]
        if len(current) != 1:
            raise PreflightError("fixture session is not uniquely listed")
        return current[0], self.api("GET", f"/task-sessions/{self.state['session_id']}/messages", None).get("messages", [])

    def nomad_snapshot(self, inventory: dict[str, Any]) -> dict[str, Any]:
        job_id = inventory["job_id"]
        if not isinstance(job_id, str) or not _JOB_ID.fullmatch(job_id):
            raise PreflightError("observed Nomad job id does not follow the kandev-<hash> pattern")
        node = self.nomad(f"/v1/node/{inventory['node_id']}")
        pooled = bool(self.state.get("pool_state_root"))
        if (not pooled and node.get("ID") != self.state["nomad"]["node_id"]) or node.get("Status") != "ready" or node.get("SchedulingEligibility") != "eligible":
            raise PreflightError("Nomad worker identity is not the configured ready and eligible node")
        job = self.nomad(f"/v1/job/{job_id}")
        job = job.get("Job", job)
        allocations = self.nomad(f"/v1/job/{job_id}/allocations")
        if job.get("ID") != job_id or job.get("Status") != "running":
            raise PreflightError("Nomad job is not the running job from the inventory")
        if len(allocations) != 1 or allocations[0].get("ID") != inventory["allocation_id"]:
            raise PreflightError("Nomad job does not have exactly the inventory allocation")
        allocation = self.nomad(f"/v1/allocation/{inventory['allocation_id']}")
        if allocation.get("NodeID") != inventory["node_id"] or allocation.get("ClientStatus") != "running":
            raise PreflightError("Nomad allocation is not running on the inventory node")
        if allocation.get("TaskStates", {}).get("agentctl", {}).get("State") != "running":
            raise PreflightError("Nomad agentctl task is not running")
        return {
            "job_id": job_id, "job_create_index": job.get("CreateIndex"), "job_definition_index": job.get("JobModifyIndex"),
            "allocation_id": allocation["ID"], "node_id": allocation["NodeID"], "allocation_count": len(allocations),
        }

    def remote_tree(self, inventory: dict[str, Any]) -> list[dict[str, Any]]:
        code, out = self.ssh("LC_ALL=C ps -eww -o pid=,ppid=,pgid=,lstart=,command=")
        if code != 0:
            raise PreflightError("remote process listing failed")
        needles = [inventory["allocation_id"], inventory["job_id"], inventory["environment_id"]]
        rows = []
        for line in out.splitlines():
            match = _PS_ROW.match(line)
            if match and any(needle in match.group(5) for needle in needles):
                rows.append({"pid": int(match.group(1)), "ppid": int(match.group(2)), "pgid": int(match.group(3)), "started": " ".join(match.group(4).split())})
        return sorted(rows, key=lambda row: row["pid"])

    def fence(self, environment_id: str | None = None) -> str | None:
        """SHA-256 of the worker's active-environment fence, or None when it is absent."""
        path = shlex.quote(_fence_path(self.state, environment_id))
        # The bootstrap fence is a directory created by mkdir; its owner file carries the identity.
        code, out = self.ssh(f"if [ -e {path} ]; then cat -- {path}/owner; else exit 3; fi")
        if code == 3:
            return None
        if code != 0:
            raise PreflightError("remote fence read failed")
        return hashlib.sha256(out.encode()).hexdigest()

    def fence_owner(self, environment_id: str | None = None) -> str | None:
        """Raw owner text of the worker's active-environment fence, or None when it is absent."""
        path = shlex.quote(_fence_path(self.state, environment_id))
        code, out = self.ssh(f"if [ -e {path} ]; then cat -- {path}/owner; else exit 3; fi")
        if code == 3:
            return None
        if code != 0:
            raise PreflightError("remote fence read failed")
        return out

    # -- security mode -------------------------------------------------------------
    def verify_security_cluster(self) -> dict[str, Any]:
        """The management token must be a management token and the cluster must be shaped like the deployment."""
        if self.nomad("/v1/acl/token/self").get("Type") != "management":
            raise PreflightError("security mode: nomad.token_file must hold the ACL management token (setup, teardown and assertions only)")
        scheduler = (self.nomad("/v1/operator/scheduler/configuration") or {}).get("SchedulerConfig") or {}
        if scheduler.get("SchedulerAlgorithm") != "spread":
            raise PreflightError(f"security mode: the Nomad scheduler algorithm is {scheduler.get('SchedulerAlgorithm')!r}, expected 'spread'")
        if scheduler.get("MemoryOversubscriptionEnabled"):
            raise PreflightError("security mode: Nomad memory oversubscription is enabled, expected off")
        for kind, path in (("namespace", f"/v1/namespace/{bootstrap.SECURITY_NAMESPACE}"), ("node pool", f"/v1/node/pool/{bootstrap.SECURITY_NODE_POOL}")):
            try:
                self.nomad(path)
            except PreflightError as exc:
                raise PreflightError(f"security mode: the {kind} {bootstrap.SECURITY_NAMESPACE} does not exist on the Nomad server ({exc})") from None
        return {"scheduler_algorithm": "spread", "memory_oversubscription": False, "namespace": bootstrap.SECURITY_NAMESPACE, "node_pool": bootstrap.SECURITY_NODE_POOL}

    def provision_scoped_token(self, cfg: dict[str, Any]) -> str:
        """Creates the scoped Kandev token from the policy file with the management token; Kandev receives only this one."""
        cached = getattr(self, "_scoped_secret", None)
        if cached:
            return cached
        report = self.verify_security_cluster()
        address = self.state["nomad"]["address"]
        policy_name = f"{ACL_POLICY_NAME}-{secrets.token_hex(4)}"
        issued = self.ops.nomad_acl_provision(address, self.token, policy_name, Path(cfg["security"]["acl_policy"]).read_text(encoding="utf-8"))
        if issued.get("accessor"):
            self.save_state(scoped_token={"accessor": issued["accessor"], "policy": policy_name})
        secret = issued.get("secret")
        if issued.get("policies") != [policy_name] or issued.get("type") != "client" or not secret or secret == self.token:
            raise PreflightError("security mode: the Kandev token is not a client token scoped to the kandev-executor policy alone")
        self.evidence.secrets.append(secret)
        self._scoped_secret = secret
        self.evidence.data["security"] = {**report, "tls": True, "kandev_token_policies": issued["policies"], "kandev_token_accessor": issued["accessor"]}
        return secret

    def revoke_scoped_token(self, report: dict[str, Any]) -> None:
        """Removes the scoped token and policy once Kandev has stopped; a no-op outside security mode."""
        scoped = self.state.get("scoped_token")
        if not scoped:
            return
        self.ops.nomad_acl_revoke(self.state["nomad"]["address"], self.token, scoped["accessor"], scoped["policy"])
        self.save_state(scoped_token=None)
        report["scoped_token_revoked"] = True

    # -- create phase ----------------------------------------------------------------
    def setup_api_objects(self, cfg: dict[str, Any]) -> None:
        api = self.api
        kandev_token = self.provision_scoped_token(cfg) if cfg.get("security") else self.token
        workspace = self._first(api("GET", "/workspaces", None).get("workspaces"), "workspace")
        workflow = self._first(api("GET", f"/workspaces/{workspace['id']}/workflows", None).get("workflows"), "workflow")
        step = self._first(api("GET", f"/workflows/{workflow['id']}/workflow/steps", None).get("steps"), "workflow step")
        profiles = [p for agent in api("GET", "/agents", None).get("agents", []) for p in agent.get("profiles", [])]
        wanted = cfg["agent_profile_name"]
        profiles = [p for p in profiles if wanted is None or p.get("name") == wanted]
        if not profiles:
            profiles = [self._create_inert_profile(wanted or INERT_AGENT_NAME)]
        if len(profiles) != 1:
            raise PreflightError("expected exactly one matching agent profile on the fresh fixture")
        self._redirect_git(workspace["id"], profiles[0]["id"])
        secret = api("POST", "/secrets", {"name": "nomad-fixture-token", "value": kandev_token})
        nomad = cfg["nomad"]
        token_ref = f"$kandev-secret:{secret['id']}"
        nomad_config = {
            "address": nomad["address"], "namespace": nomad["namespace"], "region": nomad["region"], "datacenter": nomad["datacenter"],
            "node_id": nomad["node_id"], "platform": nomad["platform"], "ssh_target": cfg["ssh"]["target"], "ssh_port": str(cfg["ssh"]["port"]),
            "ssh_host_fingerprint": cfg["ssh"]["host_fingerprint"], "workspace_root": cfg["workspace_root"],
            "allocation_root": cfg["allocation_root"], "cpu_mhz": str(cfg["cpu_mhz"]), "memory_mb": str(cfg["memory_mb"]),
        }
        if cfg.get("pool"):
            nomad_config = self.pool_profile_config(cfg)
        executors = api("GET", "/executors", None).get("executors", [])
        executor = next((item for item in executors if item.get("type") == "nomad"), None) or api("POST", "/executors", {
            "name": "nomad-fixture", "type": "nomad", "status": "active", "is_system": False, "resumable": True,
            "config": {**nomad_config, "nomad_token": token_ref},
        })
        executor = executor.get("executor", executor)
        source = validate_profile_source({
            "schema_version": 1, "executor_id": executor["id"], "profile_id": str(uuid.uuid4()), "profile_name": "nomad-fixture-profile",
            "nomad_token_ref": token_ref, "config": nomad_config,
        })
        profile = api("POST", f"/executors/{executor['id']}/profiles", {**build_profile_payload(source), "mcp_policy": "", "prepare_script": "", "cleanup_script": ""})
        profile = profile.get("profile", profile)
        repository = self._register_repository(cfg, workspace["id"])
        self._task_ctx = {
            "workspace_id": workspace["id"], "workflow_id": workflow["id"], "workflow_step_id": step["id"],
            "repositories": [{"repository_id": repository["id"], "base_branch": cfg["repository"]["base_branch"]}],
            "agent_profile_id": profiles[0]["id"], "executor_id": executor["id"], "executor_profile_id": profile["id"],
        }
        task = self.create_task("Nomad pre-ready recovery fixture", "Pre-ready recovery fixture; no prompt is sent.")
        self.save_state(workspace_id=workspace["id"], task_id=task["id"], session_id=task["session_id"], repository_id=repository["id"], executor_profile_id=profile["id"])

    @staticmethod
    def pool_profile_config(cfg: dict[str, Any]) -> dict[str, str]:
        """A pool profile names a node pool and sizing only: no node id, SSH target or worker paths."""
        nomad, pool = cfg["nomad"], cfg["pool"]
        config = {key: nomad[key] for key in ("address", "namespace", "region", "datacenter", "platform")}
        config.update(node_pool=pool["node_pool"], cpu_mhz=str(cfg["cpu_mhz"]), memory_mb=str(cfg["memory_mb"]))
        for key in ("placement", "ssh_user", "memory_max_mb"):
            if key in pool:
                config[key] = str(pool[key])
        if cfg.get("security"):
            config["ca_file"] = cfg["security"]["ca_file"]
        return config

    def create_task(self, title: str, description: str = "Nomad fixture task; no prompt is sent.") -> dict[str, str]:
        """Create one more task on the executor profile prepared by setup_api_objects."""
        task = self.api("POST", "/tasks", {**self._task_ctx, "title": title, "description": description, "prepare_session": True})
        if not task.get("id") or not task.get("session_id"):
            raise PreflightError("task creation did not return a task and session")
        return {"id": task["id"], "session_id": task["session_id"]}

    def _create_inert_profile(self, display_name: str) -> dict[str, Any]:
        """Register the repo's model-free inert ACP as a custom agent; a fresh fixture home has no profile."""
        agent = self.api("POST", "/agents/tui", {
            "display_name": display_name, "command": "python3", "command_args": [str(INERT_ACP_SCRIPT), "--identity-dir", str(self.root / "inert-identity")],
            "protocol": "acp", "description": "Model-free inert ACP for the Nomad recovery fixture",
        })
        agent = agent.get("agent", agent)
        profiles = agent.get("profiles") or []
        if len(profiles) != 1 or not profiles[0].get("id"):
            raise PreflightError("inert ACP agent creation did not return exactly one profile")
        return profiles[0]

    def start_git_daemon(self) -> dict[str, Any] | None:
        """Serve a bare copy of the fixture repository from a runner-owned loopback git daemon."""
        repo = self.state["repository"]
        if "path" not in repo:
            return None
        serve = _mkdir_private(self.root / "gitserve")
        self.ops.git_bare_clone(repo["path"], str(serve / f"{FIXTURE_REPO_NAME}.git"))
        self.save_state(expected_head=self.ops.git_head(repo["path"], repo["base_branch"]))
        port = self.ops.free_port()
        if port == self.state["port"] or self.ops.port_open(port):
            raise PreflightError("git daemon port is already in use")
        argv = [self.ops.git_binary(), "daemon", "--reuseaddr", "--listen=127.0.0.1", f"--port={port}", f"--base-path={serve}", "--export-all", str(serve)]
        record = self.registry.start(GIT_DAEMON_LABEL, argv, cwd=serve, env={"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": str(self.home)})
        self.poll(API_READY_TIMEOUT, 0.25, lambda: self.ops.port_open(port), "git daemon did not listen before timeout")
        self.save_state(git_port=port)
        return record

    def _redirect_git(self, workspace_id: str, agent_profile_id: str) -> None:
        """Give the local repository a provider-shaped origin that resolves to the runner's git daemon."""
        port = self.state.get("git_port")
        if port is None:
            return
        self.api("PUT", "/github/workspace-settings", {"workspace_id": workspace_id, "task_git_credentials_mode": "executor"})
        self.api("PATCH", f"/agent-profiles/{agent_profile_id}", {"env_vars": [
            {"key": "GIT_CONFIG_COUNT", "value": "1"},
            {"key": "GIT_CONFIG_KEY_0", "value": f"url.git://127.0.0.1:{port}/{FIXTURE_REPO_NAME}.git.insteadOf"},
            {"key": "GIT_CONFIG_VALUE_0", "value": f"https://github.com/{FIXTURE_REPO_OWNER}/{FIXTURE_REPO_NAME}.git"},
        ]})

    def _register_repository(self, cfg: dict[str, Any], workspace_id: str) -> dict[str, Any]:
        repo = cfg["repository"]
        if "path" in repo:
            body = {
                "name": Path(repo["path"]).name, "source_type": "local", "local_path": repo["path"], "default_branch": repo["base_branch"],
                "provider": "github", "provider_owner": FIXTURE_REPO_OWNER, "provider_name": FIXTURE_REPO_NAME,
            }
            return self.api("POST", f"/workspaces/{workspace_id}/repositories", body)
        return self.api("POST", f"/workspaces/{workspace_id}/repositories/remote", {"remote_url": repo["url"], "default_branch": repo["base_branch"]})

    @staticmethod
    def _first(items: Any, label: str) -> dict[str, Any]:
        if not isinstance(items, list) or not items or not isinstance(items[0], dict) or not items[0].get("id"):
            raise PreflightError(f"fresh fixture has no {label}")
        return items[0]

    # -- crash point -------------------------------------------------------------------
    def read_marker(self, marker: Path) -> dict[str, str] | None:
        try:
            text = marker.read_text(encoding="utf-8")
        except FileNotFoundError:
            return None
        lines = [line for line in text.splitlines() if line.strip()]
        if len(lines) != 1:
            return None
        try:
            value = json.loads(lines[0])
        except json.JSONDecodeError:
            return None
        return value if isinstance(value, dict) else None

    def await_barrier(self, marker: Path) -> dict[str, Any]:
        found = self.poll(self.ops.marker_timeout, 0.25, lambda: self.read_marker(marker), "pre-publish barrier marker did not appear before timeout")
        if found.keys() != MARKER_KEYS or any(not isinstance(found[key], str) or not found[key] for key in MARKER_KEYS):
            raise PreflightError("barrier marker does not contain exactly execution_id, environment_id, session_id")
        if found["session_id"] != self.state["session_id"]:
            raise PreflightError("barrier marker belongs to a different session")
        inventory = self.inventory()
        if inventory["execution_id"] != found["execution_id"] or inventory["operation_id"] != found["execution_id"] or inventory["environment_id"] != found["environment_id"]:
            raise PreflightError("durable inventory does not match the barrier marker identities")
        if inventory["phase"] != "ready" or inventory["workspace_input_captured"] is not True:
            raise PreflightError("durable inventory is not at phase ready with its workspace input captured")
        self.save_state(environment_id=found["environment_id"], execution_id=found["execution_id"], job_id=inventory["job_id"], allocation_id=inventory["allocation_id"], node_id=inventory["node_id"])
        return inventory

    def check_environment_creating(self, environment_id: str) -> int:
        return self.ops.read_db(self.database, lambda db: base.check_pre_ready_environment(db, environment_id))

    def check_session_created(self) -> list[dict[str, Any]]:
        row, messages = self.session_view()
        base.check_pre_ready_session(row, messages)
        return messages

    # -- teardown ------------------------------------------------------------------------
    def teardown(self, strict_retention: bool = True) -> dict[str, Any]:
        """Stop the session through the normal path, assert the WO04 retention contract, then stop runner-owned processes.

        An ordinary session stop retains the allocation, fence and workspace; ending the Nomad job is not part of this
        path, so the job is reported as an operator teardown and is never claimed as production cleanup.
        """
        report: dict[str, Any] = {"production_cleanup": False}
        if not any(item["label"] == PROCESS_LABEL for item in self.registry.read_records()):
            self.start_backend(None)
        record = next(item for item in self.registry.read_records() if item["label"] == PROCESS_LABEL)
        if self.state.get("session_id"):
            self.ops.ws({"mode": "session-stop", "ws_base": f"ws://127.0.0.1:{self.state['port']}", "session_id": self.state["session_id"]})
        if self.state.get("allocation_id"):
            report.update(self._observe_retention(strict_retention))
        if self.registry.stop(PROCESS_LABEL, timeout=15) not in ("stopped", "already_absent"):
            raise PreflightError("fixture backend did not stop through its exact owned process record")
        self.verify_gone(record["pid"])
        report["backend_pid_stopped"] = record["pid"]
        daemon = next((item for item in self.registry.read_records() if item["label"] == GIT_DAEMON_LABEL), None)
        if daemon is not None:
            self._stop_daemon(daemon)
            report["git_daemon_pid_stopped"] = daemon["pid"]
        self.revoke_scoped_token(report)
        (self.home / ".ssh" / "id_ed25519").unlink(missing_ok=True)
        report["ssh_identity_removed"] = True
        return report

    def _observe_retention(self, strict: bool) -> dict[str, Any]:
        self.ops.sleep(self.ops.retention_settle)
        allocation_id, job_id = self.state["allocation_id"], self.state["job_id"]
        status = self.nomad(f"/v1/allocation/{allocation_id}").get("ClientStatus")
        job = self.nomad(f"/v1/job/{job_id}")
        job = job.get("Job", job)
        fence = self.fence()
        inventory = {key: self.state[key] for key in ("allocation_id", "job_id", "environment_id")}
        retained = {
            "allocation_state_after_stop": status, "job_status_after_stop": job.get("Status"), "fence_retained": fence is not None,
            "fence_sha256_after_stop": fence, "remote_processes_retained": len(self.remote_tree(inventory)),
        }
        if strict:
            if status in TERMINAL_ALLOCATION_STATES:
                raise PreflightError("session stop unexpectedly ended the Nomad allocation; the retention contract is violated")
            if fence is None or fence != self.state.get("fence_sha256", fence):
                raise PreflightError("session stop released or changed the worker fence; the retention contract is violated")
        retained["operator_teardown_required"] = {
            "reason": "an ordinary session stop retains the allocation, fence, controller and workspace; ending the Nomad job is not part of this path",
            "job_id": job_id, "allocation_id": allocation_id, "environment_id": self.state["environment_id"], "fence_path": _fence_path(self.state),
        }
        return retained

    def stop_owned_processes(self) -> dict[str, str]:
        """Stop the runner's own backend and git daemon through their exact owned records after a failed run.

        The Nomad job, its agentctl and the node state stay for cleanup-fixture; only processes this runner started
        end here, so a failed run never leaves a headless Kandev behind.
        """
        return {label: stop_owned_process(self.registry, self.ops, label) for label in (PROCESS_LABEL, GIT_DAEMON_LABEL)}

    def _stop_daemon(self, daemon: dict[str, Any]) -> None:
        try:
            result = self.registry.stop(GIT_DAEMON_LABEL, timeout=15)
        except Exception as exc:
            # Only a verified leader identity makes the recorded group safe to signal: a forked
            # git-daemon child outliving its leader, or a leader that ignored SIGTERM.
            if not any(marker in str(exc) for marker in _GROUP_KILL_SAFE):
                raise PreflightError("git daemon ownership could not be verified; refusing to signal its process group") from exc
            self.ops.kill_group(daemon["pgid"])
            self.ops.sleep(0.5)
            result = self.registry.stop(GIT_DAEMON_LABEL, timeout=15)
        if result not in ("stopped", "already_absent"):
            raise PreflightError("git daemon did not stop through its exact owned process record")
        if self.ops.pid_alive(daemon["pid"]) or self.ops.port_open(self.state["git_port"]):
            raise PreflightError("git daemon process or port is still present")


def _identity_record(inventory: dict[str, Any], observed: dict[str, Any], environment_status: str) -> dict[str, Any]:
    nomad = observed["nomad"]
    return {
        "execution_id": inventory["execution_id"], "job_id": nomad["job_id"], "job_create_index": nomad["job_create_index"],
        "job_modify_index": nomad["job_definition_index"], "allocation_id": nomad["allocation_id"], "node_id": nomad["node_id"],
        "fence_owner_sha256": observed["fence"], "environment_status": environment_status, "inventory_phase": inventory["phase"],
        "remote_process_count": len(observed["tree"]),
    }


def _git_isolation_env(root: Path) -> dict[str, str]:
    return {"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": str(root / "gitconfig")}


def _new_state(cfg: dict[str, Any], root: Path, port: int, token_file: str) -> dict[str, Any]:
    home = root / "home"
    env = {
        "HOME": str(home), "KANDEV_HOME_DIR": str(home), "KANDEV_DATABASE_PATH": str(home / "data" / "kandev.db"),
        "KANDEV_SERVER_PORT": str(port), SERVER_HOST_ENV: LOOPBACK_HOST, **_git_isolation_env(root),
    }
    return {
        "schema_version": 1, "port": port, "kandev_binary": cfg["kandev_binary"], "bundle": cfg["bundle"], "manifest_env": env,
        "allocation_root": cfg["allocation_root"], "node_id": cfg["nomad"]["node_id"],
        "nomad": {"address": cfg["nomad"]["address"], "token_file": token_file, "namespace": cfg["nomad"]["namespace"], "region": cfg["nomad"]["region"], "node_id": cfg["nomad"]["node_id"]},
        "ssh": {"target": cfg["ssh"]["target"], "port": cfg["ssh"]["port"], "host_fingerprint": cfg["ssh"]["host_fingerprint"]},
        "expected_file": cfg["expected_file"], "expected_repo_count": 1, "repository": cfg["repository"],
    }


def _materialize(cfg: dict[str, Any], root: Path, port: int) -> None:
    home = _mkdir_private(root / "home")
    _mkdir_private(home / ".ssh")
    _mkdir_private(home / "data")
    _mkdir_private(home / "supervisor")
    identity = home / ".ssh" / "id_ed25519"
    fd = os.open(identity, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "wb") as handle:
        handle.write(Path(cfg["ssh"]["identity_file"]).read_bytes())
    env = {"HOME": str(home), "KANDEV_HOME_DIR": str(home), "KANDEV_DATABASE_PATH": str(home / "data" / "kandev.db"), "KANDEV_SERVER_PORT": str(port), SERVER_HOST_ENV: LOOPBACK_HOST, **_git_isolation_env(root)}
    if (cfg.get("pool") or {}).get("host_identity_token_file"):
        _write_private(home / "data" / "nomad-host-identity", Path(cfg["pool"]["host_identity_token_file"]).read_text(encoding="utf-8"))
    _write_private(root / "gitconfig", "")
    manifest = {"version": 1, "home_dir": str(home), "backend_executable": cfg["kandev_binary"], "argv": ["__backend"], "port": port, "env": env}
    _write_private(home / "supervisor" / "launch.json", json.dumps(manifest, sort_keys=True))
    base._verify_backend_launch_manifest(root, home, {"binary": cfg["kandev_binary"], "port": port})


def run_create_fixture(config: Any, ops: Ops | None = None, out: Callable[[str], Any] = print, err: Callable[[str], Any] | None = None,
                       run: Callable[..., Any] = subprocess.run, checkout_root: Path = bootstrap.CHECKOUT_ROOT,
                       drive: Callable[..., dict[str, Any]] | None = None, mode: str = "created-real-pre-ready-recovery",
                       label: str = "pre-ready recovery", check_config: Callable[[dict[str, Any]], Any] | None = None,
                       state_extra: Callable[[dict[str, Any]], dict[str, Any]] | None = None, stop_owned_on_failure: bool = False) -> int:
    """Shared create-fixture shell. The WO06 scenarios pass their own drive, label, config check and extra state."""
    ops = ops or Ops()
    err = err or (lambda text: print(text, file=sys.stderr))
    try:
        cfg = bootstrap.validate_bootstrap(config, run=run, checkout_root=checkout_root)
        if cfg.get("security"):
            configure_nomad_tls(cfg["nomad"]["address"], cfg["security"]["ca_file"])
        if check_config is not None:
            check_config(cfg)
        token = bootstrap.read_token(Path(cfg["nomad"]["token_file"]))
        port = ops.free_port()
        if port == bootstrap.LIVE_KANDEV_PORT:
            raise PreflightError("refusing to use the live Kandev port")
    except (PreflightError, OSError) as exc:
        err(f"real {label} bootstrap failed: {exc}")
        return 2
    root = Path(tempfile.mkdtemp(prefix="nomad-e2e-fixture-", dir=cfg["temp_root_parent"]))
    root.chmod(0o700)
    evidence = Evidence(root, [token])
    evidence.data.update({"mode": mode, "fixture_root": str(root), "port": port, "started": ops.wall()})
    runner: Runner | None = None
    try:
        _materialize(cfg, root, port)
        state = _new_state(cfg, root, port, cfg["nomad"]["token_file"])
        if cfg.get("security"):
            state["security"] = {"ca_file": cfg["security"]["ca_file"]}
        if state_extra is not None:
            state.update(state_extra(cfg))
        _save_state(root, state)
        evidence.save()
        runner = Runner(root, state, token, ops, evidence)
        report = (drive or _drive)(runner, cfg, ops, root)
    except Exception as exc:
        steps = evidence.data["steps"]
        failed = next((item["name"] for item in reversed(steps) if item["status"] == "failed"), steps[-1]["name"] if steps else "materialize")
        evidence.data.update({"status": "failed", "failed_step": failed, "error": evidence.scrub(str(exc)), "finished": ops.wall()})
        if runner is not None and stop_owned_on_failure:
            evidence.data["stopped_on_failure"] = runner.stop_owned_processes()
        evidence.save()
        err(evidence.scrub(f"real {label} failed: {exc}"))
        out(evidence.render())
        out(f"evidence: {root / EVIDENCE_FILE}")
        out(f"operator cleanup: {cleanup_command(root)}")
        return 2
    evidence.data.update({"status": "passed", "finished": ops.wall(), **report})
    evidence.save()
    out(f"evidence: {root / EVIDENCE_FILE}")
    out(evidence.render())
    return 0


def _drive(runner: Runner, cfg: dict[str, Any], ops: Ops, root: Path) -> dict[str, Any]:
    step, state, data = runner.step, runner.state, runner.evidence.data
    marker = root / "barrier-marker.json"
    step("start-git-daemon", runner.start_git_daemon)
    first = step("start-backend-with-barrier", lambda: runner.start_backend(str(marker)))
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    inventory = step("await-pre-publish-barrier", lambda: runner.await_barrier(marker))
    environment_id = inventory["environment_id"]

    def pre_crash() -> dict[str, Any]:
        generation = runner.check_environment_creating(environment_id)
        messages = runner.check_session_created()
        return {"generation": generation, "messages": messages, "nomad": runner.nomad_snapshot(inventory), "tree": runner.remote_tree(inventory), "fence": runner.fence()}

    before = step("snapshot-before-crash", pre_crash)
    runner.save_state(fence_sha256=before["fence"], environment_id=environment_id, job_id=inventory["job_id"], allocation_id=inventory["allocation_id"])
    data["identities"] = {"before": _identity_record(inventory, before, "creating")}
    if not before["tree"] or before["fence"] is None:
        raise PreflightError("remote runtime process tree or worker fence is missing before the crash")
    if before["generation"] != inventory["generation"]:
        raise PreflightError("environment ownership generation differs from the inventory generation")

    def crash() -> None:
        if runner.registry.crash(PROCESS_LABEL, timeout=10) != "crashed":
            raise PreflightError("fixture backend was not crashed through its exact owned process record")
        runner.verify_gone(first["pid"])

    step("crash-backend", crash)
    second = step("restart-backend-without-barrier", lambda: runner.start_backend(None))
    if second["pid"] == first["pid"]:
        raise PreflightError("restarted backend reused the crashed process identity")

    def recovered() -> dict[str, Any]:
        inventory_after = base._await_recovered_environment(
            lambda fn: ops.read_db(runner.database, fn), runner.inventory, environment_id=environment_id,
            generation_before=before["generation"], expected_repo_count=state["expected_repo_count"],
            timeout=ops.recovery_timeout, clock=ops.clock, sleep=ops.sleep,
        )
        row, messages = runner.session_view()
        base._check_session_untouched(row, messages, before["messages"])
        return {"inventory": inventory_after, "nomad": runner.nomad_snapshot(inventory_after), "tree": runner.remote_tree(inventory_after), "fence": runner.fence()}

    after = step("await-recovery", recovered)
    data["identities"]["after"] = _identity_record(after["inventory"], after, "ready")

    def identity() -> None:
        if after["inventory"] != inventory:
            raise PreflightError("recovery changed the execution, job, allocation, or server identity")
        if after["nomad"] != before["nomad"]:
            raise PreflightError("recovery changed the Nomad job or allocation identity")
        if after["fence"] != before["fence"]:
            raise PreflightError("recovery changed the worker fence")
        if after["tree"] != before["tree"]:
            raise PreflightError("recovery did not preserve the remote runtime process tree")

    step("assert-identity-preserved", identity)

    def workspace_read() -> None:
        read = ops.ws({"mode": "workspace-read", "ws_base": f"ws://127.0.0.1:{state['port']}", "session_id": state["session_id"], "task_id": state["task_id"], "expected_file": state["expected_file"]})
        data["workspace_read"] = {key: read.get(key) for key in ("ok", "contains_expected_file")}
        base._check_workspace_read(read)
        # Git snapshots exist only after an agent turn, so HEAD comes from the pinned read-only SSH probe.
        workspace = shlex.quote(after["inventory"]["workspace_path"])
        code, out = runner.ssh(f'for d in {workspace}/*/; do if [ -e "${{d}}.git" ]; then git -C "$d" rev-parse HEAD; fi; done')
        if code != 0:
            raise PreflightError("workspace repository HEAD read failed")
        head = base._parse_checkout_heads(out)
        data["workspace_read"]["repo_head"] = head
        base._check_workspace_head(head, state.get("expected_head"))
        runner.check_session_created()

    step("workspace-read-before-any-prompt", workspace_read)
    cleanup = step("session-stop-and-teardown", runner.teardown)
    return {
        "task_id": state["task_id"], "session_id": state["session_id"], "environment_id": environment_id, "execution_id": inventory["execution_id"],
        "job_id": inventory["job_id"], "allocation_id": inventory["allocation_id"], "node_id": inventory["node_id"],
        "server_identity": inventory["server_identity"], "backend_pids": {"before_crash": first["pid"], "after_restart": second["pid"]},
        "environment_status": {"before": "creating", "after": "ready"}, "inventory_phase": {"before": inventory["phase"], "after": after["inventory"]["phase"]},
        "workspace_path": after["inventory"]["workspace_path"], "fence_sha256": before["fence"], "prompts_sent": 0, "cleanup": cleanup,
    }


def create_fixture_command(config_path: str) -> int:
    try:
        config = json.loads(Path(config_path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        print("real pre-ready recovery bootstrap failed: config is unreadable", file=sys.stderr)
        return 2
    return run_create_fixture(config)


def load_fixture_state(root_text: str) -> tuple[Path, dict[str, Any]]:
    root = base._owned_private_root(root_text)
    path = root / STATE_FILE
    info = path.lstat() if path.exists() else None
    if info is None or not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise PreflightError("fixture state file is missing or not private")
    state = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(state, dict) or state.get("schema_version") != 1 or state.get("port") == bootstrap.LIVE_KANDEV_PORT:
        raise PreflightError("fixture state is invalid")
    return root, state


def run_cleanup_fixture(root_text: str, ops: Ops | None = None, out: Callable[[str], Any] = print, err: Callable[[str], Any] | None = None) -> int:
    ops = ops or Ops()
    err = err or (lambda text: print(text, file=sys.stderr))
    try:
        root, state = load_fixture_state(root_text)
        token = bootstrap.read_token(Path(state["nomad"]["token_file"]))
    except (PreflightError, OSError, json.JSONDecodeError, KeyError) as exc:
        err(f"cleanup-fixture failed before any action: {exc}")
        return 2
    evidence = Evidence(root, [token])
    try:
        evidence.data = json.loads((root / EVIDENCE_FILE).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        evidence.data = {"status": "unknown", "steps": []}
    try:
        if state.get("security"):
            configure_nomad_tls(state["nomad"]["address"], state["security"]["ca_file"])
        runner = Runner(root, state, token, ops, evidence)
        # The fixture's own processes end first, whatever state its job or session is in, so a stuck job
        # or a failing stop never leaves a headless Kandev or git daemon behind.
        runner.step("stop-owned-processes", runner.stop_owned_processes)
        if state.get("scenario"):
            import nomad_native_e2e_wo06 as wo06  # scenario fixtures tear down every recorded environment, not one session
            report = runner.step("cleanup-fixture", lambda: wo06.cleanup_fixture(runner))
        else:
            report = runner.step("cleanup-fixture", lambda: runner.teardown(strict_retention=False))
    except Exception as exc:
        # The cleanup path may have restarted the backend to stop the session; nothing owned outlives a failed attempt.
        with contextlib.suppress(Exception):
            runner.stop_owned_processes()
        evidence.data["cleanup"] = {"status": "failed", "error": evidence.scrub(str(exc)), "finished": ops.wall()}
        evidence.save()
        err(evidence.scrub(f"cleanup-fixture failed: {exc}"))
        out(f"evidence: {root / EVIDENCE_FILE}")
        out(f"operator cleanup: {cleanup_command(root)}")
        return 2
    evidence.data["cleanup"] = {"status": "passed", "finished": ops.wall(), **report}
    evidence.save()
    out(f"evidence: {root / EVIDENCE_FILE}")
    out(evidence.render())
    return 0


def cleanup_fixture_command(root_text: str) -> int:
    return run_cleanup_fixture(root_text)
