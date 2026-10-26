#!/usr/bin/env python3
"""Fail-closed preflight for the actual isolated Kandev child SSH identity.

This is fixture tooling only. It reads the selected child HOME from ``ps`` in
memory, validates the exact ``~/.ssh/id_ed25519`` file and key fingerprint,
then performs one strict known-hosts-pinned SSH command. It never prints
process environments, private key material, or raw SSH diagnostics.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import subprocess
import sys
from pathlib import Path
from typing import Callable

sys.path.insert(0, str(Path(__file__).resolve().parent))
from owned_processes import OwnedProcessRegistry, ProcessOwnershipError


class PreflightError(ValueError):
    pass


def parse_process_env_value(command_line: str, key: str) -> str:
    values = re.findall(r"(?:^|\s)" + re.escape(key) + r"=([^\s]+)", command_line)
    if len(values) != 1:
        raise PreflightError(f"process environment does not contain one {key}")
    return values[0]


def identity_path_for_home(home: str) -> Path:
    if not home or not os.path.isabs(home):
        raise PreflightError("backend HOME is missing or not absolute")
    return Path(home) / ".ssh" / "id_ed25519"


def private_regular_file(path: Path, *, root: Path) -> None:
    try:
        info = path.lstat()
    except OSError as exc:
        raise PreflightError("fixture identity file is unavailable") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        raise PreflightError("fixture identity must be a regular non-symlink file")
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise PreflightError("fixture identity must be user-owned and private")
    resolved = path.resolve(strict=True)
    if root.resolve(strict=True) not in resolved.parents:
        raise PreflightError("fixture identity is outside the owned fixture root")


def key_fingerprint(path: Path, *, runner: Callable[..., subprocess.CompletedProcess[str]] = subprocess.run) -> str:
    try:
        result = runner(
            ["/usr/bin/ssh-keygen", "-lf", str(path), "-E", "sha256"],
            capture_output=True,
            text=True,
            check=False,
            env={"PATH": "/usr/bin:/bin"},
            timeout=5,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise PreflightError("fixture key fingerprint could not be inspected") from exc
    if result.returncode != 0:
        raise PreflightError("fixture key fingerprint could not be inspected")
    matches = re.findall(r"SHA256:[A-Za-z0-9+/]+", result.stdout)
    if len(matches) != 1:
        raise PreflightError("fixture key fingerprint output is ambiguous")
    return matches[0]


def read_child_home(pid: int, *, runner: Callable[..., subprocess.CompletedProcess[str]] = subprocess.run) -> tuple[str, str, str]:
    try:
        result = runner(
            ["/bin/ps", "eww", "-p", str(pid), "-o", "command="],
            capture_output=True,
            text=True,
            check=False,
            env={"PATH": "/usr/bin:/bin"},
            timeout=5,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise PreflightError("backend process environment could not be inspected") from exc
    if result.returncode != 0 or not result.stdout.strip():
        raise PreflightError("backend process environment could not be inspected")
    line = result.stdout.strip()
    home = parse_process_env_value(line, "HOME")
    kandev_home = parse_process_env_value(line, "KANDEV_HOME_DIR")
    database = parse_process_env_value(line, "KANDEV_DATABASE_PATH")
    return home, kandev_home, database


def listener_pids(port: int, *, runner: Callable[..., subprocess.CompletedProcess[str]] = subprocess.run) -> set[int]:
    try:
        result = runner(
            ["/usr/sbin/lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN"],
            capture_output=True,
            text=True,
            check=False,
            env={"PATH": "/usr/bin:/bin"},
            timeout=5,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise PreflightError("backend listener ownership could not be inspected") from exc
    if result.returncode not in (0, 1):
        raise PreflightError("backend listener ownership could not be inspected")
    pids: set[int] = set()
    for line in result.stdout.splitlines()[1:]:
        fields = line.split()
        if len(fields) < 2 or not fields[1].isdigit():
            raise PreflightError("backend listener ownership output is ambiguous")
        pids.add(int(fields[1]))
    return pids


def process_details(pid: int, *, runner: Callable[..., subprocess.CompletedProcess[str]] = subprocess.run) -> tuple[int, str, str]:
    """Return parent PID, executable and full env-bearing command in memory."""
    try:
        parent = runner(
            ["/bin/ps", "-p", str(pid), "-o", "ppid="], capture_output=True,
            text=True, check=False, env={"PATH": "/usr/bin:/bin"}, timeout=5,
        )
        command = runner(
            ["/bin/ps", "eww", "-p", str(pid), "-o", "command="], capture_output=True,
            text=True, check=False, env={"PATH": "/usr/bin:/bin"}, timeout=5,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise PreflightError("backend child identity could not be inspected") from exc
    if parent.returncode != 0 or command.returncode != 0 or not parent.stdout.strip() or not command.stdout.strip():
        raise PreflightError("backend child identity could not be inspected")
    line = command.stdout.strip()
    executable = line.split(maxsplit=1)[0]
    try:
        parent_pid = int(parent.stdout.strip())
    except ValueError as exc:
        raise PreflightError("backend child parent identity is invalid") from exc
    return parent_pid, executable, line


def sha256_file(path: Path) -> str:
    try:
        h = hashlib.sha256()
        with path.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                h.update(chunk)
    except OSError as exc:
        raise PreflightError("expected backend binary cannot be read") from exc
    return h.hexdigest()


def verify_launch_manifest(
    path: str | os.PathLike[str],
    *,
    executable: str | os.PathLike[str],
    expected_home: str | os.PathLike[str],
    expected_database: str | os.PathLike[str],
    port: int,
) -> None:
    # Normalize path-like inputs at the boundary. Callers naturally have
    # ``Path`` objects for fixture roots; comparing those directly with the
    # JSON manifest's string values rejects a correct launch after it has
    # already started. The preflight must accept the same documented values
    # whether a caller supplies strings or ``Path`` objects.
    try:
        path_text = os.fspath(path)
        executable_text = os.fspath(executable)
        home_text = os.fspath(expected_home)
        database_text = os.fspath(expected_database)
    except TypeError as exc:
        raise PreflightError("supervisor manifest paths must be strings or path-like values") from exc
    if not all(isinstance(value, str) and value for value in (path_text, executable_text, home_text, database_text)):
        raise PreflightError("supervisor manifest paths must be non-empty strings")
    path = Path(path_text)
    try:
        info = path.lstat()
        if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
            raise PreflightError("supervisor manifest must be a user-owned private file")
        manifest = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise PreflightError("supervisor manifest is unavailable") from exc
    if not isinstance(manifest, dict) or manifest.get("mode") not in {"start", "run"}:
        raise PreflightError("supervisor manifest mode is invalid")
    if manifest.get("backend_executable") != executable_text or manifest.get("argv") != ["__backend"]:
        raise PreflightError("supervisor manifest does not identify the expected backend child")
    if manifest.get("home_dir") != home_text or manifest.get("port") != port:
        raise PreflightError("supervisor manifest home or port differs from fixture")
    env = manifest.get("env")
    if not isinstance(env, dict) or env.get("HOME") != home_text or env.get("KANDEV_HOME_DIR") != home_text or env.get("KANDEV_DATABASE_PATH") != database_text:
        raise PreflightError("supervisor manifest does not preserve the expected child HOME and database")


def strict_ssh_handshake(
    *, host: str, port: int, user: str, identity: Path, known_hosts: Path,
    runner: Callable[..., subprocess.CompletedProcess[str]] = subprocess.run,
) -> None:
    argv = [
        "/usr/bin/ssh", "-F", "/dev/null", "-i", str(identity),
        "-p", str(port), "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes",
        "-o", "PreferredAuthentications=publickey", "-o", "PasswordAuthentication=no",
        "-o", "StrictHostKeyChecking=yes", "-o", f"UserKnownHostsFile={known_hosts}",
        "-o", "ConnectTimeout=5", f"{user}@{host}", "/usr/bin/true",
    ]
    try:
        result = runner(argv, capture_output=True, text=True, check=False, env={"PATH": "/usr/bin:/bin"}, timeout=8)
    except (OSError, subprocess.SubprocessError) as exc:
        raise PreflightError("pinned SSH handshake could not complete") from exc
    if result.returncode != 0:
        raise PreflightError("pinned SSH handshake rejected the fixture identity or host key")


def run(args: argparse.Namespace) -> dict[str, object]:
    root = Path(args.fixture_root).resolve(strict=True)
    root_info = root.lstat()
    if stat.S_ISLNK(root_info.st_mode) or not stat.S_ISDIR(root_info.st_mode) or root_info.st_uid != os.getuid() or root_info.st_mode & 0o077:
        raise PreflightError("fixture root must be user-owned and private")

    registry = OwnedProcessRegistry(root)
    record = next((item for item in registry.read_records() if item["label"] == args.backend_label), None)
    if record is None or record["pid"] != args.backend_pid:
        raise PreflightError("backend PID does not match the owned registry")
    if registry._process_identity(record["pid"]) != (record["started_at"], record["pgid"]):
        raise PreflightError("backend process identity changed")
    listeners = listener_pids(args.port)
    if len(listeners) != 1:
        raise PreflightError("configured API listener is missing or ambiguous")
    listener_pid = next(iter(listeners))
    parent_pid, executable_text, _ = process_details(listener_pid)
    executable = Path(args.backend_binary).resolve(strict=True)
    expected_binary_hash = args.backend_sha256.lower()
    if not re.fullmatch(r"[0-9a-f]{64}", expected_binary_hash):
        raise PreflightError("expected backend SHA-256 is invalid")
    if parent_pid != record["pid"] or executable_text != str(executable):
        raise PreflightError("API listener is not the registered launcher's expected backend child")
    if sha256_file(executable) != expected_binary_hash:
        raise PreflightError("backend binary hash differs from current fixture build")

    home, kandev_home, database = read_child_home(listener_pid)
    expected_home = str(Path(args.expected_home).resolve(strict=True))
    expected_database = str(Path(args.expected_database).resolve(strict=True))
    if home != expected_home or kandev_home != expected_home or database != expected_database:
        raise PreflightError("actual backend HOME or database differs from fixture manifest")
    launch_manifest = Path(args.launch_manifest).resolve(strict=True)
    verify_launch_manifest(launch_manifest, executable=executable, expected_home=expected_home, expected_database=expected_database, port=args.port)

    identity = identity_path_for_home(home)
    private_regular_file(identity, root=root)
    expected_key = Path(args.expected_key).resolve(strict=True)
    private_regular_file(expected_key, root=root)
    identity_fp = key_fingerprint(identity)
    expected_fp = key_fingerprint(expected_key)
    if identity_fp != expected_fp:
        raise PreflightError("backend HOME resolves to a different SSH identity")

    profile = json.loads(Path(args.profile_source).read_text(encoding="utf-8"))
    config = profile.get("config", {})
    target = config.get("ssh_target", "")
    match = re.fullmatch(r"([A-Za-z0-9_.-]+)@([A-Za-z0-9_.-]+)", target)
    if not match or match.group(2) != args.host or str(config.get("ssh_port")) != str(args.ssh_port):
        raise PreflightError("private executor profile SSH target differs from the fixture")
    if config.get("ssh_host_fingerprint") != args.host_fingerprint:
        raise PreflightError("private executor profile host fingerprint differs from the fixture")
    known_hosts = Path(args.known_hosts).resolve(strict=True)
    private_regular_file(known_hosts, root=root)
    if key_fingerprint(known_hosts) != args.host_fingerprint:
        raise PreflightError("known-hosts file does not contain the configured host fingerprint")
    strict_ssh_handshake(host=args.host, port=args.ssh_port, user=match.group(1), identity=identity, known_hosts=known_hosts)
    return {
        "preflight_passed": True,
        "backend_label": args.backend_label,
        "backend_pid": record["pid"],
        "backend_child_pid": listener_pid,
        "backend_start_identity": record["started_at"],
        "backend_binary": str(executable),
        "backend_sha256": expected_binary_hash,
        "api_listener_port": args.port,
        "supervisor_manifest": str(launch_manifest),
        "supervisor_manifest_home_verified": True,
        "fixture_home_verified": True,
        "fixture_database_verified": True,
        "identity_path": str(identity),
        "identity_fingerprint_match": True,
        "pinned_ssh_handshake": True,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixture-root", required=True)
    parser.add_argument("--backend-label", required=True)
    parser.add_argument("--backend-pid", required=True, type=int)
    parser.add_argument("--port", required=True, type=int)
    parser.add_argument("--expected-home", required=True)
    parser.add_argument("--expected-database", required=True)
    parser.add_argument("--backend-binary", required=True)
    parser.add_argument("--backend-sha256", required=True)
    parser.add_argument("--launch-manifest", required=True)
    parser.add_argument("--expected-key", required=True)
    parser.add_argument("--profile-source", required=True)
    parser.add_argument("--known-hosts", required=True)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--ssh-port", type=int, default=22422)
    parser.add_argument("--host-fingerprint", required=True)
    args = parser.parse_args(argv)
    try:
        result = run(args)
    except (OSError, ValueError, ProcessOwnershipError) as exc:
        print(json.dumps({"preflight_passed": False, "error_category": type(exc).__name__, "reason": str(exc)}, sort_keys=True))
        return 2
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
