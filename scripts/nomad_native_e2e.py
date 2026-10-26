#!/usr/bin/env python3
"""Preflight and strict attached-fixture verifier for native Nomad E2E.

Preflight validates local fixture ownership and tooling only. It never starts
services. The session scenario attaches to an already running, normal-API
created fixture and verifies its live Kandev/Nomad/runtime boundaries without
creating a second allocation.
"""

from __future__ import annotations

import argparse
import hashlib
import ipaddress
import importlib.util
import json
import os
import re
import shlex
import socket
import sqlite3
import stat
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Callable
from urllib.parse import urlsplit


EXPECTED_NOMAD_VERSION = "2.0.7"
REQUIRED_SCENARIOS = {
    "session",
    "admission",
    "disconnect",
    "cleanup-positive",
    "cleanup-negative",
    "pool-placement",
    "pool-two-node",
    "pool-node-quarantine",
    "pool-queue",
    "pool-partition",
    "pool-refusals",
    "recovery",
    "pre-ready-recovery",
    "distribution",
}


class PreflightError(ValueError):
    pass


def _keys(value: Any, required: set[str], label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise PreflightError(f"{label} must be an object")
    missing = required - value.keys()
    extra = value.keys() - required
    if missing or extra:
        raise PreflightError(f"{label} has missing or unsupported fields")
    return value


def _loopback_host_port(value: Any, label: str) -> tuple[str, int]:
    if not isinstance(value, str) or value.count(":") != 1:
        raise PreflightError(f"{label} must be an IPv4 loopback host:port")
    host, port_text = value.rsplit(":", 1)
    try:
        address = ipaddress.ip_address(host)
        port = int(port_text)
    except ValueError as exc:
        raise PreflightError(f"{label} is invalid") from exc
    if not address.is_loopback or address.version != 4 or not 1 <= port <= 65535:
        raise PreflightError(f"{label} must use an IPv4 loopback address and valid port")
    return host, port


def _owned_private_root(path_text: Any) -> Path:
    if not isinstance(path_text, str) or not os.path.isabs(path_text):
        raise PreflightError("fixture_root must be an absolute path")
    path = Path(path_text)
    try:
        info = path.lstat()
    except OSError as exc:
        raise PreflightError("fixture_root must already exist") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
        raise PreflightError("fixture_root must be a real directory")
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise PreflightError("fixture_root must be owned by this user and private (0700 or stricter)")
    return path.resolve(strict=True)


def _private_fixture_file(root: Path, path_text: Any, label: str, private: bool) -> Path:
    if not isinstance(path_text, str) or not os.path.isabs(path_text):
        raise PreflightError(f"{label} must be an absolute fixture path")
    path = Path(path_text)
    try:
        info = path.lstat()
        resolved = path.resolve(strict=True)
    except OSError as exc:
        raise PreflightError(f"{label} must exist") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        raise PreflightError(f"{label} must be a regular non-symlink file")
    if root not in resolved.parents or info.st_uid != os.getuid():
        raise PreflightError(f"{label} must be owned by this user inside fixture_root")
    if private and info.st_mode & 0o077:
        raise PreflightError(f"{label} must have no group/world permissions")
    return resolved


def _executable(path_text: Any, label: str) -> Path:
    if not isinstance(path_text, str) or not os.path.isabs(path_text):
        raise PreflightError(f"{label} must be an absolute executable path")
    path = Path(path_text)
    if not path.is_file() or not os.access(path, os.X_OK):
        raise PreflightError(f"{label} is unavailable or not executable")
    return path


def _verify_runtime_bundle(binary: Path, run: Callable[..., Any]) -> dict[str, str]:
    """Fail before launch unless the normal <bundle>/bin runtime is complete."""
    resolved = binary.resolve(strict=True)
    if binary.is_symlink() or binary.name != "kandev" or binary.parent.name != "bin" or resolved != binary:
        raise PreflightError("backend binary must be the regular canonical <bundle>/bin/kandev")
    bundle = binary.parent.parent
    required = ("agentctl", "agentctl-linux-amd64", "agentctl-linux-arm64", "agentctl-darwin-arm64", "agentctl-darwin-amd64")
    for name in required:
        helper = bundle / "bin" / name
        try:
            info = helper.lstat()
        except OSError as exc:
            raise PreflightError(f"runtime bundle is missing {name}") from exc
        if not stat.S_ISREG(info.st_mode) or helper.is_symlink() or not os.access(helper, os.X_OK):
            raise PreflightError(f"runtime bundle helper {name} is not a regular executable")
    darwin_arm64 = bundle / "bin" / "agentctl-darwin-arm64"
    signature = run(["/usr/bin/codesign", "--verify", "--strict", str(darwin_arm64)], capture_output=True, text=True, check=False, timeout=10)
    if signature.returncode != 0:
        raise PreflightError("runtime bundle darwin/arm64 helper signature is invalid")
    return {"bundle": str(bundle), "binary": str(binary)}


def _fixture_backend_argv(binary: Path, startup_log: Path, port: int) -> list[str]:
    """Return the supervised fixture launch argv with one executable path."""
    binary = Path(binary)
    startup_log = Path(startup_log)
    if not binary.is_absolute() or not startup_log.is_absolute() or not 1 <= port <= 65535:
        raise PreflightError("fixture backend launch identity is invalid")
    wrapper = (
        "import os,sys;os.umask(0o077);"
        "fd=os.open(sys.argv[1],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600);"
        "os.dup2(fd,1);os.dup2(fd,2);os.execv(sys.argv[2],sys.argv[2:])"
    )
    return [
        sys.executable, "-c", wrapper, str(startup_log), str(binary),
        "run", "--headless", "--port", str(port),
    ]


def _owned_private_directory(root: Path, path_text: Any, label: str) -> Path:
    if not isinstance(path_text, str) or not os.path.isabs(path_text):
        raise PreflightError(f"{label} must be an absolute path")
    path = Path(path_text)
    try:
        info = path.lstat()
        resolved = path.resolve(strict=True)
    except OSError as exc:
        raise PreflightError(f"{label} must exist") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
        raise PreflightError(f"{label} must be a real directory")
    if root not in resolved.parents or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise PreflightError(f"{label} must be a private directory inside fixture_root")
    return resolved


def _verify_backend_launch_manifest(root: Path, home: Path, backend: Any) -> None:
    backend = _keys(backend, {"binary", "port"}, "backend")
    binary = _executable(backend["binary"], "backend.binary")
    port = backend["port"]
    if not isinstance(port, int) or isinstance(port, bool) or not 1 <= port <= 65535:
        raise PreflightError("backend.port must be a valid TCP port")
    manifest_path = _private_fixture_file(root, str(home / "supervisor" / "launch.json"), "Kandev launch manifest", private=True)
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise PreflightError("Kandev launch manifest is unreadable") from exc
    if not isinstance(manifest, dict):
        raise PreflightError("Kandev launch manifest must be an object")
    env = manifest.get("env")
    database_path = env.get("KANDEV_DATABASE_PATH") if isinstance(env, dict) else None
    if (
        manifest.get("home_dir") != str(home)
        or not isinstance(env, dict)
        or env.get("HOME") != str(home)
        or env.get("KANDEV_HOME_DIR") != str(home)
        or manifest.get("backend_executable") != str(binary)
        or manifest.get("argv") != ["__backend"]
        or manifest.get("port") != port
        or not isinstance(database_path, str)
        or home not in Path(database_path).resolve(strict=False).parents
    ):
        raise PreflightError("Kandev child launch manifest does not prove fixture HOME, database, binary, and API port")


def validate_config(config: Any, run: Callable[..., Any] = subprocess.run) -> dict[str, str]:
    allowed = {"schema_version", "fixture_only", "fixture_root", "kandev_home", "backend", "nomad", "ssh", "partition", "session", "recovery", "pre_ready_recovery"}
    if not isinstance(config, dict) or config.keys() - allowed:
        raise PreflightError("config has unsupported fields")
    required = {"schema_version", "fixture_only", "fixture_root", "kandev_home", "backend", "nomad", "ssh", "partition"}
    if required - config.keys():
        raise PreflightError("config is missing required fields")
    top = config
    if top["schema_version"] != 1 or top["fixture_only"] is not True:
        raise PreflightError("config must identify schema 1 and fixture_only=true")
    root = _owned_private_root(top["fixture_root"])
    kandev_home = _owned_private_directory(root, top["kandev_home"], "kandev_home")
    _verify_backend_launch_manifest(root, kandev_home, top["backend"])

    nomad = _keys(top["nomad"], {"binary", "version", "server_rpc", "client_rpc_proxy", "node_name"}, "nomad")
    binary = _executable(nomad["binary"], "nomad.binary")
    if nomad["version"] != EXPECTED_NOMAD_VERSION:
        raise PreflightError(f"nomad.version must be pinned to {EXPECTED_NOMAD_VERSION}")
    _loopback_host_port(nomad["server_rpc"], "nomad.server_rpc")
    _loopback_host_port(nomad["client_rpc_proxy"], "nomad.client_rpc_proxy")
    if not isinstance(nomad["node_name"], str) or not re.fullmatch(r"ff-fixture-[a-zA-Z0-9-]+", nomad["node_name"]):
        raise PreflightError("nomad.node_name must use the ff-fixture- prefix")

    ssh = _keys(top["ssh"], {"client", "server", "keygen", "host", "user", "port", "private_key", "known_hosts"}, "ssh")
    ssh_client = _executable(ssh["client"], "ssh.client")
    _executable(ssh["server"], "ssh.server")
    _executable(ssh["keygen"], "ssh.keygen")
    _loopback_host_port(f'{ssh["host"]}:{ssh["port"]}', "ssh.host/port")
    if not isinstance(ssh["user"], str) or not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_-]{0,31}", ssh["user"]):
        raise PreflightError("ssh.user is invalid")
    _private_fixture_file(root, ssh["private_key"], "ssh.private_key", private=True)
    _private_fixture_file(root, ssh["known_hosts"], "ssh.known_hosts", private=False)
    kandev_identity = _private_fixture_file(root, str(kandev_home / ".ssh" / "id_ed25519"), "kandev_home SSH identity", private=True)
    if hashlib.sha256(kandev_identity.read_bytes()).digest() != hashlib.sha256(Path(ssh["private_key"]).read_bytes()).digest():
        raise PreflightError("Kandev HOME SSH identity differs from the pinned fixture client identity")

    partition = _keys(top["partition"], {"mode", "listen", "can_drop_active_connections"}, "partition")
    if partition["mode"] != "loopback-tcp-relay" or partition["can_drop_active_connections"] is not True:
        raise PreflightError("partition must configure an owned relay that can drop active TCP connections")
    _loopback_host_port(partition["listen"], "partition.listen")

    # Validate the complete executable bundle before running local probes or
    # launching the normal Kandev entry point.
    _verify_runtime_bundle(Path(top["backend"]["binary"]), run)

    # These probes are local version/help invocations only; neither is passed
    # an address or config path, so they cannot contact a Nomad server.
    nomad_result = run([str(binary), "version"], capture_output=True, text=True, check=False, timeout=10)
    output = f"{nomad_result.stdout}\n{nomad_result.stderr}"
    match = re.search(r"\bNomad\s+v?(\d+\.\d+\.\d+)\b", output)
    if nomad_result.returncode != 0 or not match or match.group(1) != EXPECTED_NOMAD_VERSION:
        raise PreflightError("local Nomad binary version does not match the pinned fixture version")
    ssh_result = run([str(ssh_client), "-V"], capture_output=True, text=True, check=False, timeout=10)
    if ssh_result.returncode != 0 or "OpenSSH_" not in f"{ssh_result.stdout}\n{ssh_result.stderr}":
        raise PreflightError("local OpenSSH client version probe failed")
    return {"nomad_version": EXPECTED_NOMAD_VERSION, "ssh_client": "available", "fixture_root": "private"}


def _loopback_http(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value.startswith("http://"):
        raise PreflightError(f"{label} must be a loopback http URL")
    parsed = urlsplit(value)
    if parsed.hostname != "127.0.0.1" or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise PreflightError(f"{label} must use 127.0.0.1 without credentials")
    return value.rstrip("/")


def _uuid(value: Any, label: str) -> str:
    if not isinstance(value, str) or not re.fullmatch(r"[0-9a-fA-F-]{36}", value):
        raise PreflightError(f"{label} must be a UUID")
    return value


def _http_json(url: str, timeout: float = 5) -> Any:
    request = urllib.request.Request(url, headers={"Accept": "application/json"})
    with urllib.request.urlopen(request, timeout=timeout) as response:
        if response.status != 200:
            raise PreflightError("fixture API returned a non-success status")
        return json.loads(response.read())


def _ssh_command(config: dict[str, Any], command: str, timeout: float = 8) -> subprocess.CompletedProcess[str]:
    ssh = config["ssh"]
    return subprocess.run(
        [
            ssh["client"], "-F", "/dev/null", "-i", ssh["private_key"],
            "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes",
            "-o", "StrictHostKeyChecking=yes", "-o", f"UserKnownHostsFile={ssh['known_hosts']}",
            "-o", "ConnectTimeout=3", "-p", str(ssh["port"]),
            f"{ssh['user']}@{ssh['host']}", command,
        ], capture_output=True, text=True, timeout=timeout, check=False,
    )


def _run_attached_session(config: dict[str, Any]) -> dict[str, Any]:
    session = config.get("session")
    required_fields = {
        "api_base", "ws_base", "nomad_http", "task_id", "session_id", "environment_id",
        "node_id", "node_name", "job_id", "allocation_id", "workspace_path", "environment_generation", "repository_id", "repo_name", "repo_head",
        "repo_file", "repo_file_marker", "agentctl_pid", "acp_pid", "child_pid", "grandchild_pid",
        "nomad_client_pid", "kandev_backend_pid", "agentctl_binary_path", "agentctl_runtime_path", "agentctl_sha256",
        "kandev_binary_path", "kandev_sha256", "kandev_build_identity",
    }
    if not isinstance(session, dict) or session.keys() != required_fields:
        raise PreflightError("run requires the exact attached-fixture session contract")
    fixture_root = _owned_private_root(config["fixture_root"])
    kandev_home = _owned_private_directory(fixture_root, config["kandev_home"], "kandev_home")
    api_base = _loopback_http(session["api_base"], "session.api_base")
    nomad_http = _loopback_http(session["nomad_http"], "session.nomad_http")
    if urlsplit(api_base).path != "/api/v1" or urlsplit(nomad_http).path not in {"", "/"}:
        raise PreflightError("session API base paths are outside the fixture contract")
    ws_base = session["ws_base"]
    parsed_ws = urlsplit(ws_base) if isinstance(ws_base, str) else None
    if not parsed_ws or parsed_ws.scheme != "ws" or parsed_ws.hostname != "127.0.0.1" or not parsed_ws.port or parsed_ws.path not in {"", "/"} or parsed_ws.username or parsed_ws.password:
        raise PreflightError("session.ws_base must use loopback ws")
    for key in ("task_id", "session_id", "environment_id"):
        _uuid(session[key], f"session.{key}")
    for key in ("node_id",):
        _uuid(session[key], f"session.{key}")
    _uuid(session["repository_id"], "session.repository_id")
    if not isinstance(session["environment_generation"], int) or session["environment_generation"] < 1:
        raise PreflightError("session.environment_generation must be positive")
    for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid", "nomad_client_pid", "kandev_backend_pid"):
        if not isinstance(session[key], int) or session[key] <= 1:
            raise PreflightError(f"session.{key} must be a process ID")
    workspace = Path(session["workspace_path"])
    if not workspace.is_absolute() or fixture_root not in workspace.resolve(strict=True).parents:
        raise PreflightError("session.workspace_path must be an existing fixture-owned path")
    if not session["workspace_path"].endswith(f"/g{session['environment_generation']}"):
        raise PreflightError("workspace path does not encode the expected environment generation")
    if not re.fullmatch(r"[0-9a-f]{40}", session["repo_head"]):
        raise PreflightError("session.repo_head must be a Git commit ID")
    if not isinstance(session["repo_name"], str) or not session["repo_name"]:
        raise PreflightError("session.repo_name is required")
    if not isinstance(session["repo_file"], str) or session["repo_file"].startswith("/") or ".." in Path(session["repo_file"]).parts:
        raise PreflightError("session.repo_file must be a relative path")
    for key in ("agentctl_binary_path", "agentctl_runtime_path", "kandev_binary_path"):
        _private_fixture_file(fixture_root, session[key], f"session.{key}", private=False)
        if not os.access(session[key], os.X_OK):
            raise PreflightError(f"session.{key} must be executable")

    _verify_backend_launch_manifest(
        fixture_root,
        kandev_home,
        {"binary": session["kandev_binary_path"], "port": urlsplit(api_base).port},
    )

    task = _http_json(f"{api_base}/tasks/{session['task_id']}")
    sessions = _http_json(f"{api_base}/tasks/{session['task_id']}/sessions").get("sessions", [])
    matched = [item for item in sessions if item.get("id") == session["session_id"]]
    if len(matched) != 1:
        raise PreflightError("Kandev session is not uniquely attached to the fixture task")
    current = matched[0]
    if current.get("task_id") != session["task_id"] or current.get("workspace_path") != session["workspace_path"]:
        raise PreflightError("Kandev task/session/workspace ownership mismatch")
    if current.get("repository_id") != session["repository_id"]:
        raise PreflightError("Kandev session repository identity mismatch")
    if current.get("state") not in {"WAITING_FOR_INPUT", "REVIEW"}:
        raise PreflightError("Kandev session is not in an attachable state")
    messages_url = f"{api_base}/task-sessions/{session['session_id']}/messages"
    messages_before = _http_json(messages_url).get("messages", [])

    node = _http_json(f"{nomad_http}/v1/node/{session['node_id']}")
    if node.get("ID") != session["node_id"] or node.get("Name") != session["node_name"] or node.get("Status") != "ready" or node.get("SchedulingEligibility") != "eligible":
        raise PreflightError("Nomad worker is not the expected ready fixture node")
    job = _http_json(f"{nomad_http}/v1/job/{session['job_id']}")
    job_body = job.get("Job", job)
    if job_body.get("ID") != session["job_id"] or job_body.get("Status") not in {"running", "pending"}:
        raise PreflightError("Nomad job identity or state does not match fixture")
    allocations = _http_json(f"{nomad_http}/v1/job/{session['job_id']}/allocations")
    exact_allocs = [item for item in allocations if item.get("ID") == session["allocation_id"]]
    if len(exact_allocs) != 1 or exact_allocs[0].get("NodeID") != session["node_id"]:
        raise PreflightError("Nomad allocation identity/node does not match fixture")
    allocation = _http_json(f"{nomad_http}/v1/allocation/{session['allocation_id']}")
    if allocation.get("JobID") != session["job_id"] or allocation.get("NodeID") != session["node_id"] or allocation.get("ClientStatus") != "running":
        raise PreflightError("full Nomad allocation is not running under the expected job/node")
    if allocation.get("TaskStates", {}).get("agentctl", {}).get("State") != "running":
        raise PreflightError("Nomad agentctl task is not running")

    pids = [session[key] for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid")]
    ps = _ssh_command(config, "LC_ALL=C ps -o pid=,ppid=,pgid=,rss=,lstart= -p " + ",".join(str(pid) for pid in pids))
    local_pids = [session["nomad_client_pid"], session["kandev_backend_pid"]]
    local_ps = subprocess.run(
        ["env", "LC_ALL=C", "ps", "-o", "pid=,rss=,etime=,comm=", "-p", ",".join(str(pid) for pid in local_pids)],
        capture_output=True, text=True, timeout=5, check=False,
    )
    if ps.returncode != 0 or local_ps.returncode != 0:
        raise PreflightError("pinned fixture SSH process identity probe failed")
    local_starts = subprocess.run(
        ["env", "LC_ALL=C", "ps", "-o", "pid=,lstart=", "-p", ",".join(str(pid) for pid in local_pids)],
        capture_output=True, text=True, timeout=5, check=False,
    )
    if local_starts.returncode != 0:
        raise PreflightError("fixture process start-time probe failed")
    local_start_rows = {int(parts[0]): " ".join(parts[1:]) for line in local_starts.stdout.splitlines() if (parts := line.split()) and parts[0].isdigit()}
    if set(local_start_rows) != set(local_pids):
        raise PreflightError("fixture process start-time records are incomplete")
    rows = [line.split(None, 6) for line in ps.stdout.splitlines() if line.strip()]
    observed = {int(row[0]): row for row in rows if len(row) >= 6 and row[0].isdigit()}
    if set(observed) != set(pids):
        raise PreflightError("one or more recorded agentctl/ACP process identities are absent")
    leader, acp, child, grandchild = (observed[session[key]] for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid"))
    if (int(acp[1]) != session["agentctl_pid"] or int(acp[2]) != session["acp_pid"] or
        int(child[1]) != session["acp_pid"] or int(child[2]) != session["acp_pid"] or
        int(grandchild[1]) != session["child_pid"] or int(grandchild[2]) != session["acp_pid"]):
        raise PreflightError("ACP child process ancestry does not match fixture identities")
    local_rows = [line.split(None, 3) for line in local_ps.stdout.splitlines() if line.strip()]
    local_observed = {int(row[0]): row for row in local_rows if len(row) >= 4 and row[0].isdigit()}
    if set(local_observed) != set(local_pids):
        raise PreflightError("fixture Nomad client/backend process identities are absent")
    if local_observed[session["kandev_backend_pid"]][3] != session["kandev_binary_path"]:
        raise PreflightError("live backend executable does not match the fixture build")
    if not Path(session["kandev_binary_path"]).is_file() or not Path(session["agentctl_binary_path"]).is_file():
        raise PreflightError("fixture build artifacts are missing")
    kandev_sha = hashlib.sha256(Path(session["kandev_binary_path"]).read_bytes()).hexdigest()
    agentctl_sha = hashlib.sha256(Path(session["agentctl_binary_path"]).read_bytes()).hexdigest()
    if kandev_sha != session["kandev_sha256"] or agentctl_sha != session["agentctl_sha256"]:
        raise PreflightError("fixture build artifact digest does not match the recorded binary")
    remote_process_path = _ssh_command(config, f"LC_ALL=C ps -o comm= -p {session['agentctl_pid']}")
    remote_agentctl_sha = _ssh_command(config, f"shasum -a 256 {shlex.quote(session['agentctl_runtime_path'])}")
    if remote_process_path.returncode != 0 or remote_process_path.stdout.strip() != session["agentctl_runtime_path"]:
        raise PreflightError("live agentctl executable does not match the fixture build")
    if remote_agentctl_sha.returncode != 0 or remote_agentctl_sha.stdout.split()[0] != session["agentctl_sha256"]:
        raise PreflightError("live agentctl binary digest does not match the fixture build")
    repo_dir = workspace / session["repo_name"]
    git = _ssh_command(config, f"git -C {shlex.quote(str(repo_dir))} rev-parse HEAD && git -C {shlex.quote(str(repo_dir))} branch --show-current")
    if git.returncode != 0 or git.stdout.splitlines()[:2] != [session["repo_head"], "main"]:
        raise PreflightError("selected repository checkout does not match the expected branch and revision")

    # Fail closed on an untrusted host key and a closed/unknown SSH port.
    fixture_root = _owned_private_root(config["fixture_root"])
    bad_known_hosts = fixture_root / "runner-empty-known-hosts"
    bad_known_hosts.write_text("", encoding="utf-8")
    bad_known_hosts.chmod(0o600)
    ssh = config["ssh"]
    bad_hostkey = subprocess.run([
        ssh["client"], "-F", "/dev/null", "-i", ssh["private_key"],
        "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
        "-o", f"UserKnownHostsFile={bad_known_hosts}", "-o", "ConnectTimeout=3", "-p", str(ssh["port"]),
        f"{ssh['user']}@{ssh['host']}", "true",
    ], capture_output=True, text=True, timeout=8, check=False)
    if bad_hostkey.returncode == 0:
        raise PreflightError("SSH accepted an unpinned host key")
    unknown_port = int(ssh["port"]) + 1
    with socket.socket() as sock:
        sock.settimeout(0.5)
        if sock.connect_ex((ssh["host"], unknown_port)) == 0:
            raise PreflightError("unknown-port negative probe would reach a listener")
    bad_port = subprocess.run([
        ssh["client"], "-F", "/dev/null", "-i", ssh["private_key"],
        "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
        "-o", f"UserKnownHostsFile={ssh['known_hosts']}", "-o", "ConnectTimeout=2", "-p", str(unknown_port),
        f"{ssh['user']}@{ssh['host']}", "true",
    ], capture_output=True, text=True, timeout=6, check=False)
    if bad_port.returncode == 0:
        raise PreflightError("SSH connected on the fixture's unknown port")

    marker = f"WO04_RESULT_{int(time.time())}"
    ws_input = {
        "ws_base": ws_base,
        "task_id": session["task_id"], "session_id": session["session_id"],
        "environment_id": session["environment_id"],
        "repo_name": session["repo_name"], "repo_head": session["repo_head"],
        "repo_file": session["repo_file"], "repo_file_marker": session["repo_file_marker"],
        "result_marker": marker, "terminal_marker": f"WO04_TERMINAL_{int(time.time())}",
    }
    node_script = Path(__file__).with_name("nomad_native_e2e_ws.js")
    result = subprocess.run(["node", str(node_script)], input=json.dumps(ws_input), text=True, capture_output=True, timeout=45, check=False)
    if result.returncode != 0:
        raise PreflightError("Kandev WebSocket surface probe failed")
    try:
        ws_facts = json.loads(result.stdout.strip().splitlines()[-1])
    except (json.JSONDecodeError, IndexError) as exc:
        raise PreflightError("WebSocket surface probe returned no fact record") from exc
    if not all(ws_facts.get(key) for key in (
        "promptAccepted", "treeContainsRepo", "fileContainsExpectedText", "gitHeadBaseMatch",
        "unknownRuntimeTerminalRejected", "terminalMarkerObserved", "terminalDestroyed",
    )):
        raise PreflightError("one or more required WebSocket surfaces did not verify")

    deadline = time.monotonic() + 35
    assistant_result_count = 0
    after = []
    while time.monotonic() < deadline:
        after = _http_json(messages_url).get("messages", [])
        new = after[len(messages_before):]
        assistant_result_count = sum(marker in str(item.get("content", "")) for item in new)
        if assistant_result_count == 1:
            break
        time.sleep(0.5)
    if assistant_result_count != 1:
        raise PreflightError("inert ACP did not return exactly one requested result")

    report = {
        "mode": "attached-existing-fixture",
        "fresh_task_bootstrap": False,
        "task_state": task.get("state"),
        "session_state": current.get("state"),
        "workspace_owned": True,
        "allocation_running": True,
        "node_ready": True,
        "processes": {str(pid): {"rss_kb": int(row[3]), "start": " ".join(row[4:])} for pid, row in observed.items()},
        "new_assistant_result": assistant_result_count == 1,
        "assistant_result_count": assistant_result_count,
        "new_messages": len(after) - len(messages_before),
        "message_count_before": len(messages_before),
        "message_count_after": len(after),
        "websocket_surfaces": ws_facts,
        "ssh_wrong_host_key_rejected": True,
        "ssh_unknown_port_rejected": True,
        "repo_head": session["repo_head"],
        "node_name": node.get("Name"),
        "node_cpu_shares": node.get("NodeResources", {}).get("Cpu", {}).get("CpuShares"),
        "result_marker": marker,
        "kandev_build_identity": session["kandev_build_identity"],
        "kandev_binary_sha256": kandev_sha,
        "agentctl_binary_sha256": agentctl_sha,
        "local_processes": {str(pid): {"rss_kb": int(row[1]), "elapsed": row[2], "start": local_start_rows[pid]} for pid, row in local_observed.items()},
    }
    return report


def _recovery_completed_turn(messages: list[dict[str, Any]], turn: Any) -> dict[str, str]:
    turn = _keys(turn, {"user_message_id", "agent_delivery_id", "turn_id", "prompt_marker", "expected_output"}, "recovery.completed_turn")
    if not all(isinstance(turn[key], str) and turn[key] for key in turn):
        raise PreflightError("recovery completed-turn identity is incomplete")
    users = [item for item in messages if item.get("id") == turn["user_message_id"]]
    deliveries = [item for item in messages if item.get("id") == turn["agent_delivery_id"]]
    if len(users) != 1 or len(deliveries) != 1:
        raise PreflightError("completed turn must have one exact user message and one delivery")
    user, delivery = users[0], deliveries[0]
    if user.get("author_type") != "user" or user.get("type") != "message" or user.get("turn_id") != turn["turn_id"]:
        raise PreflightError("completed user message identity does not match the recorded turn")
    if turn["prompt_marker"] not in str(user.get("content", "")):
        raise PreflightError("completed user message does not contain its fixture marker")
    if delivery.get("author_type") != "agent" or delivery.get("type") != "message" or delivery.get("turn_id") != turn["turn_id"]:
        raise PreflightError("agent delivery is not correlated to the recorded user turn")
    if delivery.get("content") != turn["expected_output"]:
        raise PreflightError("completed turn result differs from its fixture expectation")
    same_turn_deliveries = [item for item in messages if item.get("turn_id") == turn["turn_id"] and item.get("type") == "message" and item.get("author_type") == "agent"]
    if same_turn_deliveries != [delivery]:
        raise PreflightError("completed turn has duplicate or unexpected agent deliveries")
    return {"user_message_id": turn["user_message_id"], "agent_delivery_id": turn["agent_delivery_id"], "turn_id": turn["turn_id"]}


def _nomad_recovery_snapshot(config: dict[str, Any], session: dict[str, Any], expected: dict[str, Any]) -> dict[str, Any]:
    base = session["nomad_http"].rstrip("/") + "/v1"
    node = _http_json(f"{base}/node/{session['node_id']}")
    job_response = _http_json(f"{base}/job/{session['job_id']}")
    job = job_response.get("Job", job_response)
    allocations = _http_json(f"{base}/job/{session['job_id']}/allocations")
    matches = [item for item in allocations if item.get("ID") == session["allocation_id"]]
    if len(matches) != 1 or len(allocations) != 1:
        raise PreflightError("recovery found a missing, duplicate, or successor allocation")
    allocation = _http_json(f"{base}/allocation/{session['allocation_id']}")
    if node.get("ID") != session["node_id"] or node.get("Name") != session["node_name"] or node.get("Status") != "ready" or node.get("SchedulingEligibility") != "eligible":
        raise PreflightError("recovery worker identity is not ready and eligible")
    if job.get("ID") != session["job_id"] or job.get("Status") != "running":
        raise PreflightError("recovery Nomad job identity or state changed")
    if job.get("CreateIndex") != expected["job_create_index"] or job.get("JobModifyIndex") != expected["job_definition_index"]:
        raise PreflightError("recovery Nomad job indexes differ from the original operation")
    if allocation.get("ID") != session["allocation_id"] or allocation.get("JobID") != session["job_id"] or allocation.get("NodeID") != session["node_id"] or allocation.get("ClientStatus") != "running":
        raise PreflightError("recovery allocation identity or state changed")
    if allocation.get("TaskStates", {}).get("agentctl", {}).get("State") != "running":
        raise PreflightError("recovery agentctl task is not running")
    return {"job_id": job["ID"], "job_create_index": job["CreateIndex"], "job_definition_index": job["JobModifyIndex"], "allocation_id": allocation["ID"], "node_id": allocation["NodeID"]}


def _fixture_database(home: Path) -> str:
    manifest = json.loads((home / "supervisor" / "launch.json").read_text(encoding="utf-8"))
    database = manifest["env"].get("KANDEV_DATABASE_PATH")
    if not isinstance(database, str) or home not in Path(database).resolve(strict=True).parents:
        raise PreflightError("recovery database path is outside fixture HOME")
    return database


def _nomad_inventory_snapshot(home: Path, session: dict[str, Any], expected: dict[str, Any], require_captured: bool = False) -> dict[str, Any]:
    database = _fixture_database(home)
    db = sqlite3.connect(f"file:{database}?mode=ro", uri=True, timeout=3)
    try:
        rows = db.execute("SELECT task_id, runtime, status, agent_execution_id, metadata FROM executors_running WHERE session_id = ?", (session["session_id"],)).fetchall()
    finally:
        db.close()
    if len(rows) != 1:
        raise PreflightError("recovery expected one durable executor inventory row")
    task_id, runtime, status, execution_id, raw_metadata = rows[0]
    try:
        metadata = json.loads(raw_metadata)
        state = metadata["nomad_environment_state"]
    except (TypeError, json.JSONDecodeError, KeyError) as exc:
        raise PreflightError("recovery Nomad inventory state is unreadable") from exc
    exact = {
        "environment_id": session["environment_id"], "generation": session["environment_generation"],
        "operation_id": expected["operation_id"], "execution_id": expected["operation_id"],
        "job_id": session["job_id"], "job_create_index": expected["job_create_index"],
        "job_definition_index": expected["job_definition_index"], "allocation_id": session["allocation_id"],
        "node_id": session["node_id"], "workspace_path": session["workspace_path"],
        "server_identity": expected["server_identity"], "phase": "ready",
    }
    if task_id != session["task_id"] or runtime != "nomad" or status != "ready" or execution_id != expected["operation_id"]:
        raise PreflightError("recovery durable executor row owner or status changed")
    if any(state.get(key) != value for key, value in exact.items()):
        raise PreflightError("recovery durable Nomad operation identity changed")
    if not metadata.get("env_secret_id_AGENTCTL_AUTH_TOKEN") or not metadata.get("env_secret_id_AGENTCTL_BOOTSTRAP_NONCE"):
        raise PreflightError("recovery credential references are incomplete")
    snapshot = {"runtime": runtime, "status": status, "execution_id": execution_id, **exact, "auth_token_ref_present": True, "bootstrap_nonce_ref_present": True}
    if require_captured:
        if state.get("workspace_input_captured") is not True:
            raise PreflightError("recovery durable Nomad state did not capture its workspace input")
        snapshot["workspace_input_captured"] = True
    return snapshot


def _message_identity_snapshot(messages: list[dict[str, Any]]) -> list[tuple[str, str, str]]:
    return sorted((item.get("id", ""), item.get("type", ""), item.get("turn_id", "")) for item in messages)


def _local_process(path: str, pid: int) -> dict[str, Any]:
    result = subprocess.run(["/bin/ps", "-p", str(pid), "-o", "pid=,rss=,etime=,comm="], capture_output=True, text=True, timeout=5, check=False)
    parts = result.stdout.split()
    if result.returncode != 0 or len(parts) < 4 or not parts[0].isdigit() or int(parts[0]) != pid or parts[3] != path:
        raise PreflightError("fixture backend process identity does not match the pinned binary")
    return {"pid": pid, "rss_kb": int(parts[1]), "elapsed": parts[2], "path": parts[3]}


def _listener_pid(port: int) -> int:
    result = subprocess.run(["/usr/sbin/lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN", "-t"], capture_output=True, text=True, timeout=5, check=False)
    values = [line for line in result.stdout.splitlines() if line.isdigit()]
    if result.returncode != 0 or len(values) != 1:
        raise PreflightError("fixture API port does not have exactly one listener")
    return int(values[0])


def _run_attached_recovery(config: dict[str, Any]) -> dict[str, Any]:
    validate_config(config)
    root = _owned_private_root(config["fixture_root"])
    home = _owned_private_directory(root, config["kandev_home"], "kandev_home")
    session = config.get("session")
    expected = config.get("recovery")
    if not isinstance(session, dict) or not isinstance(expected, dict):
        raise PreflightError("recovery verification requires exact session and fault evidence")
    required_session = {
        "api_base", "ws_base", "nomad_http", "task_id", "session_id", "environment_id", "node_id", "node_name",
        "job_id", "allocation_id", "workspace_path", "environment_generation", "agentctl_pid", "acp_pid",
        "child_pid", "grandchild_pid", "nomad_client_pid", "kandev_backend_pid", "kandev_binary_path",
    }
    if session.keys() != required_session:
        raise PreflightError("recovery session identity contract is incomplete")
    for key in ("task_id", "session_id", "environment_id", "node_id"):
        _uuid(session[key], f"recovery.session.{key}")
    if not isinstance(session["environment_generation"], int) or session["environment_generation"] < 1:
        raise PreflightError("recovery environment generation is invalid")
    for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid", "nomad_client_pid", "kandev_backend_pid"):
        if not isinstance(session[key], int) or session[key] <= 1:
            raise PreflightError(f"recovery session {key} is invalid")
    if not isinstance(session["workspace_path"], str) or not session["workspace_path"].endswith(f"/g{session['environment_generation']}"):
        raise PreflightError("recovery workspace path does not match its generation")
    required = {"process_label", "response_loss_marker", "operation_id", "server_identity", "job_create_index", "job_definition_index", "completed_turns", "new_prompt_marker", "expected_output"}
    if expected.keys() != required:
        raise PreflightError("recovery evidence contract is incomplete")
    marker_path = _private_fixture_file(root, expected["response_loss_marker"], "response-loss marker", private=True)
    marker = json.loads(marker_path.read_text(encoding="utf-8"))
    if marker.get("schema") != 1 or marker.get("committed_http_status") != 200 or not isinstance(marker.get("dropped_at_unix_ns"), int):
        raise PreflightError("fixture did not record a committed Nomad response loss")
    for item in expected["completed_turns"]:
        _recovery_completed_turn(_http_json(f"{session['api_base']}/task-sessions/{session['session_id']}/messages").get("messages", []), item)
    initial_messages = _http_json(f"{session['api_base']}/task-sessions/{session['session_id']}/messages").get("messages", [])
    prior_turns = [_recovery_completed_turn(initial_messages, item) for item in expected["completed_turns"]]
    inventory_before = _nomad_inventory_snapshot(home, session, expected)
    nomad_before = _nomad_recovery_snapshot(config, session, expected)
    task = _http_json(f"{session['api_base']}/tasks/{session['task_id']}")
    sessions = _http_json(f"{session['api_base']}/tasks/{session['task_id']}/sessions").get("sessions", [])
    current = [item for item in sessions if item.get("id") == session["session_id"]]
    if task.get("state") != "REVIEW" or len(current) != 1 or current[0].get("state") != "WAITING_FOR_INPUT" or current[0].get("workspace_path") != session["workspace_path"]:
        raise PreflightError("recovery task/session/workspace is not the expected ready fixture")

    spec = importlib.util.spec_from_file_location("owned_processes", Path(__file__).with_name("owned_processes.py"))
    if spec is None or spec.loader is None:
        raise PreflightError("fixture process registry is unavailable")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    registry = module.OwnedProcessRegistry(root)
    records = registry.read_records()
    record = next((item for item in records if item["label"] == expected["process_label"]), None)
    if record is None or record["pid"] != session["kandev_backend_pid"]:
        raise PreflightError("backend process label is not the exact fixture listener owner")
    manifest = json.loads((home / "supervisor" / "launch.json").read_text(encoding="utf-8"))
    backend_path = str(_executable(manifest.get("backend_executable"), "launch manifest backend executable"))
    if backend_path != session["kandev_binary_path"] or _local_process(backend_path, record["pid"])["pid"] != _listener_pid(manifest["port"]):
        raise PreflightError("fixture API listener is not the registered current Kandev child")
    local_before = _local_process(backend_path, record["pid"])
    agent_before = _ssh_command(config, "LC_ALL=C ps -o pid=,ppid=,pgid=,rss=,lstart= -p " + ",".join(str(session[key]) for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid")))
    if agent_before.returncode != 0 or len(agent_before.stdout.splitlines()) != 4:
        raise PreflightError("recovery agentctl process tree cannot be identity-checked")
    process_rows_before = {}
    for line in agent_before.stdout.splitlines():
        parts = line.split(None, 4)
        if len(parts) == 5 and parts[0].isdigit():
            process_rows_before[int(parts[0])] = {"ppid": int(parts[1]), "pgid": int(parts[2]), "started": parts[4]}
    if set(process_rows_before) != {session[key] for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid")}:
        raise PreflightError("recovery agentctl process identities are incomplete")
    agentctl_pid, acp_pid, child_pid, grandchild_pid = (session[key] for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid"))
    if (process_rows_before[acp_pid]["ppid"] != agentctl_pid or process_rows_before[acp_pid]["pgid"] != acp_pid or
        process_rows_before[child_pid]["ppid"] != acp_pid or process_rows_before[child_pid]["pgid"] != acp_pid or
        process_rows_before[grandchild_pid]["ppid"] != child_pid or process_rows_before[grandchild_pid]["pgid"] != acp_pid):
        raise PreflightError("recovery ACP child ancestry does not match the captured runtime tree")
    if _local_process(config["nomad"]["binary"], session["nomad_client_pid"])["pid"] != session["nomad_client_pid"]:
        raise PreflightError("fixture Nomad client is absent before recovery")

    stopped = registry.stop(expected["process_label"], timeout=10)
    if stopped != "stopped":
        raise PreflightError("fixture backend did not stop through its exact owned process record")
    with socket.socket() as probe:
        probe.settimeout(0.5)
        if probe.connect_ex(("127.0.0.1", manifest["port"])) == 0:
            raise PreflightError("fixture API port remained open after backend shutdown")
    restarted = registry.start(expected["process_label"], [backend_path, *manifest["argv"]], cwd=home, env=manifest["env"])
    deadline = time.monotonic() + 60
    ready = False
    while time.monotonic() < deadline:
        try:
            _http_json(f"{session['api_base']}/tasks/{session['task_id']}")
            ready = True
            break
        except (OSError, urllib.error.URLError, json.JSONDecodeError):
            time.sleep(0.25)
    if not ready:
        raise PreflightError("fixture backend did not recover its API before timeout")
    if restarted["pid"] == record["pid"] or _listener_pid(manifest["port"]) != restarted["pid"]:
        raise PreflightError("restarted API listener does not belong to a new owned backend process")

    time.sleep(2)
    messages_after_restart = _http_json(f"{session['api_base']}/task-sessions/{session['session_id']}/messages").get("messages", [])
    if _message_identity_snapshot(messages_after_restart) != _message_identity_snapshot(initial_messages):
        raise PreflightError("backend restart duplicated, removed, or altered durable message identities")
    replayed = [_recovery_completed_turn(messages_after_restart, item) for item in expected["completed_turns"]]
    inventory_after = _nomad_inventory_snapshot(home, session, expected)
    nomad_after = _nomad_recovery_snapshot(config, session, expected)
    if inventory_after != inventory_before or nomad_after != nomad_before or replayed != prior_turns:
        raise PreflightError("backend restart changed the original Nomad operation or completed turn evidence")
    agent_after = _ssh_command(config, "LC_ALL=C ps -o pid=,ppid=,pgid=,rss=,lstart= -p " + ",".join(str(session[key]) for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid")))
    process_rows_after = {}
    if agent_after.returncode == 0:
        for line in agent_after.stdout.splitlines():
            parts = line.split(None, 4)
            if len(parts) == 5 and parts[0].isdigit():
                process_rows_after[int(parts[0])] = {"ppid": int(parts[1]), "pgid": int(parts[2]), "started": parts[4]}
    if process_rows_after != process_rows_before:
        raise PreflightError("backend restart did not preserve the exact remote runtime process tree")
    client_after = _local_process(config["nomad"]["binary"], session["nomad_client_pid"])
    if client_after["pid"] != session["nomad_client_pid"]:
        raise PreflightError("Nomad client identity changed during Kandev backend recovery")

    prompt_marker = expected["new_prompt_marker"]
    if not isinstance(prompt_marker, str) or not re.fullmatch(r"WO05_RECOVERY_[A-Za-z0-9_-]{8,80}", prompt_marker):
        raise PreflightError("recovery prompt marker is invalid")
    if any(prompt_marker in str(item.get("content", "")) for item in messages_after_restart):
        raise PreflightError("recovery prompt marker was already used")
    prompt_input = {"ws_base": session["ws_base"], "task_id": session["task_id"], "session_id": session["session_id"], "content": f"For this inert transport check, output exactly {prompt_marker}"}
    prompt_script = Path(__file__).with_name("nomad_native_e2e_recovery.js")
    result = subprocess.run(["node", str(prompt_script)], input=json.dumps(prompt_input), text=True, capture_output=True, timeout=25, check=False)
    if result.returncode != 0:
        raise PreflightError("post-recovery fixture prompt was not accepted")
    try:
        ack = json.loads(result.stdout.strip().splitlines()[-1])
    except (json.JSONDecodeError, IndexError) as exc:
        raise PreflightError("post-recovery prompt acknowledgment is unreadable") from exc
    if ack.get("accepted") is not True or not isinstance(ack.get("message_id"), str):
        raise PreflightError("post-recovery prompt was not durably accepted")
    deadline = time.monotonic() + 35
    final_messages = []
    new_delivery = None
    while time.monotonic() < deadline:
        final_messages = _http_json(f"{session['api_base']}/task-sessions/{session['session_id']}/messages").get("messages", [])
        new_users = [item for item in final_messages if item.get("id") == ack["message_id"]]
        if len(new_users) == 1:
            new_turn = new_users[0].get("turn_id")
            deliveries = [item for item in final_messages if item.get("author_type") == "agent" and item.get("type") == "message" and item.get("turn_id") == new_turn]
            if deliveries:
                if len(deliveries) != 1 or deliveries[0].get("content") != expected["expected_output"]:
                    raise PreflightError("post-recovery prompt did not produce exactly one expected inert result")
                new_delivery = deliveries[0]
                break
        time.sleep(0.25)
    if new_delivery is None:
        raise PreflightError("recovered agentctl did not return a result for the post-restart prompt")
    if len([item for item in final_messages if item.get("id") == ack["message_id"]]) != 1:
        raise PreflightError("post-recovery prompt was duplicated")
    for turn in expected["completed_turns"]:
        _recovery_completed_turn(final_messages, turn)
    if _nomad_inventory_snapshot(home, session, expected) != inventory_before or _nomad_recovery_snapshot(config, session, expected) != nomad_before:
        raise PreflightError("post-recovery prompt changed the retained Nomad allocation identity")
    return {
        "mode": "attached-real-recovery",
        "response_loss_committed_status": marker["committed_http_status"],
        "backend_old_pid": record["pid"], "backend_new_pid": restarted["pid"],
        "backend_listener_pid": _listener_pid(manifest["port"]),
        "prior_completed_turns_preserved_once": len(replayed),
        "messages_before_restart": len(initial_messages), "messages_after_restart": len(messages_after_restart),
        "new_prompt_message_id": ack["message_id"], "new_delivery_id": new_delivery["id"],
        "new_delivery_content": expected["expected_output"],
        "nomad_job_id": nomad_after["job_id"], "allocation_id": nomad_after["allocation_id"],
        "node_id": nomad_after["node_id"], "workspace_path": inventory_after["workspace_path"],
        "server_identity": inventory_after["server_identity"],
    }


PRE_READY_KEYS = {"process_label", "operation_id", "server_identity", "job_create_index", "job_definition_index", "expected_repo_count", "expected_file"}
_ACP_SESSION_FIELDS = ("acp_session_id", "agent_session_id", "downstream_acp_session_id")


def _pre_ready_contract(config: dict[str, Any]) -> tuple[dict[str, Any], dict[str, Any]]:
    session = config.get("session")
    expected = config.get("pre_ready_recovery")
    if not isinstance(session, dict) or not isinstance(expected, dict):
        raise PreflightError("pre-ready recovery requires exact session and pre_ready_recovery evidence")
    required_session = {
        "api_base", "ws_base", "nomad_http", "task_id", "session_id", "environment_id", "node_id", "node_name",
        "job_id", "allocation_id", "workspace_path", "environment_generation", "agentctl_pid", "acp_pid",
        "child_pid", "grandchild_pid", "nomad_client_pid", "kandev_backend_pid", "kandev_binary_path",
    }
    if session.keys() != required_session:
        raise PreflightError("pre-ready recovery session identity contract is incomplete")
    if expected.keys() != PRE_READY_KEYS:
        raise PreflightError("pre-ready recovery evidence contract is incomplete")
    for key in ("task_id", "session_id", "environment_id", "node_id"):
        _uuid(session[key], f"pre_ready_recovery.session.{key}")
    if not isinstance(session["environment_generation"], int) or session["environment_generation"] < 1:
        raise PreflightError("pre-ready recovery environment generation is invalid")
    for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid", "nomad_client_pid", "kandev_backend_pid"):
        if not isinstance(session[key], int) or session[key] <= 1:
            raise PreflightError(f"pre-ready recovery session {key} is invalid")
    if not isinstance(session["workspace_path"], str) or not session["workspace_path"].endswith(f"/g{session['environment_generation']}"):
        raise PreflightError("pre-ready recovery workspace path does not match its generation")
    count = expected["expected_repo_count"]
    if not isinstance(count, int) or isinstance(count, bool) or count < 0:
        raise PreflightError("pre-ready recovery expected_repo_count is invalid")
    if not isinstance(expected["expected_file"], str) or not expected["expected_file"]:
        raise PreflightError("pre-ready recovery expected_file is invalid")
    return session, expected


def _environment_rows(db: sqlite3.Connection, environment_id: str) -> tuple[tuple[Any, ...], int]:
    rows = db.execute("SELECT status, workspace_path, ownership_generation FROM task_environments WHERE id = ?", (environment_id,)).fetchall()
    if len(rows) != 1:
        raise PreflightError("pre-ready recovery expected exactly one task environment row")
    count = db.execute("SELECT COUNT(*) FROM task_environment_repos WHERE task_environment_id = ?", (environment_id,)).fetchone()[0]
    return rows[0], count


def check_pre_ready_environment(db: sqlite3.Connection, environment_id: str) -> int:
    (status, workspace_path, generation), _ = _environment_rows(db, environment_id)
    if status != "creating" or workspace_path:
        raise PreflightError("pre-ready recovery environment is not in the creating state with an empty workspace path")
    return generation


def check_post_ready_environment(db: sqlite3.Connection, environment_id: str, inventory_workspace_path: str, generation_before: int, expected_repo_count: int) -> str:
    """Returns "pending" until the environment is ready; raises when the ready row is wrong."""
    (status, workspace_path, generation), repo_count = _environment_rows(db, environment_id)
    if status != "ready":
        return "pending"
    if workspace_path != inventory_workspace_path:
        raise PreflightError("recovered environment workspace path differs from the retained inventory workspace path")
    if generation != generation_before:
        raise PreflightError("recovered environment ownership generation changed")
    if repo_count != expected_repo_count:
        raise PreflightError("recovered environment repository row count differs from the expected count")
    return "ready"


def check_pre_ready_session(session_row: dict[str, Any], messages: list[dict[str, Any]]) -> None:
    if session_row.get("state") != "CREATED":
        raise PreflightError("pre-ready recovery session is not in the CREATED state")
    if any(session_row.get(key) for key in _ACP_SESSION_FIELDS):
        raise PreflightError("pre-ready recovery session already has an ACP session")
    if any(item.get("author_type") == "user" for item in messages):
        raise PreflightError("pre-ready recovery session already holds a user message")


def _pre_ready_session_row(session: dict[str, Any]) -> dict[str, Any]:
    sessions = _http_json(f"{session['api_base']}/tasks/{session['task_id']}/sessions").get("sessions", [])
    current = [item for item in sessions if item.get("id") == session["session_id"]]
    if len(current) != 1:
        raise PreflightError("pre-ready recovery session is not uniquely listed")
    return current[0]


def _pre_ready_messages(session: dict[str, Any]) -> list[dict[str, Any]]:
    return _http_json(f"{session['api_base']}/task-sessions/{session['session_id']}/messages").get("messages", [])


def _remote_process_rows(config: dict[str, Any], session: dict[str, Any]) -> dict[int, dict[str, Any]]:
    result = _ssh_command(config, "LC_ALL=C ps -o pid=,ppid=,pgid=,rss=,lstart= -p " + ",".join(str(session[key]) for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid")))
    rows: dict[int, dict[str, Any]] = {}
    if result.returncode == 0:
        for line in result.stdout.splitlines():
            parts = line.split(None, 4)
            if len(parts) == 5 and parts[0].isdigit():
                rows[int(parts[0])] = {"ppid": int(parts[1]), "pgid": int(parts[2]), "started": parts[4]}
    return rows


def _workspace_read(session: dict[str, Any], expected_file: str) -> dict[str, Any]:
    script = Path(__file__).with_name("nomad_native_e2e_ws.js")
    payload = {"mode": "workspace-read", "ws_base": session["ws_base"], "session_id": session["session_id"], "task_id": session["task_id"], "expected_file": expected_file}
    result = subprocess.run(["node", str(script)], input=json.dumps(payload), text=True, capture_output=True, timeout=30, check=False)
    if result.returncode != 0:
        raise PreflightError("normal workspace read after pre-ready recovery failed")
    try:
        return json.loads(result.stdout.strip().splitlines()[-1])
    except (json.JSONDecodeError, IndexError) as exc:
        raise PreflightError("workspace read result is unreadable") from exc


def _await_recovered_environment(
    read_db: Callable[[Callable[[sqlite3.Connection], Any]], Any],
    read_inventory: Callable[[], dict[str, Any]],
    *, environment_id: str, generation_before: int, expected_repo_count: int,
    timeout: float = 120, clock: Callable[[], float] = time.monotonic, sleep: Callable[[float], Any] = time.sleep,
) -> dict[str, Any]:
    """Poll until the recovered environment is ready; returns the final inventory snapshot."""
    deadline = clock() + timeout
    while True:
        inventory = read_inventory()
        state = read_db(lambda db: check_post_ready_environment(db, environment_id, inventory["workspace_path"], generation_before, expected_repo_count))
        if state == "ready":
            return inventory
        if clock() >= deadline:
            raise PreflightError("recovered environment did not become ready before timeout")
        sleep(0.5)


def _check_session_untouched(session_row: dict[str, Any], messages_after: list[dict[str, Any]], messages_before: list[dict[str, Any]]) -> None:
    check_pre_ready_session(session_row, messages_after)
    if _message_identity_snapshot(messages_after) != _message_identity_snapshot(messages_before):
        raise PreflightError("pre-ready recovery changed the durable message identities")


def _check_workspace_read(read: dict[str, Any]) -> None:
    if read.get("ok") is not True or read.get("contains_expected_file") is not True:
        raise PreflightError("normal workspace read did not return the expected file")


def _parse_checkout_heads(output: str) -> str:
    """Return the single checkout HEAD printed by the read-only workspace probe."""
    heads = [line.strip() for line in output.splitlines() if line.strip()]
    if len(heads) != 1 or not re.fullmatch(r"[0-9a-f]{40}", heads[0]):
        raise PreflightError("workspace did not report exactly one repository checkout HEAD")
    return heads[0]


def _check_workspace_head(head: str, expected_head: str | None) -> None:
    # A URL repository has no local bare repo to compare against; the probe already proved a real commit ID.
    if expected_head is not None and head != expected_head:
        raise PreflightError("workspace repository HEAD does not match the fixture repository commit")


def _run_attached_pre_ready_recovery(config: dict[str, Any]) -> dict[str, Any]:
    validate_config(config)
    root = _owned_private_root(config["fixture_root"])
    home = _owned_private_directory(root, config["kandev_home"], "kandev_home")
    session, expected = _pre_ready_contract(config)
    database = _fixture_database(home)

    def read_db(fn: Callable[[sqlite3.Connection], Any]) -> Any:
        db = sqlite3.connect(f"file:{database}?mode=ro", uri=True, timeout=3)
        try:
            return fn(db)
        finally:
            db.close()

    messages_before = _pre_ready_messages(session)
    check_pre_ready_session(_pre_ready_session_row(session), messages_before)
    generation_before = read_db(lambda db: check_pre_ready_environment(db, session["environment_id"]))
    inventory_before = _nomad_inventory_snapshot(home, session, expected, require_captured=True)
    nomad_before = _nomad_recovery_snapshot(config, session, expected)

    spec = importlib.util.spec_from_file_location("owned_processes", Path(__file__).with_name("owned_processes.py"))
    if spec is None or spec.loader is None:
        raise PreflightError("fixture process registry is unavailable")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    registry = module.OwnedProcessRegistry(root)
    record = next((item for item in registry.read_records() if item["label"] == expected["process_label"]), None)
    if record is None or record["pid"] != session["kandev_backend_pid"]:
        raise PreflightError("backend process label is not the exact fixture listener owner")
    manifest = json.loads((home / "supervisor" / "launch.json").read_text(encoding="utf-8"))
    backend_path = str(_executable(manifest.get("backend_executable"), "launch manifest backend executable"))
    if backend_path != session["kandev_binary_path"] or _local_process(backend_path, record["pid"])["pid"] != _listener_pid(manifest["port"]):
        raise PreflightError("fixture API listener is not the registered current Kandev child")
    process_rows_before = _remote_process_rows(config, session)
    if set(process_rows_before) != {session[key] for key in ("agentctl_pid", "acp_pid", "child_pid", "grandchild_pid")}:
        raise PreflightError("recovery agentctl process identities are incomplete")
    if _local_process(config["nomad"]["binary"], session["nomad_client_pid"])["pid"] != session["nomad_client_pid"]:
        raise PreflightError("fixture Nomad client is absent before recovery")

    if registry.stop(expected["process_label"], timeout=10) != "stopped":
        raise PreflightError("fixture backend did not stop through its exact owned process record")
    with socket.socket() as probe:
        probe.settimeout(0.5)
        if probe.connect_ex(("127.0.0.1", manifest["port"])) == 0:
            raise PreflightError("fixture API port remained open after backend shutdown")
    restarted = registry.start(expected["process_label"], [backend_path, *manifest["argv"]], cwd=home, env=manifest["env"])
    deadline = time.monotonic() + 60
    ready = False
    while time.monotonic() < deadline:
        try:
            _http_json(f"{session['api_base']}/tasks/{session['task_id']}")
            ready = True
            break
        except (OSError, urllib.error.URLError, json.JSONDecodeError):
            time.sleep(0.25)
    if not ready:
        raise PreflightError("fixture backend did not recover its API before timeout")
    if restarted["pid"] == record["pid"] or _listener_pid(manifest["port"]) != restarted["pid"]:
        raise PreflightError("restarted API listener does not belong to a new owned backend process")

    inventory_after = _await_recovered_environment(
        read_db, lambda: _nomad_inventory_snapshot(home, session, expected, require_captured=True),
        environment_id=session["environment_id"], generation_before=generation_before,
        expected_repo_count=expected["expected_repo_count"],
    )

    _check_session_untouched(_pre_ready_session_row(session), _pre_ready_messages(session), messages_before)
    nomad_after = _nomad_recovery_snapshot(config, session, expected)
    if inventory_after != inventory_before or nomad_after != nomad_before:
        raise PreflightError("pre-ready recovery changed the retained Nomad operation, allocation, or fence")
    if _remote_process_rows(config, session) != process_rows_before:
        raise PreflightError("pre-ready recovery did not preserve the exact remote runtime process tree")
    if _local_process(config["nomad"]["binary"], session["nomad_client_pid"])["pid"] != session["nomad_client_pid"]:
        raise PreflightError("Nomad client identity changed during Kandev backend recovery")

    read = _workspace_read(session, expected["expected_file"])
    _check_workspace_read(read)
    check_pre_ready_session(_pre_ready_session_row(session), _pre_ready_messages(session))
    report = {
        "mode": "attached-real-pre-ready-recovery",
        "backend_old_pid": record["pid"], "backend_new_pid": restarted["pid"],
        "environment_status_before": "creating", "environment_status_after": "ready",
        "job_id": nomad_after["job_id"], "allocation_id": nomad_after["allocation_id"],
        "node_id": nomad_after["node_id"], "workspace_path": inventory_after["workspace_path"],
        "server_identity": inventory_after["server_identity"],
        "repo_count": expected["expected_repo_count"], "workspace_read_ok": True, "prompts_sent": 0,
    }
    return report


def _fixture_module() -> Any:
    # The fixture module shares this module's PreflightError and helpers, also when run as __main__.
    sys.modules.setdefault("nomad_native_e2e", sys.modules[__name__])
    import nomad_native_e2e_fixture
    return nomad_native_e2e_fixture


def _wo06_module() -> Any:
    _fixture_module()
    import nomad_native_e2e_wo06
    return nomad_native_e2e_wo06


def _wo07_module() -> Any:
    _fixture_module()
    import nomad_native_e2e_wo07
    return nomad_native_e2e_wo07


def _pool_module() -> Any:
    _fixture_module()
    import nomad_native_e2e_pool
    return nomad_native_e2e_pool


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    preflight = commands.add_parser("preflight", help="validate fixture config and local tools only")
    preflight.add_argument("--config", required=True)
    run_parser = commands.add_parser("run", help="verify the attached, already-running real fixture")
    run_parser.add_argument("--scenario", choices=sorted(REQUIRED_SCENARIOS), required=True)
    run_parser.add_argument("--require-real", action="store_true")
    run_parser.add_argument("--config", required=True)
    run_parser.add_argument("--create-fixture", action="store_true", help="pre-ready-recovery, admission, disconnect, cleanup-positive and cleanup-negative: create a fresh fixture from a bootstrap config, crash, recover and clean up")
    cleanup = commands.add_parser("cleanup-fixture", help="stop the session of an existing created fixture through the production path and verify teardown")
    cleanup.add_argument("--fixture-root", required=True)
    args = parser.parse_args(argv)
    if args.command == "cleanup-fixture":
        return _fixture_module().cleanup_fixture_command(args.fixture_root)
    if args.command == "run" and args.create_fixture:
        if args.scenario not in ("pre-ready-recovery", "admission", "disconnect", "cleanup-positive", "cleanup-negative", "pool-placement", "pool-two-node", "pool-node-quarantine", "pool-queue", "pool-partition", "pool-refusals") or not args.require_real:
            print("--create-fixture requires --require-real --scenario pre-ready-recovery|admission|disconnect|cleanup-positive|cleanup-negative|pool-placement|pool-two-node|pool-node-quarantine|pool-queue|pool-partition|pool-refusals", file=sys.stderr)
            return 2
        if args.scenario == "pool-partition":
            _fixture_module()
            import nomad_native_e2e_pool_partition
            return nomad_native_e2e_pool_partition.create_fixture_command(args.config)
        if args.scenario in ("pool-placement", "pool-two-node", "pool-node-quarantine", "pool-queue", "pool-refusals"):
            return _pool_module().create_fixture_command(args.config, args.scenario)
        if args.scenario in ("cleanup-positive", "cleanup-negative"):
            return _wo07_module().create_fixture_command(args.config, args.scenario)
        if args.scenario in ("admission", "disconnect"):
            return _wo06_module().create_fixture_command(args.config, args.scenario)
        return _fixture_module().create_fixture_command(args.config)
    if args.command == "run":
        if not args.require_real:
            print("run requires --require-real --scenario session", file=sys.stderr)
            return 2
        if args.scenario not in ("session", "recovery", "pre-ready-recovery"):
            print("run requires --require-real --scenario session; the selected scenario is not implemented", file=sys.stderr)
            return 2
        try:
            config = json.loads(Path(args.config).read_text(encoding="utf-8"))
            if args.scenario == "session":
                validate_config(config)
                report = _run_attached_session(config)
            elif args.scenario == "recovery":
                report = _run_attached_recovery(config)
            elif args.scenario == "pre-ready-recovery":
                report = _run_attached_pre_ready_recovery(config)
            else:
                raise PreflightError("this real fixture runner does not implement the requested scenario")
        except (OSError, json.JSONDecodeError, PreflightError, subprocess.TimeoutExpired) as exc:
            label = args.scenario if args.scenario != "pre-ready-recovery" else "pre-ready recovery"
            print(f"real {label} verification failed: {exc}", file=sys.stderr)
            return 2
        print(json.dumps(report, sort_keys=True))
        return 0
    try:
        config = json.loads(Path(args.config).read_text(encoding="utf-8"))
        result = validate_config(config)
    except (OSError, json.JSONDecodeError, PreflightError) as exc:
        print(f"preflight failed: {exc}", file=sys.stderr)
        return 1
    print(f"preflight passed: Nomad {result['nomad_version']}; OpenSSH client available; fixture root private")
    print("preflight did not start services or contact a Nomad server")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
